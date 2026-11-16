//go:build darwin && cgo

package capture

/*
#cgo CFLAGS: -mmacosx-version-min=12.0
#cgo LDFLAGS: -mmacosx-version-min=12.0 -framework CoreGraphics -framework ImageIO -framework CoreFoundation
#include <CoreGraphics/CoreGraphics.h>
#include <ImageIO/ImageIO.h>
#include <CoreFoundation/CoreFoundation.h>

// td_display_ids fills ids with the active displays, main display first, the
// same order screencapture's -D indexes and Wails' screen list use.
static size_t td_display_ids(CGDirectDisplayID *ids, size_t cap) {
	uint32_t n = 0;
	if (CGGetActiveDisplayList((uint32_t)cap, ids, &n) != kCGErrorSuccess) {
		return 0;
	}
	CGDirectDisplayID main = CGMainDisplayID();
	for (uint32_t i = 0; i < n; i++) {
		if (ids[i] == main) {
			CGDirectDisplayID first = ids[0];
			ids[0] = ids[i];
			ids[i] = first;
			break;
		}
	}
	return n;
}

static CGImageRef td_display_image(CGDirectDisplayID id) {
	// Full-resolution image of the whole display. The rect is in the display's
	// own space, so it starts at the origin even on a secondary display; the
	// image comes back at backing-store pixels, which is what OCR wants.
	CGSize size = CGDisplayBounds(id).size;
	return CGDisplayCreateImageForRect(id, CGRectMake(0, 0, size.width, size.height));
}

static size_t td_image_width(CGImageRef img) { return CGImageGetWidth(img); }
static size_t td_image_height(CGImageRef img) { return CGImageGetHeight(img); }
static int td_image_is_null(CGImageRef img) { return img == NULL; }
static void td_image_release(CGImageRef img) { CGImageRelease(img); }

static CFDataRef td_encode(CGImageRef img, CFStringRef type, CFDictionaryRef opts) {
	CFMutableDataRef data = CFDataCreateMutable(kCFAllocatorDefault, 0);
	if (data == NULL) {
		return NULL;
	}
	CGImageDestinationRef dst = CGImageDestinationCreateWithData(data, type, 1, NULL);
	if (dst == NULL) {
		CFRelease(data);
		return NULL;
	}
	CGImageDestinationAddImage(dst, img, opts);
	bool ok = CGImageDestinationFinalize(dst);
	CFRelease(dst);
	if (!ok) {
		CFRelease(data);
		return NULL;
	}
	return data;
}

static CFDataRef td_encode_png(CGImageRef img) {
	CFMutableDictionaryRef png = CFDictionaryCreateMutable(kCFAllocatorDefault, 1,
		&kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
	if (png != NULL) {
		int filter = 0; // no PNG row filters: the frozen frame is cropped before it leaves the app
		CFNumberRef n = CFNumberCreate(kCFAllocatorDefault, kCFNumberIntType, &filter);
		if (n != NULL) {
			CFDictionarySetValue(png, kCGImagePropertyPNGCompressionFilter, n);
			CFRelease(n);
		}
	}
	CFMutableDictionaryRef opts = CFDictionaryCreateMutable(kCFAllocatorDefault, 1,
		&kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
	if (opts != NULL && png != NULL) {
		CFDictionarySetValue(opts, kCGImagePropertyPNGDictionary, png);
	}
	CFDataRef data = td_encode(img, CFSTR("public.png"), opts);
	if (opts != NULL) {
		CFRelease(opts);
	}
	if (png != NULL) {
		CFRelease(png);
	}
	return data;
}

static CFDataRef td_encode_jpeg(CGImageRef img, double quality) {
	CFMutableDictionaryRef opts = CFDictionaryCreateMutable(kCFAllocatorDefault, 1,
		&kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
	if (opts == NULL) {
		return NULL;
	}
	CFNumberRef q = CFNumberCreate(kCFAllocatorDefault, kCFNumberDoubleType, &quality);
	if (q != NULL) {
		CFDictionarySetValue(opts, kCGImageDestinationLossyCompressionQuality, q);
		CFRelease(q);
	}
	CFDataRef data = td_encode(img, CFSTR("public.jpeg"), opts);
	CFRelease(opts);
	return data;
}

static const void *td_data_bytes(CFDataRef data) { return CFDataGetBytePtr(data); }
static CFIndex td_data_length(CFDataRef data) { return CFDataGetLength(data); }
static void td_data_release(CFDataRef data) { CFRelease(data); }
*/
import "C"

import (
	"errors"
)

// previewQuality is invisible under the overlay's dim layer but keeps the
// image small enough to reach the page in one binding round trip.
const previewQuality = 0.8

// captureFast takes every display in one CGDisplayCreateImage pass and encodes
// both the OCR frame and the display copy with ImageIO. The alternative is one
// `screencapture` process per display (~185ms) plus a Go PNG decode and JPEG
// encode (~170ms) before the overlay can show anything.
func captureFast() ([]Frame, error) {
	ids := make([]C.CGDirectDisplayID, 16)
	n := int(C.td_display_ids(&ids[0], C.size_t(len(ids))))
	if n == 0 {
		return nil, errors.New("capture: no active displays")
	}
	frames := make([]Frame, 0, n)
	for i := 0; i < n; i++ {
		img := C.td_display_image(ids[i])
		if C.td_image_is_null(img) != 0 {
			return nil, errors.New("capture: CGDisplayCreateImageForRect returned nil")
		}
		width, height := int(C.td_image_width(img)), int(C.td_image_height(img))
		png := copyCFData(C.td_encode_png(img))
		preview := copyCFData(C.td_encode_jpeg(img, C.double(previewQuality)))
		C.td_image_release(img)
		if len(png) == 0 || len(preview) == 0 || width == 0 || height == 0 {
			return nil, errors.New("capture: display image encode failed")
		}
		frames = append(frames, Frame{Index: i, PNG: png, Preview: preview, Width: width, Height: height})
	}
	return frames, nil
}

func copyCFData(data C.CFDataRef) []byte {
	size := C.td_data_length(data)
	if size == 0 {
		return nil
	}
	defer C.td_data_release(data)
	return C.GoBytes(C.td_data_bytes(data), C.int(size))
}
