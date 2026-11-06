package capture

import (
	"bytes"
	"image"
	"image/jpeg"
	_ "image/png"
)

// Frame is one frozen display image in native pixel resolution.
type Frame struct {
	Index  int    `json:"index"`
	PNG    []byte `json:"-"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

// Displays captures every active display, frozen at native resolution.
func Displays() ([]Frame, error) {
	return captureDisplays()
}

// ForDisplay re-encodes a captured frame for the selection overlay. The frozen
// screenshot is a lossless PNG close to 3 MB at Retina size, and the overlay
// cannot be dragged until it has been decoded; the same frame as a JPEG is an
// order of magnitude smaller and the difference is invisible under the dim
// layer. OCR keeps reading the original pixels, so this copy is display-only.
func ForDisplay(png []byte) ([]byte, error) {
	img, _, err := image.Decode(bytes.NewReader(png))
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	out.Grow(len(png) / 4)
	if err := jpeg.Encode(&out, img, &jpeg.Options{Quality: 85}); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
