//go:build darwin

package capture

/*
#cgo LDFLAGS: -framework CoreGraphics
#include <CoreGraphics/CoreGraphics.h>
*/
import "C"
import "image"

// ScreenAccess reports whether the app may read other applications' windows.
// Without it, screencapture still succeeds but silently returns a picture
// containing only the wallpaper and our own windows, so OCR would run on a
// nearly blank image.
func ScreenAccess() bool { return bool(C.CGPreflightScreenCaptureAccess()) }

// RequestScreenAccess shows the system prompt the first time and returns the
// current state. A newly granted permission only applies after relaunch.
func RequestScreenAccess() bool { return bool(C.CGRequestScreenCaptureAccess()) }

// Cursor is the pointer in global display coordinates, origin top-left. The
// floating panel is placed next to it because that is where a selection just
// happened.
func Cursor() image.Point {
	p := C.CGEventGetLocation(C.CGEventCreate(C.CGEventSourceRef(0)))
	return image.Point{X: int(p.x), Y: int(p.y)}
}
