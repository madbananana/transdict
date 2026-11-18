//go:build !darwin || !cgo

package capture

import "errors"

// Region needs the native display read; without it the caller falls back to
// capturing every display and cropping the rectangle out.
func Region(index, x, y, w, h int) ([]byte, error) {
	return nil, errors.New("capture: region capture is unavailable")
}
