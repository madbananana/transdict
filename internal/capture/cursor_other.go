//go:build !darwin && !windows

package capture

import "image"

// Cursor is unavailable here; callers fall back to centering the panel.
func Cursor() image.Point { return image.Point{} }
