//go:build windows

package capture

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// ClipboardImage returns the picture on the clipboard as PNG. Win32 DIB
// handling would mean a cgo bridge, so the pasteboard is read through
// Windows.Forms the same way the OCR side reaches WinRT: the Go build stays
// pure and PowerShell turns the clipboard image into a PNG file.
func ClipboardImage() ([]byte, error) {
	dir, err := os.MkdirTemp("", "transdict-clip-")
	if err != nil {
		return nil, fmt.Errorf("Windows 剪贴板：无法创建临时目录：%w", err)
	}
	defer os.RemoveAll(dir)

	scriptPath := filepath.Join(dir, "clip.ps1")
	// The BOM keeps Windows PowerShell's Chinese-capable decode consistent with
	// the OCR script, which the same interpreter runs.
	if err := os.WriteFile(scriptPath, []byte("\ufeff"+clipboardPowerShell), 0o600); err != nil {
		return nil, fmt.Errorf("Windows 剪贴板：无法写入读取脚本：%w", err)
	}
	outPath := filepath.Join(dir, "clipboard.png")

	// -STA matters: Windows.Forms clipboard calls fail from an MTA host, and
	// the OCR script's plain invocation would leave this one in MTA.
	cmd := exec.Command(powershellExe(), "-NoProfile", "-NonInteractive", "-STA",
		"-ExecutionPolicy", "Bypass", "-File", scriptPath, "-OutPath", outPath)
	// The app is a GUI process; without this every clipboard OCR would flash a
	// console window over the result.
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if strings.TrimSpace(stdout.String()) == clipboardNoImage {
			return nil, errors.New("剪贴板里没有图片")
		}
		reason := strings.TrimSpace(stderr.String())
		if reason == "" {
			reason = err.Error()
		}
		return nil, fmt.Errorf("Windows 剪贴板读取失败：%s", reason)
	}
	png, err := os.ReadFile(outPath)
	if err != nil || len(png) == 0 {
		return nil, errors.New("剪贴板里没有图片")
	}
	return png, nil
}

// powershellExe finds Windows PowerShell; a GUI launch can inherit a PATH
// without it, so the absolute copy under System32 is the fallback.
func powershellExe() string {
	if _, err := exec.LookPath("powershell.exe"); err == nil {
		return "powershell.exe"
	}
	root := os.Getenv("SystemRoot")
	if root == "" {
		root = filepath.Join("C:", "Windows")
	}
	candidate := filepath.Join(root, "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}
	return "powershell.exe"
}

// clipboardNoImage is the script's marker for "the clipboard holds no picture".
// It is ASCII so the console code page cannot mangle it.
const clipboardNoImage = "NOIMAGE"

// clipboardPowerShell saves the clipboard bitmap as PNG. GetImage returns null
// both for an empty clipboard and for one whose owner is mid-copy, so a failed
// call is retried briefly before it counts as "no picture".
const clipboardPowerShell = `
param(
  [Parameter(Mandatory=$true)][string]$OutPath
)
Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing

$image = $null
for ($i = 0; $i -lt 10; $i++) {
  try {
    $image = [System.Windows.Forms.Clipboard]::GetImage()
    break
  } catch {
    Start-Sleep -Milliseconds 50
  }
}

if ($null -eq $image) {
  [Console]::Out.Write('NOIMAGE')
  exit 2
}

try {
  $image.Save($OutPath, [System.Drawing.Imaging.ImageFormat]::Png)
} finally {
  $image.Dispose()
}

if (-not (Test-Path -LiteralPath $OutPath)) {
  [Console]::Out.Write('NOIMAGE')
  exit 2
}
`
