//go:build darwin

package dict

/*
#cgo LDFLAGS: -framework CoreServices -framework CoreFoundation
#include <stdlib.h>
#include <CoreFoundation/CoreFoundation.h>

// DCSCopyTextDefinition lives in CoreServices' DictionaryServices and is not
// declared in any public SDK header, only exported by the framework binary:
//
//	CFStringRef DCSCopyTextDefinition(CFStringRef dictName,
//	                                  CFStringRef text,
//	                                  CFRange pinpoint);
//
// The prototype is therefore written by hand. A CFRange is two 8-byte
// integers, and both the arm64 and the x86-64 C ABIs pass that struct in two
// integer registers, so spelling the trailing pair as scalars matches how the
// function is really called. dictName is NULL to mean "the dictionaries the
// user enabled", and pinpoint is (0, length of text) — a zero length makes the
// service answer nothing.
extern void *DCSCopyTextDefinition(void *dictName, void *text, long long loc, long long len);

// dictCopyUTF8 copies a CFStringRef into a malloc'd UTF-8 buffer, or returns
// NULL. The caller owns the buffer.
static char *dictCopyUTF8(void *value) {
	if (value == NULL) {
		return NULL;
	}
	CFStringRef text = (CFStringRef) value;
	if (CFGetTypeID(text) != CFStringGetTypeID()) {
		return NULL;
	}
	char stack[4096];
	if (CFStringGetCString(text, stack, sizeof(stack), kCFStringEncodingUTF8)) {
		return strdup(stack);
	}
	CFIndex units = CFStringGetLength(text);
	CFIndex bytes = CFStringGetMaximumSizeForEncoding(units, kCFStringEncodingUTF8);
	if (bytes <= 0) {
		return NULL;
	}
	char *heap = (char *) malloc((size_t) bytes + 1);
	if (heap == NULL) {
		return NULL;
	}
	if (CFStringGetCString(text, heap, bytes + 1, kCFStringEncodingUTF8)) {
		return heap;
	}
	free(heap);
	return NULL;
}

// dictAskDefinition does the whole CoreFoundation dance in C so no CF reference
// ever becomes a Go unsafe.Pointer: build a CFStringRef, ask Dictionary
// Services, copy the answer out, release both references.
static char *dictAskDefinition(const char *word) {
	if (word == NULL) {
		return NULL;
	}
	CFStringRef text = CFStringCreateWithCString(kCFAllocatorDefault, word,
		kCFStringEncodingUTF8);
	if (text == NULL) {
		return NULL;
	}
	CFRange whole = CFRangeMake(0, CFStringGetLength(text));
	CFStringRef result = (CFStringRef) DCSCopyTextDefinition(NULL, (void *) text,
		(long long) whole.location, (long long) whole.length);
	CFRelease(text);
	if (result == NULL) {
		return NULL;
	}
	char *out = dictCopyUTF8((void *) result);
	CFRelease(result);
	return out;
}
*/
import "C"

import (
	"context"
	"fmt"
	"strings"
	"unicode"
	"unsafe"

	"transdict/internal/settings"
)

// apple asks macOS itself for a definition through Dictionary Services, the
// engine behind the Dictionary app and look-up. It is offline, free and
// word-only.
type appleDict struct{}

// appleMaxQueryRunes keeps long selections out of an API meant for headwords.
const appleMaxQueryRunes = 128

func init() {
	Register(settings.ServiceType{
		Kind: "apple", Name: "苹果词典", Note: "macOS 系统词典，仅单词", Free: true,
		Default: settings.Provider{Enabled: true},
	}, func(settings.Provider) (Provider, error) {
		return &appleDict{}, nil
	})
}

func (a *appleDict) Label() string { return "苹果词典" }

func (a *appleDict) Explain(ctx context.Context, text, sourceLang, targetLang string) ([]Entry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	word := cleanLookup(text)
	if word == "" {
		return nil, fmt.Errorf("苹果词典：查询内容为空")
	}
	if len([]rune(word)) > appleMaxQueryRunes {
		return nil, fmt.Errorf("苹果词典：仅支持单词查询（最多 %d 个字符）", appleMaxQueryRunes)
	}

	definition := copyTextDefinition(word)
	if strings.TrimSpace(definition) == "" {
		return nil, fmt.Errorf("苹果词典未收录该词：%s", clip(word, 40))
	}
	if wrongScript(word, definition) {
		return nil, fmt.Errorf("苹果词典未收录该词：%s", clip(word, 40))
	}
	// The service returns already-formatted plain text that repeats the
	// headword, so Word carries the query and Text the body.
	return []Entry{{Word: word, Text: definition}}, nil
}

// wrongScript reports an answer that is in the wrong writing system for the
// query. Dictionary Services ranks a pinyin match above the English headword,
// so with the Chinese dictionary installed it answered "run" with 瞤 (rún) and
// the card showed an unrelated word instead of saying the word was not found.
func wrongScript(query, definition string) bool {
	latin := false
	for _, r := range query {
		switch {
		case unicode.Is(unicode.Han, r):
			return false
		case unicode.IsLetter(r):
			latin = true
		}
	}
	if !latin {
		return false
	}
	for _, r := range definition {
		switch {
		case unicode.IsSpace(r), unicode.IsDigit(r), unicode.IsPunct(r):
			continue
		case unicode.IsLetter(r):
			return unicode.Is(unicode.Han, r)
		}
	}
	return false
}

func copyTextDefinition(word string) string {
	cword := C.CString(word)
	defer C.free(unsafe.Pointer(cword))

	cstr := C.dictAskDefinition(cword)
	if cstr == nil {
		return ""
	}
	defer C.free(unsafe.Pointer(cstr))
	return C.GoString(cstr)
}
