//go:build windows

package capture

import (
	"image"
	"syscall"
	"unsafe"
)

var (
	user32           = syscall.NewLazyDLL("user32.dll")
	procGetCursorPos = user32.NewProc("GetCursorPos")
)

type winPoint struct{ X, Y int32 }

// Cursor is the pointer in virtual-screen coordinates, which is the same
// top-left origin Wails positions windows from.
func Cursor() image.Point {
	var p winPoint
	if r, _, _ := procGetCursorPos.Call(uintptr(unsafe.Pointer(&p))); r == 0 {
		return image.Point{}
	}
	return image.Point{X: int(p.X), Y: int(p.Y)}
}
