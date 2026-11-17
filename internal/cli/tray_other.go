//go:build !windows

package cli

import (
	"github.com/wailsapp/wails/v3/pkg/application"

	"transdict/internal/gui"
)

// installTrayClick is Windows-only: the other platforms deliver left clicks to
// the tray API themselves, and a menu bar icon opens its menu on click.
func installTrayClick(*application.SystemTray, *gui.Service) {}
