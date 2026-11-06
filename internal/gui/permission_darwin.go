//go:build darwin

package gui

import (
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	"transdict/internal/l10n"
)

// Deep links to the two panes that hold the checkboxes. Naming them here is
// what lets a button be right about where the setting lives.
const (
	screenPane        = "x-apple.systempreferences:com.apple.preference.security?Privacy_ScreenCapture"
	accessibilityPane = "x-apple.systempreferences:com.apple.preference.security?Privacy_Accessibility"
)

// offerPermission gives a missing grant somewhere to go. It runs on its own
// goroutine because a message dialog is a modal loop, and the hotkey callback
// that ends up here has to return.
func (s *Service) offerPermission(title, body, pane string) {
	dialog := s.wails.Dialog.Info().SetTitle(title).SetMessage(body)
	dialog.AddButton(l10n.T("打开系统设置")).OnClick(func() {
		if err := s.wails.Browser.OpenURL(pane); err != nil {
			log.Printf("transdict: 打开系统设置失败: %v", err)
		}
	})
	dialog.AddButton(l10n.T("重启 Transdict")).OnClick(func() { go s.relaunch() })
	dialog.AddButton(l10n.T("稍后"))
	dialog.Show()
}

// Restart relaunches the app so macOS re-reads the two grants. A checkbox
// change is only honoured by a fresh process, and the settings page cannot know
// when the user has finished in System Settings, so it offers the step.
func (s *Service) Restart() { go s.relaunch() }

// relaunchScript waits for this process to die before handing the bundle back
// to LaunchServices: the single-instance lock is a file lock, so an `open` that
// arrives while the old copy still lives only re-activates it — and the app
// would then be gone. The wait is bounded so a wedged process cannot hang the
// relaunch forever.
const relaunchScript = `i=0; while kill -0 "$1" 2>/dev/null; do i=$((i+1)); [ "$i" -gt 50 ] && break; sleep 0.1; done; open "$2"`

func (s *Service) relaunch() {
	bin, err := os.Executable()
	if err != nil {
		log.Printf("transdict: 找不到自身路径: %v", err)
		return
	}
	bundle := filepath.Dir(filepath.Dir(filepath.Dir(bin)))
	if filepath.Ext(bundle) != ".app" {
		// A binary run straight from the build directory has no bundle for
		// LaunchServices to take back, so quitting would only lose the app.
		log.Print("transdict: 只有打包成 .app 才能自动重启")
		return
	}
	cmd := exec.Command("sh", "-c", relaunchScript, "transdict-relaunch",
		strconv.Itoa(os.Getpid()), bundle)
	if err := cmd.Start(); err != nil {
		log.Printf("transdict: 重启失败: %v", err)
		return
	}
	go func() { _ = cmd.Wait() }()
	s.wails.Quit()
}
