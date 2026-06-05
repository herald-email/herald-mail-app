//go:build darwin && cgo

package printing

/*
#cgo darwin CFLAGS: -x objective-c -fobjc-arc
#cgo darwin LDFLAGS: -framework Cocoa -framework WebKit -framework PDFKit
#include <stdlib.h>
#import <Cocoa/Cocoa.h>
#import <PDFKit/PDFKit.h>
#import <WebKit/WebKit.h>

@interface HeraldPrintNavigationDelegate : NSObject <WKNavigationDelegate>
@property(nonatomic, assign) BOOL finished;
@property(nonatomic, assign) BOOL failed;
@end

@implementation HeraldPrintNavigationDelegate
- (void)webView:(WKWebView *)webView didFinishNavigation:(WKNavigation *)navigation {
	(void)webView;
	(void)navigation;
	self.finished = YES;
}
- (void)webView:(WKWebView *)webView didFailNavigation:(WKNavigation *)navigation withError:(NSError *)error {
	(void)webView;
	(void)navigation;
	(void)error;
	self.failed = YES;
}
- (void)webView:(WKWebView *)webView didFailProvisionalNavigation:(WKNavigation *)navigation withError:(NSError *)error {
	(void)webView;
	(void)navigation;
	(void)error;
	self.failed = YES;
}
@end

@interface HeraldPrintOperationDelegate : NSObject <NSApplicationDelegate>
@property(nonatomic, strong) NSPrintOperation *operation;
@property(nonatomic, assign) int status;
@end

@implementation HeraldPrintOperationDelegate
- (void)applicationDidFinishLaunching:(NSNotification *)notification {
	(void)notification;
	dispatch_after(dispatch_time(DISPATCH_TIME_NOW, (int64_t)(0.10 * NSEC_PER_SEC)), dispatch_get_main_queue(), ^{
		[NSApp activateIgnoringOtherApps:YES];
		BOOL ok = [self.operation runOperation];
		self.status = ok ? 0 : 2;
		[NSApp stop:nil];
		NSEvent *event = [NSEvent otherEventWithType:NSEventTypeApplicationDefined
			location:NSZeroPoint
			modifierFlags:0
			timestamp:0
			windowNumber:0
			context:nil
			subtype:0
			data1:0
			data2:0];
		[NSApp postEvent:event atStart:NO];
	});
}
@end

static int HeraldRunPrintOperationOnAppRunLoop(NSPrintOperation *operation) {
	HeraldPrintOperationDelegate *delegate = [[HeraldPrintOperationDelegate alloc] init];
	delegate.operation = operation;
	delegate.status = 2;
	[NSApp setDelegate:delegate];
	[NSApp run];
	[NSApp setDelegate:nil];
	return delegate.status;
}

static void HeraldSpinRunLoopUntil(NSDate *deadline) {
	while ([deadline timeIntervalSinceNow] > 0) {
		[[NSRunLoop currentRunLoop] runMode:NSDefaultRunLoopMode
			beforeDate:[NSDate dateWithTimeIntervalSinceNow:0.05]];
	}
}

static CGFloat HeraldDocumentHeight(WKWebView *webView, CGFloat minimumHeight) {
	__block BOOL done = NO;
	__block BOOL failed = NO;
	__block CGFloat height = 0.0;
	NSString *script = @"Math.max(document.body.scrollHeight, document.documentElement.scrollHeight)";
	[webView evaluateJavaScript:script completionHandler:^(id result, NSError *error) {
		if (error != nil || result == nil || ![result respondsToSelector:@selector(doubleValue)]) {
			failed = YES;
		} else {
			height = (CGFloat)[result doubleValue];
		}
		done = YES;
	}];
	NSDate *deadline = [NSDate dateWithTimeIntervalSinceNow:5.0];
	while (!done && [deadline timeIntervalSinceNow] > 0) {
		[[NSRunLoop currentRunLoop] runMode:NSDefaultRunLoopMode
			beforeDate:[NSDate dateWithTimeIntervalSinceNow:0.05]];
	}
	if (!done || failed) {
		return 0.0;
	}
	if (height < minimumHeight) {
		return minimumHeight;
	}
	return height;
}

static PDFDocument *HeraldCreatePaginatedPDFDocument(WKWebView *webView, NSPrintInfo *printInfo) {
	CGFloat pageWidth = NSWidth([webView frame]);
	if (pageWidth <= 0.0) {
		pageWidth = 820.0;
	}
	NSSize paperSize = [printInfo paperSize];
	CGFloat paperWidth = paperSize.width > 0.0 ? paperSize.width : 612.0;
	CGFloat paperHeight = paperSize.height > 0.0 ? paperSize.height : 792.0;
	CGFloat pageHeight = pageWidth * paperHeight / paperWidth;
	if (pageHeight <= 0.0) {
		pageHeight = 1061.0;
	}
	CGFloat documentHeight = HeraldDocumentHeight(webView, pageHeight);
	if (documentHeight <= 0.0) {
		return nil;
	}

	NSInteger pageCount = (NSInteger)ceil(documentHeight / pageHeight);
	if (pageCount < 1) {
		pageCount = 1;
	}
	if (pageCount > 500) {
		return nil;
	}

	PDFDocument *combinedDocument = [[PDFDocument alloc] init];
	for (NSInteger pageIndex = 0; pageIndex < pageCount; pageIndex++) {
		WKPDFConfiguration *pdfConfig = [[WKPDFConfiguration alloc] init];
		pdfConfig.rect = CGRectMake(0.0, (CGFloat)pageIndex * pageHeight, pageWidth, pageHeight);

		__block BOOL done = NO;
		__block BOOL failed = NO;
		__block NSData *pageData = nil;
		[webView createPDFWithConfiguration:pdfConfig completionHandler:^(NSData *data, NSError *error) {
			if (error != nil || data == nil || [data length] == 0) {
				failed = YES;
			} else {
				pageData = data;
			}
			done = YES;
		}];

		NSDate *deadline = [NSDate dateWithTimeIntervalSinceNow:20.0];
		while (!done && [deadline timeIntervalSinceNow] > 0) {
			[[NSRunLoop currentRunLoop] runMode:NSDefaultRunLoopMode
				beforeDate:[NSDate dateWithTimeIntervalSinceNow:0.05]];
		}
		if (!done || failed) {
			return nil;
		}

		PDFDocument *pageDocument = [[PDFDocument alloc] initWithData:pageData];
		if (pageDocument == nil || [pageDocument pageCount] == 0) {
			return nil;
		}
		for (NSUInteger i = 0; i < [pageDocument pageCount]; i++) {
			PDFPage *page = [pageDocument pageAtIndex:i];
			if (page == nil) {
				return nil;
			}
			[combinedDocument insertPage:page atIndex:[combinedDocument pageCount]];
		}
	}
	return [combinedDocument pageCount] > 0 ? combinedDocument : nil;
}

static int HeraldRunPrintOperation(const char *filePath, const char *jobTitle, const char *savePath) {
	@autoreleasepool {
		if (filePath == NULL) {
			return 1;
		}
		NSString *path = [NSString stringWithUTF8String:filePath];
		if (path == nil || [path length] == 0) {
			return 1;
		}
		NSURL *url = [NSURL fileURLWithPath:path];
		if (url == nil) {
			return 1;
		}

		[NSApplication sharedApplication];
		[NSApp setActivationPolicy:savePath == NULL
			? NSApplicationActivationPolicyRegular
			: NSApplicationActivationPolicyAccessory];

		NSRect frame = NSMakeRect(-20000, -20000, 820, 1100);
		WKWebViewConfiguration *configuration = [[WKWebViewConfiguration alloc] init];
		WKWebView *webView = [[WKWebView alloc] initWithFrame:frame configuration:configuration];
		HeraldPrintNavigationDelegate *delegate = [[HeraldPrintNavigationDelegate alloc] init];
		webView.navigationDelegate = delegate;

		NSWindow *window = [[NSWindow alloc] initWithContentRect:frame
			styleMask:NSWindowStyleMaskBorderless
			backing:NSBackingStoreBuffered
			defer:NO];
		[window setIgnoresMouseEvents:YES];
		[window setContentView:webView];
		[window orderFront:nil];

		[webView loadFileURL:url allowingReadAccessToURL:[url URLByDeletingLastPathComponent]];
		NSDate *deadline = [NSDate dateWithTimeIntervalSinceNow:15.0];
		while (!delegate.finished && !delegate.failed && [deadline timeIntervalSinceNow] > 0) {
			[[NSRunLoop currentRunLoop] runMode:NSDefaultRunLoopMode
				beforeDate:[NSDate dateWithTimeIntervalSinceNow:0.05]];
		}
		if (delegate.failed || !delegate.finished) {
			[window close];
			return 1;
		}
		[webView setNeedsLayout:YES];
		[webView layoutSubtreeIfNeeded];
		[webView displayIfNeeded];
		[window displayIfNeeded];
		HeraldSpinRunLoopUntil([NSDate dateWithTimeIntervalSinceNow:0.30]);

		NSPrintInfo *printInfo = [[NSPrintInfo sharedPrintInfo] copy];
		[printInfo setTopMargin:0.0];
		[printInfo setBottomMargin:0.0];
		[printInfo setLeftMargin:0.0];
		[printInfo setRightMargin:0.0];
		if (savePath != NULL) {
			NSString *outputPath = [NSString stringWithUTF8String:savePath];
			if (outputPath == nil || [outputPath length] == 0) {
				[window close];
				return 1;
			}
			NSMutableDictionary *settings = [printInfo dictionary];
			[settings setObject:NSPrintSaveJob forKey:NSPrintJobDisposition];
			[settings setObject:[NSURL fileURLWithPath:outputPath] forKey:NSPrintJobSavingURL];
		}

		PDFDocument *printDocument = HeraldCreatePaginatedPDFDocument(webView, printInfo);
		if (printDocument == nil) {
			[window close];
			return 1;
		}
		NSPrintOperation *operation = [printDocument printOperationForPrintInfo:printInfo
			scalingMode:kPDFPrintPageScaleDownToFit
			autoRotate:YES];
		if (operation == nil) {
			[window close];
			return 1;
		}
		[operation setShowsPrintPanel:savePath == NULL];
		[operation setShowsProgressPanel:savePath == NULL];
		if (jobTitle != NULL) {
			NSString *title = [NSString stringWithUTF8String:jobTitle];
			if (title != nil && [title length] > 0) {
				[operation setJobTitle:title];
			}
		}

		BOOL ok = NO;
		int runStatus = 2;
		if (savePath == NULL) {
			runStatus = HeraldRunPrintOperationOnAppRunLoop(operation);
		} else {
			ok = [operation runOperation];
			runStatus = ok ? 0 : 2;
		}
		[window close];
		return runStatus;
	}
}

static int HeraldRunPrintDialog(const char *filePath, const char *jobTitle) {
	return HeraldRunPrintOperation(filePath, jobTitle, NULL);
}

static int HeraldRunPrintSave(const char *filePath, const char *jobTitle, const char *savePath) {
	return HeraldRunPrintOperation(filePath, jobTitle, savePath);
}

*/
import "C"

import (
	"fmt"
	"runtime"
	"unsafe"
)

func runNativePrintDialog(filePath, title string) (Status, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	cPath := C.CString(filePath)
	cTitle := C.CString(title)
	defer C.free(unsafe.Pointer(cPath))
	defer C.free(unsafe.Pointer(cTitle))

	switch code := C.HeraldRunPrintDialog(cPath, cTitle); code {
	case 0:
		return StatusOpened, nil
	case 2:
		return StatusCanceled, nil
	default:
		return "", fmt.Errorf("macOS print dialog failed to load printable document")
	}
}

func runNativePrintSave(filePath, title, outputPath string) (Status, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	cPath := C.CString(filePath)
	cTitle := C.CString(title)
	cOutput := C.CString(outputPath)
	defer C.free(unsafe.Pointer(cPath))
	defer C.free(unsafe.Pointer(cTitle))
	defer C.free(unsafe.Pointer(cOutput))

	switch code := C.HeraldRunPrintSave(cPath, cTitle, cOutput); code {
	case 0:
		return StatusOpened, nil
	case 2:
		return StatusCanceled, nil
	default:
		return "", fmt.Errorf("macOS print helper failed to save printable PDF")
	}
}
