package capture

import (
	"image"

	"github.com/kbinani/screenshot"
)

// Bounds lists the frame of every active display. Unlike Displays it reads no
// pixels, so it is cheap enough to ask while the first window is still being
// placed — Wails' own screen cache is only filled once the app finishes
// launching, which is later than that.
func Bounds() []image.Rectangle {
	n := screenshot.NumActiveDisplays()
	out := make([]image.Rectangle, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, screenshot.GetDisplayBounds(i))
	}
	return out
}
