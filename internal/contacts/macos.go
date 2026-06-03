//go:build darwin && cgo

package contacts

/*
#cgo CFLAGS: -x objective-c -fblocks
#cgo LDFLAGS: -framework Foundation -framework Contacts
#import <Foundation/Foundation.h>
#import <Contacts/Contacts.h>
#import <dispatch/dispatch.h>
#include <stdlib.h>

static NSString *heraldContactDisplayName(CNContact *contact) {
	NSMutableArray<NSString *> *parts = [NSMutableArray arrayWithCapacity:3];
	if (contact.givenName.length > 0) {
		[parts addObject:contact.givenName];
	}
	if (contact.middleName.length > 0) {
		[parts addObject:contact.middleName];
	}
	if (contact.familyName.length > 0) {
		[parts addObject:contact.familyName];
	}
	NSString *name = [parts componentsJoinedByString:@" "];
	if (name.length > 0) {
		return name;
	}
	if (contact.organizationName.length > 0) {
		return contact.organizationName;
	}
	return @"";
}

static char *heraldEmptyContactsJSON(void) {
	return strdup("[]");
}

static BOOL heraldEnsureContactsAccess(void) {
	CNAuthorizationStatus status = [CNContactStore authorizationStatusForEntityType:CNEntityTypeContacts];
	if (status == CNAuthorizationStatusAuthorized) {
		return YES;
	}
	if (status != CNAuthorizationStatusNotDetermined) {
		return NO;
	}

	CNContactStore *store = [[CNContactStore alloc] init];
	if (store == nil) {
		return NO;
	}
	__block BOOL granted = NO;
	dispatch_semaphore_t semaphore = dispatch_semaphore_create(0);
	[store requestAccessForEntityType:CNEntityTypeContacts completionHandler:^(BOOL ok, NSError *error) {
		granted = ok;
		dispatch_semaphore_signal(semaphore);
	}];
	dispatch_semaphore_wait(semaphore, DISPATCH_TIME_FOREVER);
#if !OS_OBJECT_USE_OBJC
	dispatch_release(semaphore);
#endif
	[store release];
	return granted;
}

static char *heraldCopyContactsJSON(void) {
	@autoreleasepool {
		if (!heraldEnsureContactsAccess()) {
			return heraldEmptyContactsJSON();
		}
		CNContactStore *store = [[CNContactStore alloc] init];
		if (store == nil) {
			return heraldEmptyContactsJSON();
		}

		NSArray *keys = @[
			CNContactGivenNameKey,
			CNContactMiddleNameKey,
			CNContactFamilyNameKey,
			CNContactOrganizationNameKey,
			CNContactEmailAddressesKey,
		];
		CNContactFetchRequest *request = [[CNContactFetchRequest alloc] initWithKeysToFetch:keys];
		request.mutableObjects = NO;
		request.unifyResults = YES;

		NSMutableArray<NSDictionary *> *rows = [NSMutableArray array];
		NSError *fetchError = nil;
		BOOL ok = [store enumerateContactsWithFetchRequest:request error:&fetchError usingBlock:^(CNContact *contact, BOOL *stop) {
			NSString *name = heraldContactDisplayName(contact);
			for (CNLabeledValue<NSString *> *labeledEmail in contact.emailAddresses) {
				NSString *email = labeledEmail.value;
				if (email.length == 0) {
					continue;
				}
				[rows addObject:@{@"name": name, @"email": email}];
			}
		}];

		[request release];
		[store release];
		if (!ok) {
			return heraldEmptyContactsJSON();
		}

		NSError *jsonError = nil;
		NSData *data = [NSJSONSerialization dataWithJSONObject:rows options:0 error:&jsonError];
		if (data == nil) {
			return heraldEmptyContactsJSON();
		}
		NSString *json = [[NSString alloc] initWithData:data encoding:NSUTF8StringEncoding];
		if (json == nil) {
			return heraldEmptyContactsJSON();
		}
		char *result = strdup([json UTF8String]);
		[json release];
		return result;
	}
}
*/
import "C"

import (
	"unsafe"

	"github.com/herald-email/herald-mail-app/internal/models"
)

// ImportFromAppleContacts reads macOS Contacts through Contacts.framework.
// Permission failures and unavailable contact stores are non-fatal.
func ImportFromAppleContacts() ([]models.ContactAddr, error) {
	ptr := C.heraldCopyContactsJSON()
	if ptr == nil {
		return nil, nil
	}
	defer C.free(unsafe.Pointer(ptr))
	return parseContactsJSON(C.GoString(ptr)), nil
}
