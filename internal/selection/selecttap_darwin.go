//go:build darwin

package selection

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework CoreGraphics -framework CoreFoundation
#include <stdlib.h>
#include <CoreGraphics/CoreGraphics.h>

void guiSelectMouse(int kind, double x, double y, int clicks, unsigned long long flags);
void guiSelectDragged(void);
void guiSelectKey(int keyCode, unsigned long long flags, int repeat);

static CFMachPortRef guiSelectTap = NULL;
// guiSelectTapWanted is the watcher's intent, kept apart so a tap that timed out
// is re-enabled only while 自动划词 is still on.
static int guiSelectTapWanted = 0;

// guiSelectTapCallback listens without consuming: the drag still selects text in
// the application it was aimed at. A listen-only tap is the point — EasyDict
// watches the same events and reads the selection with the accessibility API
// afterwards, which is why nothing here synthesizes a copy.
static CGEventRef guiSelectTapCallback(CGEventTapProxy proxy, CGEventType type, CGEventRef event, void *info) {
	if (type == kCGEventTapDisabledByTimeout || type == kCGEventTapDisabledByUserInput) {
		if (guiSelectTap != NULL && guiSelectTapWanted) {
			CGEventTapEnable(guiSelectTap, true);
		}
		return event;
	}
	switch (type) {
	case kCGEventLeftMouseDown:
	case kCGEventLeftMouseUp:
		;
		CGPoint p = CGEventGetLocation(event);
		int clicks = (int)CGEventGetIntegerValueField(event, kCGMouseEventClickState);
		int kind = type == kCGEventLeftMouseDown ? 0 : 1;
		guiSelectMouse(kind, p.x, p.y, clicks, (unsigned long long)CGEventGetFlags(event));
		break;
	case kCGEventLeftMouseDragged:
		guiSelectDragged();
		break;
	case kCGEventKeyDown:
		guiSelectKey((int)CGEventGetIntegerValueField(event, kCGKeyboardEventKeycode),
		             (unsigned long long)CGEventGetFlags(event),
		             (int)CGEventGetIntegerValueField(event, kCGKeyboardEventAutorepeat));
		break;
	default:
		break;
	}
	return event;
}

// guiStartSelectTap begins watching the pointer for selection gestures. A NULL
// port means 辅助功能 is not granted; the caller falls back to polling.
static int guiStartSelectTap(void) {
	if (guiSelectTap == NULL) {
		CGEventMask mask = CGEventMaskBit(kCGEventLeftMouseDown) | CGEventMaskBit(kCGEventLeftMouseUp) |
		                   CGEventMaskBit(kCGEventLeftMouseDragged) | CGEventMaskBit(kCGEventKeyDown);
		guiSelectTap = CGEventTapCreate(kCGSessionEventTap, kCGHeadInsertEventTap,
		                                kCGEventTapOptionListenOnly, mask, guiSelectTapCallback, NULL);
		if (guiSelectTap == NULL) {
			return 0;
		}
		CFRunLoopSourceRef src = CFMachPortCreateRunLoopSource(kCFAllocatorDefault, guiSelectTap, 0);
		CFRunLoopAddSource(CFRunLoopGetMain(), src, kCFRunLoopCommonModes);
		CFRelease(src);
	}
	guiSelectTapWanted = 1;
	CGEventTapEnable(guiSelectTap, true);
	return 1;
}

// guiStopSelectTap keeps the port and stops listening, so 自动划词 off costs no
// callbacks at all.
static void guiStopSelectTap(void) {
	guiSelectTapWanted = 0;
	if (guiSelectTap != NULL) {
		CGEventTapEnable(guiSelectTap, false);
	}
}
*/
import "C"

import (
	"context"
	"log"
	"sync"
)

// selectTapSink is the watcher the tap reports to. The callbacks arrive on the
// tap's run loop, so the pointer is guarded like every other shared field.
var selectTapSink struct {
	sync.Mutex
	w *pointerWatch
}

func selectTapSinkSet(w *pointerWatch) {
	selectTapSink.Lock()
	selectTapSink.w = w
	selectTapSink.Unlock()
}

func selectTapSinkGet() *pointerWatch {
	selectTapSink.Lock()
	w := selectTapSink.w
	selectTapSink.Unlock()
	return w
}

func selectTapSinkClear(w *pointerWatch) {
	selectTapSink.Lock()
	if selectTapSink.w == w {
		selectTapSink.w = nil
	}
	selectTapSink.Unlock()
}

// WatchPointer reports each selection as the gesture that made it ends: the
// release of a drag, the click that selected a word or a paragraph, a
// shift-click extending a selection, ⌘A in a field that takes typing. Reading on
// the gesture is what makes 自动划词 feel immediate, and it is the only way to
// know the user is done dragging — polling can only guess at it from a stable
// selection, which both lags behind the release and can fire mid-drag.
//
// ErrUnsupported means the event tap could not be created; the caller should
// fall back to Watch, the polling route.
func WatchPointer(ctx context.Context, report func(string)) error {
	w := newPointerWatch(ctx, report)
	selectTapSinkSet(w)
	if C.guiStartSelectTap() == 0 {
		selectTapSinkClear(w)
		return ErrUnsupported
	}
	log.Print("transdict: 自动划词: 事件监听已启动")
	<-ctx.Done()
	selectTapSinkClear(w)
	C.guiStopSelectTap()
	return nil
}

//export guiSelectMouse
func guiSelectMouse(kind C.int, x C.double, y C.double, clicks C.int, flags C.ulonglong) {
	w := selectTapSinkGet()
	if w == nil {
		return
	}
	shift := uint64(flags)&pointerFlagShift != 0
	switch int(kind) {
	case 0:
		w.press(int(clicks), shift)
	case 1:
		w.release()
	}
}

//export guiSelectDragged
func guiSelectDragged() {
	if w := selectTapSinkGet(); w != nil {
		w.dragged()
	}
}

//export guiSelectKey
func guiSelectKey(keyCode C.int, flags C.ulonglong, repeat C.int) {
	if repeat != 0 {
		return
	}
	if !isSelectAll(int(keyCode), uint64(flags)) {
		return
	}
	// ⌘A on a page selects the reading text, not what the user wrote; EasyDict
	// only takes it as 取词 where the focused element accepts typing.
	if !FocusedElementEditable() {
		return
	}
	if w := selectTapSinkGet(); w != nil {
		w.fire(selectAll)
	}
}
