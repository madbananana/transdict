//go:build !darwin || ios

package gui

import (
	"github.com/wailsapp/wails/v3/pkg/application"
)

// The white webview background is an AppKit detail; other platforms paint the
// configured window colour from the start.
func paintOverlayBackdrop(*application.WebviewWindow) {}
