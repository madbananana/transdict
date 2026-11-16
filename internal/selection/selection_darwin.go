//go:build darwin

package selection

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework ApplicationServices -framework AppKit
#include <stdlib.h>
#include <unistd.h>
#include <ApplicationServices/ApplicationServices.h>
#import <AppKit/AppKit.h>

// CGEventKeyCodes live in IOKit/hidsystem/ev_keymap.h, which ApplicationServices
// does not pull in; the numbers are fixed by the HID spec.
enum { kVK_Command = 55, kVK_ANSI_A = 0, kVK_ANSI_C = 8, kVK_ANSI_V = 9 };

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

// frontmostPid names the process that owns the keyboard right now; 0 when the
// workspace has no frontmost application.
static pid_t frontmostPid(void) {
	NSRunningApplication *app = [NSWorkspace sharedWorkspace].frontmostApplication;
	if (app == nil) return 0;
	return app.processIdentifier;
}

// AXSelectedText reads the focused UI element of the frontmost application.
//
// state: 0 answered (possibly an empty selection, which is a real answer),
// 1 this process is not trusted for accessibility, 2 the target app exposes no
// selectable element. 1 and 2 both mean "use the clipboard route".
//
// The application is named by NSWorkspace rather than by the system-wide
// kAXFocusedApplication attribute. A non-activating panel of ours — the float,
// the 划词查询图标 — can become the accessibility server's idea of the focused
// application while another app is still the one the user is reading in. Naming
// the frontmost pid is the same rule 复制 and 替换 follow, and it keeps our own
// windows from blinding the 自动划词 watcher.
static char *AXSelectedText(int *state) {
	*state = 2;
	if (!AXIsProcessTrustedWithOptions(NULL)) { *state = 1; return NULL; }

	pid_t pid = frontmostPid();
	if (pid <= 0) return NULL;
	AXUIElementRef app = AXUIElementCreateApplication(pid);
	if (app == NULL) return NULL;
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

	char *out = cfToUTF8(value);
	if (value != NULL) CFRelease(value);
	if (out == NULL && *state == 0) {
		// The attribute read succeeded but carried no text. An empty selection is
		// still an answer: 自动划词 needs it to forget what it last saw, so the
		// same words highlighted again are reported again.
		out = strdup("");
	}
	return out;
}

// AXFocusedValueSettable answers whether the focused element accepts typing.
// ⌘A is a selection gesture only there: on a page it selects the reading text,
// and 自动划词 would then ask about the whole page instead of what the user wrote.
static int AXFocusedValueSettable(void) {
	if (!AXIsProcessTrustedWithOptions(NULL)) return 0;
	pid_t pid = frontmostPid();
	if (pid <= 0) return 0;
	AXUIElementRef app = AXUIElementCreateApplication(pid);
	if (app == NULL) return 0;
	int settable = 0;
	AXUIElementRef element = NULL;
	if (AXUIElementCopyAttributeValue(app, kAXFocusedUIElementAttribute,
			(CFTypeRef *)&element) == kAXErrorSuccess && element != NULL) {
		Boolean flag = false;
		if (AXUIElementIsAttributeSettable(element, kAXValueAttribute, &flag) == kAXErrorSuccess && flag) {
			settable = 1;
		}
		CFRelease(element);
	}
	CFRelease(app);
	return settable;
}

// postKey posts one key event with the chord's modifiers carried on the event
// itself. The modifier is not sent as separate key-down events: that relies on
// the hardware modifier state, and a remote session with a stale latch turns
// cmd+V into option+V. Flags on the event are what the target actually reads.
//
// The event goes to the frontmost process rather than the HID tap: a background
// helper (this app has no Dock tile) is not the active application, and a
// session driven over remote input can leave the HID tap without a target. The
// pid route delivers the keystroke to the window that holds the selection.
static void postKey(CGKeyCode key, CGEventFlags flags, bool down) {
	CGEventRef event = CGEventCreateKeyboardEvent(NULL, key, down);
	if (event == NULL) return;
	CGEventSetFlags(event, flags);
	pid_t pid = frontmostPid();
	if (pid > 0) {
		CGEventPostToPid(pid, event);
	} else {
		CGEventPost(kCGHIDEventTap, event);
	}
	CFRelease(event);
}

// SendCopy is the fallback every application understands: cmd+C into whatever
// currently has keyboard focus.
static void SendCopy(void) {
	postKey(kVK_ANSI_C, kCGEventFlagMaskCommand, true);
	postKey(kVK_ANSI_C, kCGEventFlagMaskCommand, false);
}

// SendPaste is the other half of 翻译并替换: the translation is placed on the
// pasteboard and typed into the field the selection came from.
static void SendPaste(void) {
	postKey(kVK_ANSI_V, kCGEventFlagMaskCommand, true);
	postKey(kVK_ANSI_V, kCGEventFlagMaskCommand, false);
}

// SendSelectAll is cmd+A, which 自动选中所有文本 fires when the field had no
// highlight: replacing an empty selection would otherwise paste nowhere.
static void SendSelectAll(void) {
	postKey(kVK_ANSI_A, kCGEventFlagMaskCommand, true);
	postKey(kVK_ANSI_A, kCGEventFlagMaskCommand, false);
}

static long long PasteboardCount(void) {
	return (long long)[[NSPasteboard generalPasteboard] changeCount];
}

// AXInsertText replaces the focused element's selection with the given text.
// This is the route 兼容模式替换 takes first: it needs no keystrokes, so it also
// works in apps that swallow synthetic key events, and replacing the selection
// is exactly what a paste into a highlighted field does.
//
// answers "ok", "untrusted", "noapp" (no focused element) or "setfail".
static char *AXInsertText(const char *utf8) {
	if (!AXIsProcessTrustedWithOptions(NULL)) return strdup("untrusted");
	CFStringRef text = CFStringCreateWithCString(NULL, utf8, kCFStringEncodingUTF8);
	if (text == NULL) return strdup("setfail");
	AXUIElementRef system = AXUIElementCreateSystemWide();
	AXUIElementRef app = NULL;
	AXUIElementRef element = NULL;
	char *result = strdup("noapp");
	if (AXUIElementCopyAttributeValue(system, kAXFocusedApplicationAttribute,
			(CFTypeRef *)&app) == kAXErrorSuccess && app != NULL) {
		if (AXUIElementCopyAttributeValue(app, kAXFocusedUIElementAttribute,
				(CFTypeRef *)&element) == kAXErrorSuccess && element != NULL) {
			free(result);
			result = strdup(AXUIElementSetAttributeValue(element, kAXSelectedTextAttribute, text) == kAXErrorSuccess
				? "ok" : "setfail");
			CFRelease(element);
		}
		CFRelease(app);
	}
	CFRelease(system);
	CFRelease(text);
	return result;
}

// pressCopyIn walks the menu tree looking for the Copy item: it is identified by
// its command character, so the search works whatever the interface language is.
// A menu item cannot be pressed while its menu is closed — the action succeeds
// but nothing happens — so the menu bar item that owns it is opened first.
// Returns 0 once the item was pressed.
static int pressCopyIn(AXUIElementRef element, int depth, AXUIElementRef opener) {
	if (depth > 4) return 1;
	CFArrayRef children = NULL;
	if (AXUIElementCopyAttributeValue(element, kAXChildrenAttribute,
			(CFTypeRef *)&children) != kAXErrorSuccess || children == NULL) {
		return 1;
	}
	int result = 1;
	for (CFIndex i = 0; i < CFArrayGetCount(children); i++) {
		AXUIElementRef child = (AXUIElementRef)CFArrayGetValueAtIndex(children, i);
		CFTypeRef cmdChar = NULL;
		int modifiers = -1;
		Boolean copy = false;
		if (AXUIElementCopyAttributeValue(child, CFSTR("AXMenuItemCmdChar"), &cmdChar) == kAXErrorSuccess && cmdChar != NULL) {
			if (CFGetTypeID(cmdChar) == CFStringGetTypeID()) {
				copy = CFStringCompare((CFStringRef)cmdChar, CFSTR("c"), kCFCompareCaseInsensitive) == kCFCompareEqualTo;
			}
			CFRelease(cmdChar);
		}
		CFTypeRef modValue = NULL;
		if (AXUIElementCopyAttributeValue(child, CFSTR("AXMenuItemCmdModifiers"), &modValue) == kAXErrorSuccess && modValue != NULL) {
			if (CFGetTypeID(modValue) == CFNumberGetTypeID()) {
				CFNumberGetValue((CFNumberRef)modValue, kCFNumberIntType, &modifiers);
			}
			CFRelease(modValue);
		}
		// Plain cmd+C only: Copy Style is cmd+opt+C and some transforms ride on
		// cmd+shift+C; running those instead of Copy would be worse than nothing.
		copy = copy && modifiers == 0;
		if (copy) {
			if (opener != NULL) {
				AXUIElementPerformAction(opener, kAXPressAction);
				usleep(150 * 1000);
			}
			AXError err = AXUIElementPerformAction(child, kAXPressAction);
			usleep(80 * 1000);
			if (err != kAXErrorSuccess && opener != NULL) {
				AXUIElementPerformAction(opener, kAXPressAction);
			}
			if (err == kAXErrorSuccess) {
				result = 0;
				break;
			}
		}
		AXUIElementRef nextOpener = opener;
		if (depth == 0) nextOpener = child;
		if (pressCopyIn(child, depth + 1, nextOpener) == 0) {
			result = 0;
			break;
		}
	}
	CFRelease(children);
	return result;
}

// PressCopyMenuItem runs the frontmost app's own Copy command, which is
// 强制取词's 菜单栏动作复制. Apps that ignore a synthesized cmd+C still route
// this one to their editor. Returns 0 when an item was pressed.
static int PressCopyMenuItem(void) {
	if (!AXIsProcessTrustedWithOptions(NULL)) return 1;
	AXUIElementRef system = AXUIElementCreateSystemWide();
	AXUIElementRef app = NULL;
	if (AXUIElementCopyAttributeValue(system, kAXFocusedApplicationAttribute,
			(CFTypeRef *)&app) != kAXErrorSuccess || app == NULL) {
		CFRelease(system);
		return 1;
	}
	int result = 1;
	AXUIElementRef menuBar = NULL;
	if (AXUIElementCopyAttributeValue(app, kAXMenuBarAttribute,
			(CFTypeRef *)&menuBar) == kAXErrorSuccess && menuBar != NULL) {
		result = pressCopyIn(menuBar, 0, NULL);
		CFRelease(menuBar);
	}
	CFRelease(app);
	CFRelease(system);
	return result;
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

// lastOtherBundleID names the application the user was reading in before a
// click on one of our own windows took the keyboard. That application, not
// whatever is frontmost when 划词翻译 is pressed, is where the selection lives.
static NSString *lastOtherBundleID = nil;
static id activationObserver = nil;

static void RememberOtherApplications(void) {
	if (activationObserver != nil) return;
	NSNotificationCenter *center = [[NSWorkspace sharedWorkspace] notificationCenter];
	activationObserver = [center addObserverForName:NSWorkspaceDidActivateApplicationNotification
		object:nil queue:[NSOperationQueue mainQueue]
		usingBlock:^(NSNotification *note) {
			NSRunningApplication *app = note.userInfo[NSWorkspaceApplicationKey];
			NSString *identifier = app.bundleIdentifier;
			NSString *own = [NSBundle mainBundle].bundleIdentifier;
			if (identifier == nil || (own != nil && [identifier isEqualToString:own])) return;
			lastOtherBundleID = [identifier copy];
		}];
}

static Boolean ReactivateLastOtherApplication(void) {
	if (lastOtherBundleID == nil) return false;
	NSRunningApplication *app = [NSRunningApplication
		runningApplicationsWithBundleIdentifier:lastOtherBundleID].firstObject;
	if (app == nil) return false;
#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Wdeprecated-declarations"
	return [app activateWithOptions:NSApplicationActivateIgnoringOtherApps];
#pragma clang diagnostic pop
}
*/
import "C"

import (
	"errors"
	"unsafe"
)

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

// CopyMenu runs the frontmost application's own Copy menu command — 强制取词's
// 菜单栏动作复制.
func CopyMenu() error {
	if C.PressCopyMenuItem() == 0 {
		return nil
	}
	return errors.New("取词：没有可用的拷贝菜单项")
}

// Paste synthesizes cmd+V in the frontmost application.
func Paste() error {
	C.SendPaste()
	return nil
}

// SelectAll synthesizes cmd+A in the frontmost application.
func SelectAll() error {
	C.SendSelectAll()
	return nil
}

// TypeText writes the answer into the focused element instead of pasting it,
// which is 兼容模式替换. The accessibility route is the one that works in apps
// that ignore cmd+V; a refusal is reported so the caller can paste instead.
func TypeText(text string) error {
	if text == "" {
		return nil
	}
	raw := C.CString(text)
	defer C.free(unsafe.Pointer(raw))
	switch take(C.AXInsertText(raw)) {
	case "ok":
		return nil
	case "untrusted":
		return errors.New("替换：没有辅助功能权限")
	default:
		return errors.New("replace: the focused element refused inserted text")
	}
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

// FrontmostBundleID names the application holding the keyboard. It is how a
// selection taken from Apple Books is told apart from the same text copied
// anywhere else: only Books appends the copyright notice that gets removed.
func FrontmostBundleID() string { return take(C.BundleIdentifierOfFrontmost()) }

// FocusedElementEditable reports whether the frontmost application's focused
// element accepts typing, which is where ⌘A can mean 取词.
func FocusedElementEditable() bool { return C.AXFocusedValueSettable() != 0 }

// TrackFrontmostApps starts recording which application held the keyboard last,
// so a 划词 fired from our own window can hand it back. Without this the
// accessibility read would ask our own window and come back empty.
func TrackFrontmostApps() { C.RememberOtherApplications() }

// ReactivateLastOther gives the keyboard back to the application the user was
// reading in. It reports false when there is nothing to go back to, in which
// case the caller has to step aside instead.
func ReactivateLastOther() bool { return C.ReactivateLastOtherApplication() != 0 }

// take copies a malloc'd C string and frees it, returning "" for NULL.
func take(raw *C.char) string {
	if raw == nil {
		return ""
	}
	defer C.free(unsafe.Pointer(raw))
	return C.GoString(raw)
}
