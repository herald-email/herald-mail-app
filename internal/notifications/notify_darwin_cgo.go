//go:build darwin && cgo

package notifications

/*
#cgo CFLAGS: -x objective-c -fblocks
#cgo LDFLAGS: -framework Foundation -framework UserNotifications
#import <Foundation/Foundation.h>
#import <UserNotifications/UserNotifications.h>
#include <stdlib.h>
#include <string.h>

extern void heraldNotificationActivated(char* link);

@interface HeraldNotificationDelegate : NSObject<UNUserNotificationCenterDelegate>
@end

@implementation HeraldNotificationDelegate
- (void)userNotificationCenter:(UNUserNotificationCenter *)center didReceiveNotificationResponse:(UNNotificationResponse *)response withCompletionHandler:(void (^)(void))completionHandler {
	NSDictionary *info = response.notification.request.content.userInfo;
	id link = [info objectForKey:@"deep_link"];
	if ([link isKindOfClass:[NSString class]]) {
		const char *utf8 = [(NSString *)link UTF8String];
		if (utf8 != NULL) {
			heraldNotificationActivated((char *)utf8);
		}
	}
	completionHandler();
}

- (void)userNotificationCenter:(UNUserNotificationCenter *)center willPresentNotification:(UNNotification *)notification withCompletionHandler:(void (^)(UNNotificationPresentationOptions options))completionHandler {
	UNNotificationPresentationOptions options = UNNotificationPresentationOptionSound;
	if (@available(macOS 11.0, *)) {
		options |= UNNotificationPresentationOptionBanner | UNNotificationPresentationOptionList;
	} else {
#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Wdeprecated-declarations"
		options |= UNNotificationPresentationOptionAlert;
#pragma clang diagnostic pop
	}
	completionHandler(options);
}
@end

static HeraldNotificationDelegate *heraldDelegate;

static NSString *heraldString(const char *value, NSString *fallback) {
	if (value == NULL || strlen(value) == 0) {
		return fallback;
	}
	return [NSString stringWithUTF8String:value];
}

int heraldCanUseUserNotifications(void) {
	@autoreleasepool {
		NSBundle *bundle = [NSBundle mainBundle];
		if (bundle == nil) {
			return 0;
		}
		NSURL *bundleURL = [bundle bundleURL];
		if (bundleURL == nil) {
			return 0;
		}
		NSString *extension = [[bundleURL pathExtension] lowercaseString];
		if (extension == nil || ![extension isEqualToString:@"app"]) {
			return 0;
		}
		NSString *identifier = [bundle bundleIdentifier];
		if (identifier == nil || [identifier length] == 0) {
			return 0;
		}
		return 1;
	}
}

int heraldInitNotifications(void) {
	@autoreleasepool {
		@try {
			UNUserNotificationCenter *center = [UNUserNotificationCenter currentNotificationCenter];
			if (heraldDelegate == nil) {
				heraldDelegate = [HeraldNotificationDelegate new];
			}
			center.delegate = heraldDelegate;
			[center requestAuthorizationWithOptions:(UNAuthorizationOptionAlert | UNAuthorizationOptionSound)
				completionHandler:^(BOOL granted, NSError * _Nullable error) {}];
			return 1;
		} @catch (NSException *exception) {
			return 0;
		}
	}
}

int heraldPostNotification(const char *identifier, const char *title, const char *body, const char *deepLink, int sound) {
	@autoreleasepool {
		@try {
			UNMutableNotificationContent *content = [UNMutableNotificationContent new];
			content.title = heraldString(title, @"Herald");
			content.body = heraldString(body, @"");
			if (sound) {
				content.sound = [UNNotificationSound defaultSound];
			}
			if (deepLink != NULL && strlen(deepLink) > 0) {
				content.userInfo = @{@"deep_link": heraldString(deepLink, @"")};
			}
			NSString *requestID = heraldString(identifier, [[NSUUID UUID] UUIDString]);
			UNNotificationRequest *request = [UNNotificationRequest requestWithIdentifier:requestID content:content trigger:nil];
			[[UNUserNotificationCenter currentNotificationCenter] addNotificationRequest:request withCompletionHandler:^(NSError * _Nullable error) {}];
			return 1;
		} @catch (NSException *exception) {
			return 0;
		}
	}
}
*/
import "C"

import (
	"context"
	"sync"
	"unsafe"
)

type platformNotifier struct {
	opts      Options
	responses chan string
}

var (
	darwinMu          sync.Mutex
	darwinInitialized bool
	darwinAvailable   bool

	canUseDarwinUserNotifications = func() bool {
		return C.heraldCanUseUserNotifications() != 0
	}
	initDarwinNotifications = func() bool {
		return C.heraldInitNotifications() != 0
	}
)

func newPlatformNotifier(opts Options) Notifier {
	if !ensureDarwinNotifications() {
		return noopNotifier{platform: "darwin"}
	}
	n := &platformNotifier{
		opts:      opts,
		responses: make(chan string, 16),
	}
	setDarwinResponseChannel(n.responses)
	return n
}

func ensureDarwinNotifications() bool {
	darwinMu.Lock()
	defer darwinMu.Unlock()

	if !darwinInitialized {
		darwinAvailable = canUseDarwinUserNotifications() && initDarwinNotifications()
		darwinInitialized = true
	}
	return darwinAvailable
}

func (n *platformNotifier) Notify(ctx context.Context, req Request) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	id := C.CString(req.ID)
	title := C.CString(req.Title)
	body := C.CString(req.Body)
	deepLink := C.CString(req.DeepLink)
	defer C.free(unsafe.Pointer(id))
	defer C.free(unsafe.Pointer(title))
	defer C.free(unsafe.Pointer(body))
	defer C.free(unsafe.Pointer(deepLink))
	sound := 0
	if req.Sound || n.opts.Sound {
		sound = 1
	}
	C.heraldPostNotification(id, title, body, deepLink, C.int(sound))
	return ctx.Err()
}

func (n *platformNotifier) Responses() <-chan string { return n.responses }
func (n *platformNotifier) Capabilities() Capabilities {
	return Capabilities{Delivery: true, Activation: true, Platform: "darwin"}
}
func (n *platformNotifier) Close() error { return nil }
