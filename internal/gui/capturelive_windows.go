//go:build windows

package gui

import (
	"errors"
	"image"
	"math"

	"transdict/internal/capture"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// liveOverlay is EasyDict's flow on Windows too: the marquee is a transparent
// sheet over the running screen and the pixels are read only after the release.
// The frozen flow put a full BitBlt, a PNG encode and a WebView2 load between
// the shortcut and the first visible reaction, which is the stall 截图翻译 had.
const liveOverlay = true

// placeOverlay pins a selection window to its display exactly. Windows treats a
// window created at (0,0) as "let the system choose", and the primary display
// starts at (0,0), so the creation options alone cannot place it there. Wails
// also converts DIP to physical against the display nearest the rectangle it is
// given, so the position goes first — only then does a size land on the target
// display's scale factor — and the position is repeated to undo the rounding
// the round trip through physical pixels can introduce.
func placeOverlay(win *application.WebviewWindow, bounds application.Rect) {
	win.SetPosition(bounds.X, bounds.Y)
	win.SetSize(bounds.Width, bounds.Height)
	win.SetPosition(bounds.X, bounds.Y)
}

// captureLiveRegion reads the selection off the running screen once the marquee
// is gone. The overlay is in display points while the desktop read is in
// physical pixels, so the rectangle is scaled by the display's own factor and
// offset into the virtual screen — no full-screen capture, only the selection.
func captureLiveRegion(s *Service, frameIndex, x, y, w, h int) ([]byte, error) {
	screens := s.wails.Screen.GetAll()
	if frameIndex < 0 || frameIndex >= len(screens) {
		return nil, errors.New("capture: no such display")
	}
	dip, phys := screens[frameIndex].Bounds, screens[frameIndex].PhysicalBounds
	if dip.Width <= 0 || phys.Width <= 0 {
		return nil, errors.New("capture: bad display bounds")
	}
	k := float64(phys.Width) / float64(dip.Width)
	rect := image.Rect(
		phys.X+int(math.Round(float64(x)*k)),
		phys.Y+int(math.Round(float64(y)*k)),
		phys.X+int(math.Round(float64(x+w)*k)),
		phys.Y+int(math.Round(float64(y+h)*k)),
	)
	return capture.RegionRect(rect)
}
