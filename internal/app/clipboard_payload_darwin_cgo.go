//go:build darwin && cgo

package app

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Foundation -framework AppKit
#include <stdlib.h>
#import <Foundation/Foundation.h>
#import <AppKit/AppKit.h>

static int heraldWritePasteboard(const char *plain, const char *html, const void *imageBytes, long imageLen) {
	@autoreleasepool {
		NSPasteboard *pb = [NSPasteboard generalPasteboard];
		[pb clearContents];
		BOOL wrote = NO;
		if (imageBytes != NULL && imageLen > 0) {
			NSData *data = [NSData dataWithBytes:imageBytes length:(NSUInteger)imageLen];
			NSImage *image = [[NSImage alloc] initWithData:data];
			if (image != nil) {
				wrote = [pb writeObjects:@[image]];
			}
		}
		if (plain != NULL) {
			NSString *plainString = [NSString stringWithUTF8String:plain];
			if (plainString != nil) {
				wrote = [pb setString:plainString forType:NSPasteboardTypeString] || wrote;
			}
		}
		if (html != NULL) {
			NSString *htmlString = [NSString stringWithUTF8String:html];
			if (htmlString != nil) {
				wrote = [pb setString:htmlString forType:NSPasteboardTypeHTML] || wrote;
			}
		}
		return wrote ? 0 : 1;
	}
}
*/
import "C"

import (
	"fmt"
	"unsafe"
)

func writeSystemPreviewClipboard(payload previewClipboardPayload) (string, error) {
	var plain *C.char
	if payload.Plain != "" {
		plain = C.CString(payload.Plain)
		defer C.free(unsafe.Pointer(plain))
	}
	var html *C.char
	if payload.HTML != "" {
		html = C.CString(payload.HTML)
		defer C.free(unsafe.Pointer(html))
	}
	var imagePtr unsafe.Pointer
	var imageLen C.long
	if payload.Image != nil && len(payload.Image.Data) > 0 {
		imagePtr = unsafe.Pointer(&payload.Image.Data[0])
		imageLen = C.long(len(payload.Image.Data))
	}
	if C.heraldWritePasteboard(plain, html, imagePtr, imageLen) != 0 {
		return "", fmt.Errorf("pasteboard rejected payload")
	}
	switch {
	case payload.Image != nil && len(payload.Image.Data) > 0:
		return "Image copied", nil
	case payload.HTML != "":
		return "Rich text copied", nil
	default:
		return "Text copied", nil
	}
}
