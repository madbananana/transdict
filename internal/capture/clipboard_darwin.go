//go:build darwin

package capture

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework AppKit -framework Foundation
#include <stdlib.h>
#include <string.h>
#import <AppKit/AppKit.h>

// clipboardPNG copies the image on the pasteboard out as PNG bytes. Most apps
// publish a PNG or TIFF flavour; a TIFF-only copy is re-encoded so the caller
// always gets one format. NULL means "no picture on the pasteboard".
static void *clipboardPNG(size_t *length) {
	@autoreleasepool {
		NSPasteboard *pb = [NSPasteboard generalPasteboard];
		NSData *data = [pb dataForType:NSPasteboardTypePNG];
		if (data == nil) {
			NSData *tiff = [pb dataForType:NSPasteboardTypeTIFF];
			if (tiff == nil) { return NULL; }
			NSBitmapImageRep *rep = [NSBitmapImageRep imageRepWithData:tiff];
			if (rep == nil) { return NULL; }
			data = [rep representationUsingType:NSBitmapImageFileTypePNG properties:@{}];
		}
		if (data == nil || data.length == 0) { return NULL; }
		void *out = malloc(data.length);
		if (out == NULL) { return NULL; }
		memcpy(out, data.bytes, data.length);
		*length = (size_t)data.length;
		return out;
	}
}
*/
import "C"

import (
	"errors"
	"unsafe"
)

// ClipboardImage returns the picture the pasteboard holds as PNG. It is what
// 剪贴板 OCR reads: a screenshot taken with ⌃⇧⌘4, an image copied out of a PDF
// or a web page, anything the user already had on the clipboard.
func ClipboardImage() ([]byte, error) {
	var n C.size_t
	p := C.clipboardPNG(&n)
	if p == nil || n == 0 {
		return nil, errors.New("剪贴板里没有图片")
	}
	defer C.free(unsafe.Pointer(p))
	return C.GoBytes(p, C.int(n)), nil
}
