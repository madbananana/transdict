//go:build windows

package update

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// Launch hands the installer to a detached PowerShell that waits for this
// process to exit, runs the NSIS installer silently, and starts the new build.
// Waiting is what makes the overwrite possible: the installer cannot replace an
// executable that is still running. A per-machine installer still raises the
// UAC prompt, which no amount of silence here can avoid.
func Launch(path string) (bool, string, error) {
	exe, err := os.Executable()
	if err != nil {
		return false, "", fmt.Errorf("无法定位当前程序：%w", err)
	}
	script := fmt.Sprintf(`$ErrorActionPreference = 'Stop'
while (Get-Process -Id %d -ErrorAction SilentlyContinue) { Start-Sleep -Milliseconds 200 }
try {
  Start-Process -FilePath '%s' -ArgumentList '/S' -Wait
} catch {
}
Start-Process -FilePath '%s'
`, os.Getpid(), quotePS(path), quotePS(exe))

	cmd := exec.Command(powershell(), "-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden", "-Command", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := cmd.Start(); err != nil {
		return false, "", fmt.Errorf("无法启动安装程序：%w", err)
	}
	_ = cmd.Process.Release()
	return true, "正在安装新版本，应用即将退出；安装完成后会自动重新打开。", nil
}

// quotePS keeps a path with an apostrophe from ending the single-quoted
// PowerShell string it is pasted into.
func quotePS(s string) string { return strings.ReplaceAll(s, "'", "''") }

// powershell resolves the executable that ships with every supported Windows,
// because a GUI launch can inherit a PATH without System32.
func powershell() string {
	if _, err := exec.LookPath("powershell.exe"); err == nil {
		return "powershell.exe"
	}
	root := os.Getenv("SystemRoot")
	if root == "" {
		root = `C:\Windows`
	}
	return filepath.Join(root, "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
}
