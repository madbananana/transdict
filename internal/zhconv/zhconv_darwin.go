//go:build darwin

// Package zhconv restates Chinese text in 繁体. Some services accept 繁体 as a
// target and answer in 简体 anyway (有道's free endpoint ignores t=zh-CHT), and
// a card of 简体 glosses beside a 繁體 translation reads as half-finished work.
package zhconv

/*
#cgo LDFLAGS: -framework CoreFoundation
#include <CoreFoundation/CoreFoundation.h>
#include <stdlib.h>

// ConvertSimplified returns the ICU Simplified-Traditional transform of s as a
// malloc'd UTF-8 string, or NULL when the transform is unavailable. ICU's table
// is phrase-aware enough for common words (头发 → 頭髮, 干了 → 乾了), so the
// answer reads as real 繁体 rather than a character-by-character guess.
static char *ConvertSimplified(const char *s) {
	CFStringRef src = CFStringCreateWithCString(NULL, s, kCFStringEncodingUTF8);
	if (src == NULL) return NULL;
	CFMutableStringRef out = CFStringCreateMutableCopy(NULL, 0, src);
	CFRelease(src);
	if (out == NULL) return NULL;
	if (!CFStringTransform(out, NULL, CFSTR("Simplified-Traditional"), false)) {
		CFRelease(out);
		return NULL;
	}
	CFIndex size = CFStringGetMaximumSizeForEncoding(CFStringGetLength(out), kCFStringEncodingUTF8) + 1;
	char *buf = (char *)malloc((size_t)size);
	if (buf == NULL) {
		CFRelease(out);
		return NULL;
	}
	Boolean ok = CFStringGetCString(out, buf, size, kCFStringEncodingUTF8);
	CFRelease(out);
	if (!ok) {
		free(buf);
		return NULL;
	}
	return buf;
}
*/
import "C"

import "unsafe"

// Traditional renders Chinese text in 繁体; text already in 繁体 comes back
// unchanged, and English passes through. A failure returns the input as it was
// rather than losing the answer.
func Traditional(s string) string {
	if s == "" {
		return s
	}
	cs := C.CString(s)
	defer C.free(unsafe.Pointer(cs))
	out := C.ConvertSimplified(cs)
	if out == nil {
		return s
	}
	defer C.free(unsafe.Pointer(out))
	return C.GoString(out)
}
