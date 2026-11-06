//go:build darwin

package selection

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework ApplicationServices -framework AppKit
#include <stdlib.h>
#include <ApplicationServices/ApplicationServices.h>
#import <AppKit/AppKit.h>

// CGEventKeyCodes live in IOKit/hidsystem/ev_keymap.h, which ApplicationServices
// does not pull in; the numbers are fixed by the HID spec.
enum { kVK_Command = 55, kVK_ANSI_C = 8, kVK_ANSI_V = 9 };

// cfToUTF8 copies a CFStringRef into a fresh malloc'd buffer. NULL for anything
// that is not a string keeps the Go side from having to type-check CoreFoundation.
static char *cfToUTF8(CFTypeRef value) {
	if (value == NULL || CFGetTypeID(value) != CFStringGetTypeID()) return NULL;
	CFStringRef text = (CFStringRef)value;
	CFIndex length = CFStringGetLength(text);
	CFIndex size = CFStringGetMaximumSizeForEncoding(length, kCFStringEncodingUTF8) + 1;
	char *out = (char *)malloc((size_t)size);
	if (out == NULL) return NULL;
	CFIndex used = 0;
	if (!CFStringGetBytes(text, CFRangeMake(0, length), kCFStringEncodingUTF8, 0, false,
						  (UInt8 *)out, size - 1, &used)) {
		free(out);
		return NULL;
	}
	out[used] = '\0';
	return out;
}

// AXSelectedText reads the focused UI element of the frontmost application.
//
// state: 0 answered (possibly an empty selection, which is a real answer),
// 1 this process is not trusted for accessibility, 2 the target app exposes no
// selectable element. 1 and 2 both mean "use the clipboard route".
static char *AXSelectedText(int *state) {
	*state = 2;
	if (!AXIsProcessTrustedWithOptions(NULL)) { *state = 1; return NULL; }

	AXUIElementRef system = AXUIElementCreateSystemWide();
	AXUIElementRef app = NULL;
	if (AXUIElementCopyAttributeValue(system, kAXFocusedApplicationAttribute,
			(CFTypeRef *)&app) != kAXErrorSuccess || app == NULL) {
		CFRelease(system);
		return NULL;
	}
	AXUIElementRef element = NULL;
	CFTypeRef value = NULL;
	if (AXUIElementCopyAttributeValue(app, kAXFocusedUIElementAttribute,
			(CFTypeRef *)&element) == kAXErrorSuccess && element != NULL) {
		if (AXUIElementCopyAttributeValue(element, kAXSelectedTextAttribute, &value) == kAXErrorSuccess) {
			*state = 0;
		}
		CFRelease(element);
	}
	CFRelease(app);
	CFRelease(system);

	char *out = cfToUTF8(value);
	if (value != NULL) CFRelease(value);
	return out;
}

static void pressKey(CGKeyCode key, bool down) {
	CGEventRef event = CGEventCreateKeyboardEvent(NULL, key, down);
	if (event == NULL) return;
	CGEventPost(kCGHIDEventTap, event);
	CFRelease(event);
}

// SendCopy is the fallback every application understands: cmd+C into whatever
// currently has keyboard focus.
static void SendCopy(void) {
	pressKey(kVK_Command, true);
	pressKey(kVK_ANSI_C, true);
	pressKey(kVK_ANSI_C, false);
	pressKey(kVK_Command, false);
}

// SendPaste is the other half of 翻译并替换: the translation is placed on the
// pasteboard and typed into the field the selection came from.
static void SendPaste(void) {
	pressKey(kVK_Command, true);
	pressKey(kVK_ANSI_V, true);
	pressKey(kVK_ANSI_V, false);
	pressKey(kVK_Command, false);
}

static long long PasteboardCount(void) {
	return (long long)[[NSPasteboard generalPasteboard] changeCount];
}

static Boolean PromptTrust(void) {
	CFStringRef options[] = { kAXTrustedCheckOptionPrompt };
	CFTypeRef values[] = { kCFBooleanTrue };
	CFDictionaryRef dict = CFDictionaryCreate(NULL, (const void **)options,
			(const void **)values, 1, &kCFTypeDictionaryKeyCallBacks,
			&kCFTypeDictionaryValueCallBacks);
	Boolean trusted = AXIsProcessTrustedWithOptions(dict);
	CFRelease(dict);
	return trusted;
}

// BundleIdentifierOfFrontmost names the application that currently owns the
// keyboard, which is how the watcher tells another app's selection apart from a
// highlight inside our own result panel.
static char *BundleIdentifierOfFrontmost(void) {
	NSRunningApplication *app = [NSWorkspace sharedWorkspace].frontmostApplication;
	if (app == nil || app.bundleIdentifier == nil) return NULL;
	return strdup(app.bundleIdentifier.UTF8String);
}

static char *OwnBundleIdentifier(void) {
	NSString *identifier = [NSBundle mainBundle].bundleIdentifier;
	if (identifier == nil) return NULL;
	return strdup(identifier.UTF8String);
}
*/
import "C"

import "unsafe"

// Trusted reports whether this process may read other applications' UI. Without
// it neither route works, so the settings page needs to say so plainly.
func Trusted() bool { return C.AXIsProcessTrustedWithOptions(C.CFDictionaryRef(0)) != 0 }

// RequestTrust shows the system prompt and returns the current state. A newly
// granted permission only applies after relaunch.
func RequestTrust() bool { return C.PromptTrust() != 0 }

// SelectedText asks the accessibility API directly. ok=false means the answer is
// unknown — untrusted, or an app that exposes no selection — not "empty".
func SelectedText() (string, bool) {
	state := C.int(0)
	raw := C.AXSelectedText(&state)
	if raw == nil {
		return "", false
	}
	text := C.GoString(raw)
	C.free(unsafe.Pointer(raw))
	if state != 0 {
		return "", false
	}
	return text, true
}

// Copy synthesizes cmd+C in the frontmost application.
func Copy() error {
	C.SendCopy()
	return nil
}

// Paste synthesizes cmd+V in the frontmost application.
func Paste() error {
	C.SendPaste()
	return nil
}

// ChangeCount is the pasteboard's monotonic revision, or -1 if unavailable.
func ChangeCount() int64 { return int64(C.PasteboardCount()) }

// Pollable reports that a selection can be read without side effects, which is
// what makes 自动划词 tolerable as a background loop.
const Pollable = true

// ownBundle is resolved once: the main bundle does not change while the process
// lives, and the comparison runs on every poll.
var ownBundle = func() string { return take(C.OwnBundleIdentifier()) }()

// FrontmostIsSelf is true while Transdict itself holds the keyboard, so the
// watcher ignores highlights made in its own results.
func FrontmostIsSelf() bool {
	if ownBundle == "" {
		return false
	}
	return take(C.BundleIdentifierOfFrontmost()) == ownBundle
}

// take copies a malloc'd C string and frees it, returning "" for NULL.
func take(raw *C.char) string {
	if raw == nil {
		return ""
	}
	defer C.free(unsafe.Pointer(raw))
	return C.GoString(raw)
}
