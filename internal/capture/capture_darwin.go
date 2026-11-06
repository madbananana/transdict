//go:build darwin

package capture

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	"github.com/kbinani/screenshot"
)

// captureDisplays uses the system screencapture tool because the portable
// capture path below returns images at backing-store *point* resolution on
// Retina displays, which halves the pixel density fed to OCR.
func captureDisplays() ([]Frame, error) {
	n := screenshot.NumActiveDisplays()
	if n == 0 {
		return nil, errors.New("capture: no active displays")
	}
	frames := make([]Frame, 0, n)
	for i := 0; i < n; i++ {
		f, err := captureNative(i)
		if err != nil {
			f, err = captureViaLib(i)
			if err != nil {
				return nil, err
			}
		}
		frames = append(frames, f)
	}
	return frames, nil
}

func captureNative(idx int) (Frame, error) {
	path := filepath.Join(os.TempDir(), fmt.Sprintf("transdict-cap-%d-%d.png", os.Getpid(), idx))
	defer os.Remove(path)

	cmd := exec.Command("/usr/sbin/screencapture", "-x", "-t", "png", "-D", strconv.Itoa(idx+1), path)
	if out, err := cmd.CombinedOutput(); err != nil {
		return Frame{}, fmt.Errorf("screencapture display %d: %w: %s", idx, err, out)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Frame{}, err
	}
	return frameFromPNG(idx, data)
}
