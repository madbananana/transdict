//go:build darwin

package gui

import (
	"transdict/internal/capture"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// liveOverlay is EasyDict's flow: a transparent marquee over the running screen,
// with the pixels read only after the selection is released. Freezing first
// would put a full-display capture and encode in front of the shortcut.
const liveOverlay = true

// placeOverlay pins a selection window to its display exactly. AppKit keeps a
// window's origin when its size changes, and Wails flips the Y with the frame
// height, so the size has to land before the position or the top edge drifts by
// the difference.
func placeOverlay(win *application.WebviewWindow, bounds application.Rect) {
	win.SetSize(bounds.Width, bounds.Height)
	win.SetPosition(bounds.X, bounds.Y)
}

// captureLiveRegion reads the selection off the running screen once the marquee
// is gone. The rectangle is in the display's points, which is what
// CGDisplayCreateImageForRect takes.
func captureLiveRegion(_ *Service, frameIndex, x, y, w, h int) ([]byte, error) {
	return capture.Region(frameIndex, x, y, w, h)
}
