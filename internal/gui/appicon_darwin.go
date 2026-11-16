//go:build darwin

package gui

/*
#cgo CFLAGS: -x objective-c -fblocks
#cgo LDFLAGS: -framework AppKit
#include <stdlib.h>
#include <string.h>
#include <dispatch/dispatch.h>
#import <AppKit/AppKit.h>

// guiAppIconPNG renders the Finder icon of a bundle identifier into a 64px PNG,
// which is what 禁止名单 shows beside each app. AppKit drawing belongs on the
// main thread; a binding call arrives on a Go thread, so the work is dispatched
// and the caller blocks for the answer.
static void *guiAppIconPNG(const char *bundleID, int *outLen) {
	__block void *result = NULL;
	__block int len = 0;
	void (^work)(void) = ^{
		@autoreleasepool {
			NSString *ident = [NSString stringWithUTF8String:bundleID];
			if (ident == nil) return;
			NSURL *url = [[NSWorkspace sharedWorkspace] URLForApplicationWithBundleIdentifier:ident];
			if (url == nil) return;
			NSImage *image = [[NSWorkspace sharedWorkspace] iconForFile:[url path]];
			if (image == nil) return;
			NSImage *scaled = [[NSImage alloc] initWithSize:NSMakeSize(64, 64)];
			[scaled lockFocus];
			[image drawInRect:NSMakeRect(0, 0, 64, 64) fromRect:NSZeroRect
					operation:NSCompositingOperationCopy fraction:1.0];
			[scaled unlockFocus];
			NSBitmapImageRep *rep = [NSBitmapImageRep imageRepWithData:[scaled TIFFRepresentation]];
			if (rep == nil) return;
			NSData *png = [rep representationUsingType:NSBitmapImageFileTypePNG properties:@{}];
			if (png == nil) return;
			NSUInteger n = [png length];
			void *buf = malloc(n);
			if (buf == NULL) return;
			memcpy(buf, [png bytes], n);
			result = buf;
			len = (int)n;
		}
	};
	if ([NSThread isMainThread]) {
		work();
	} else {
		dispatch_sync(dispatch_get_main_queue(), work);
	}
	*outLen = len;
	return result;
}
*/
import "C"

import (
	"encoding/base64"
	"unsafe"
)

func appIconData(bundleID string) string {
	cid := C.CString(bundleID)
	defer C.free(unsafe.Pointer(cid))
	var n C.int
	p := C.guiAppIconPNG(cid, &n)
	if p == nil || n <= 0 {
		return ""
	}
	defer C.free(p)
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(C.GoBytes(p, n))
}
