//go:build !darwin && !windows

package gui

import (
	"errors"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// liveOverlay stays off where the transparent sheet has not been verified: the
// frozen-frame flow works there, and a see-through overlay that paints opaque
// would hide the very screen the user is selecting from.
const liveOverlay = false

func placeOverlay(_ *application.WebviewWindow, _ application.Rect) {}

func captureLiveRegion(_ *Service, _, _, _, _, _ int) ([]byte, error) {
	return nil, errors.New("capture: region capture is unavailable")
}
