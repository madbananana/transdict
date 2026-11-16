//go:build darwin

package langs

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework NaturalLanguage -framework Foundation
#include <stdlib.h>
#include <Foundation/Foundation.h>
#include <NaturalLanguage/NaturalLanguage.h>

// detectLanguage returns the dominant language code for a UTF-8 string, or NULL
// when the system has no opinion. strdup hands ownership to the caller.
static char *detectLanguage(const char *utf8) {
	@autoreleasepool {
		NSString *text = [NSString stringWithUTF8String:utf8];
		if (text == nil || text.length == 0) return NULL;
		NLLanguageRecognizer *recognizer = [[NLLanguageRecognizer alloc] init];
		[recognizer processString:text];
		NLLanguage language = [recognizer dominantLanguage];
		if (language == nil) return NULL;
		return strdup([language UTF8String]);
	}
}

// detectLanguageHinted is detectLanguage with the languages the reader expects.
// A hint is a tie-breaker, not an override: when the recogniser is already sure
// — a sentence of Japanese has kana, English prose has words — its own answer
// stands, and only an uncertain call is re-asked with the reader's languages.
static char *detectLanguageHinted(const char *utf8, const char *hint1, const char *hint2) {
	@autoreleasepool {
		NSString *text = [NSString stringWithUTF8String:utf8];
		if (text == nil || text.length == 0) return NULL;
		NLLanguageRecognizer *plain = [[NLLanguageRecognizer alloc] init];
		[plain processString:text];
		NSDictionary<NLLanguage, NSNumber *> *guess = [plain languageHypothesesWithMaximum:1];
		double confidence = guess.count > 0 ? [guess.allValues[0] doubleValue] : 0;
		if (confidence >= 0.8) {
			NLLanguage language = [plain dominantLanguage];
			if (language != nil) return strdup([language UTF8String]);
		}
		NLLanguageRecognizer *recognizer = [[NLLanguageRecognizer alloc] init];
		NSMutableDictionary<NLLanguage, NSNumber *> *hints = [NSMutableDictionary dictionary];
		if (hint1 != NULL && *hint1 != '\0') {
			hints[[NSString stringWithUTF8String:hint1]] = @1.0;
		}
		if (hint2 != NULL && *hint2 != '\0') {
			hints[[NSString stringWithUTF8String:hint2]] = @0.9;
		}
		if (hints.count > 0) {
			[recognizer setLanguageHints:hints];
		}
		[recognizer processString:text];
		NLLanguage language = [recognizer dominantLanguage];
		if (language == nil) return NULL;
		return strdup([language UTF8String]);
	}
}
*/
import "C"

import "unsafe"

func detectText(text string) string {
	c := C.CString(text)
	defer C.free(unsafe.Pointer(c))
	out := C.detectLanguage(c)
	if out == nil {
		return ""
	}
	defer C.free(unsafe.Pointer(out))
	return C.GoString(out)
}

// detectTextHinted runs the recogniser with up to two preferred languages. The
// app's own codes are folded into the recogniser's spelling first: it knows
// zh-Hans, not zh-CN.
func detectTextHinted(text string, hints []string) string {
	c := C.CString(text)
	defer C.free(unsafe.Pointer(c))
	var first, second *C.char
	if len(hints) > 0 {
		first = C.CString(hintCode(hints[0]))
		defer C.free(unsafe.Pointer(first))
	}
	if len(hints) > 1 {
		second = C.CString(hintCode(hints[1]))
		defer C.free(unsafe.Pointer(second))
	}
	out := C.detectLanguageHinted(c, first, second)
	if out == nil {
		return ""
	}
	defer C.free(unsafe.Pointer(out))
	return C.GoString(out)
}

func hintCode(code string) string {
	switch code {
	case "zh-CN":
		return "zh-Hans"
	case "zh-TW":
		return "zh-Hant"
	}
	return code
}
