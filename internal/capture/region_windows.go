//go:build windows

package capture

import (
	"bytes"
	"errors"
	"image"
	"image/png"

	"github.com/kbinani/screenshot"
)

// RegionRect reads one rectangle of the virtual screen, in physical pixels, and
// encodes it as PNG. Only the selected pixels are copied and encoded, so the
// cost follows the selection rather than the display.
func RegionRect(rect image.Rectangle) ([]byte, error) {
	if rect.Empty() {
		return nil, errors.New("capture: empty region")
	}
	img, err := screenshot.CaptureRect(rect)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
