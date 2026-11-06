//go:build !darwin

package capture

import (
	"errors"

	"github.com/kbinani/screenshot"
)

// ScreenAccess is always granted where the capture API is a plain BitBlt.
func ScreenAccess() bool        { return true }
func RequestScreenAccess() bool { return true }

func captureDisplays() ([]Frame, error) {
	n := screenshot.NumActiveDisplays()
	if n == 0 {
		return nil, errors.New("capture: no active displays")
	}
	frames := make([]Frame, 0, n)
	for i := 0; i < n; i++ {
		f, err := captureViaLib(i)
		if err != nil {
			return nil, err
		}
		frames = append(frames, f)
	}
	return frames, nil
}
