//go:build darwin

package gui

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework CoreGraphics -framework CoreFoundation

#include <CoreGraphics/CoreGraphics.h>
#include <unistd.h>

void guiIconMouseDown(double x, double y, int ownWindow);
void guiIconMouseMoved(double x, double y);
void guiIconScrolled(double delta);
void guiIconKeyDown(int keyCode, unsigned long long flags);

static CFMachPortRef guiIconTap = NULL;
// guiIconTapActive is the service's intent, kept separately: a tap that timed
// out must be re-enabled only while the button is still on screen, not after it
// was already withdrawn.
static int guiIconTapActive = 0;

// guiPointOverOwnWindow answers whether the click landed on one of this app's
// windows. It looks for a window of ours under the point instead of naming the
// frontmost window there: the desktop wallpaper is a Dock-owned window that
// CGWindowListCopyWindowInfo reports ahead of everything else, so "first window
// containing the point" would call every click elsewhere.
static int guiPointOverOwnWindow(CGPoint p) {
	CFArrayRef list = CGWindowListCopyWindowInfo(kCGWindowListOptionOnScreenOnly, kCGNullWindowID);
	if (list == NULL) {
		return 0;
	}
	pid_t self = getpid();
	int hit = 0;
	CFIndex count = CFArrayGetCount(list);
	for (CFIndex i = 0; i < count; i++) {
		CFDictionaryRef d = CFArrayGetValueAtIndex(list, i);
		CFNumberRef owner = CFDictionaryGetValue(d, kCGWindowOwnerPID);
		pid_t pid = 0;
		if (owner == NULL || !CFNumberGetValue(owner, kCFNumberIntType, &pid) || pid != self) {
			continue;
		}
		CGRect b = CGRectZero;
		CFDictionaryRef bounds = CFDictionaryGetValue(d, kCGWindowBounds);
		if (bounds == NULL || !CGRectMakeWithDictionaryRepresentation(bounds, &b)) {
			continue;
		}
		if (CGRectContainsPoint(b, p)) {
			hit = 1;
			break;
		}
	}
	CFRelease(list);
	return hit;
}

// guiIconTapCallback listens without consuming: the click still reaches the
// application it was aimed at, which is why a listen-only tap is right here.
// The pointer, the wheel and the keyboard all say the same thing as a click —
// the user has moved on — so the button hears them too, the way EasyDict's
// event monitor does while its pop button is visible.
static CGEventRef guiIconTapCallback(CGEventTapProxy proxy, CGEventType type, CGEventRef event, void *info) {
	if (type == kCGEventTapDisabledByTimeout || type == kCGEventTapDisabledByUserInput) {
		if (guiIconTap != NULL && guiIconTapActive) {
			CGEventTapEnable(guiIconTap, true);
		}
		return event;
	}
	switch (type) {
	case kCGEventLeftMouseDown:
	case kCGEventRightMouseDown: {
		CGPoint p = CGEventGetLocation(event);
		guiIconMouseDown(p.x, p.y, guiPointOverOwnWindow(p));
		break;
	}
	case kCGEventMouseMoved: {
		CGPoint p = CGEventGetLocation(event);
		guiIconMouseMoved(p.x, p.y);
		break;
	}
	case kCGEventScrollWheel: {
		// Point delta matches the wheel units EasyDict adds up out of
		// NSEvent.scrollingDeltaY.
		int64_t d = CGEventGetIntegerValueField(event, kCGScrollWheelEventPointDeltaAxis1);
		guiIconScrolled((double)d);
		break;
	}
	case kCGEventKeyDown: {
		guiIconKeyDown((int)CGEventGetIntegerValueField(event, kCGKeyboardEventKeycode),
		               (unsigned long long)CGEventGetFlags(event));
		break;
	}
	default:
		break;
	}
	return event;
}

// guiStartIconTap brings the tap up for the button's lifetime and answers
// whether it is listening. A NULL port means 辅助功能 is not granted, and the
// caller has to fall back to expiring the button on a timer.
static int guiStartIconTap(void) {
	if (guiIconTap == NULL) {
		CGEventMask mask = CGEventMaskBit(kCGEventLeftMouseDown) | CGEventMaskBit(kCGEventRightMouseDown) |
		                   CGEventMaskBit(kCGEventMouseMoved) | CGEventMaskBit(kCGEventScrollWheel) |
		                   CGEventMaskBit(kCGEventKeyDown);
		guiIconTap = CGEventTapCreate(kCGSessionEventTap, kCGHeadInsertEventTap,
		                              kCGEventTapOptionListenOnly, mask, guiIconTapCallback, NULL);
		if (guiIconTap == NULL) {
			return 0;
		}
		CFRunLoopSourceRef src = CFMachPortCreateRunLoopSource(kCFAllocatorDefault, guiIconTap, 0);
		CFRunLoopAddSource(CFRunLoopGetMain(), src, kCFRunLoopCommonModes);
		CFRelease(src);
	}
	guiIconTapActive = 1;
	CGEventTapEnable(guiIconTap, true);
	return 1;
}

// guiStopIconTap keeps the port but stops listening, so no mouse move costs a
// callback while no button is on screen. Re-enabling is cheap next time.
static void guiStopIconTap(void) {
	guiIconTapActive = 0;
	if (guiIconTap != NULL) {
		CGEventTapEnable(guiIconTap, false);
	}
}
*/
import "C"

import "sync"

// iconTapSink is the service whose 划词查询图标 the tap dismisses. The callback
// arrives on the tap's run loop, so the pointer is guarded like the service's
// own fields.
var iconTapSink struct {
	sync.Mutex
	svc *Service
}

// startIconTap makes the button answer the pointer, the wheel and the keyboard.
// It reports false when the tap cannot be created, which is the caller's cue to
// arm the lifetime timer instead.
func startIconTap(s *Service) bool {
	iconTapSink.Lock()
	iconTapSink.svc = s
	iconTapSink.Unlock()
	return C.guiStartIconTap() != 0
}

// stopIconTap silences the tap while the button is away.
func stopIconTap() { C.guiStopIconTap() }

//export guiIconMouseDown
func guiIconMouseDown(x C.double, y C.double, ownWindow C.int) {
	iconTapSink.Lock()
	s := iconTapSink.svc
	iconTapSink.Unlock()
	if s != nil {
		s.iconClickedAt(float64(x), float64(y), ownWindow != 0)
	}
}

//export guiIconMouseMoved
func guiIconMouseMoved(x C.double, y C.double) {
	iconTapSink.Lock()
	s := iconTapSink.svc
	iconTapSink.Unlock()
	if s != nil {
		s.iconMovedTo(float64(x), float64(y))
	}
}

//export guiIconScrolled
func guiIconScrolled(delta C.double) {
	iconTapSink.Lock()
	s := iconTapSink.svc
	iconTapSink.Unlock()
	if s != nil {
		s.iconScrolled(float64(delta))
	}
}

//export guiIconKeyDown
func guiIconKeyDown(keyCode C.int, flags C.ulonglong) {
	iconTapSink.Lock()
	s := iconTapSink.svc
	iconTapSink.Unlock()
	if s != nil {
		s.iconKeyDown(int(keyCode), uint64(flags))
	}
}
