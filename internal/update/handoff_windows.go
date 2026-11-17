//go:build windows

package update

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
)

// Launch stages a small PowerShell script that waits for this process to exit,
// runs the NSIS installer, and reopens the app. Passing the paths as -File
// arguments avoids the quoting and newline problems of -Command, and the ready
// file proves the helper really started: a script that never runs must not be
// allowed to quit the app into a broken update.
func Launch(path string) (bool, string, error) {
	exe, err := os.Executable()
	if err != nil {
		return false, "", fmt.Errorf("无法定位当前程序：%w", err)
	}
	dir := filepath.Dir(path)
	scriptPath := filepath.Join(dir, "install.ps1")
	logPath := filepath.Join(dir, "install.log")
	readyPath := logPath + ".ready"
	_ = os.Remove(readyPath)
	if err := os.WriteFile(scriptPath, []byte(installScript), 0o600); err != nil {
		return false, "", fmt.Errorf("无法写入更新脚本：%w", err)
	}

	cmd := exec.Command(powershell(),
		"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass",
		"-File", scriptPath,
		"-AppPid", strconv.Itoa(os.Getpid()),
		"-Installer", path,
		"-Exe", exe,
		"-Log", logPath,
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := cmd.Start(); err != nil {
		return false, "", fmt.Errorf("无法启动更新程序：%w", err)
	}

	// PowerShell's cold start can take a few seconds; if the marker never
	// appears, the helper failed to parse or launch, and the app stays open.
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(readyPath); err == nil {
			_ = cmd.Process.Release()
			return true, "正在安装新版本，应用即将退出；安装完成后会自动重新打开。", nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	_ = cmd.Process.Kill()
	_ = cmd.Process.Release()
	return false, "", errors.New("更新程序没有启动，请稍后重试")
}

// installScript stays ASCII: Windows PowerShell 5.1 reads a BOM-less file in
// the ANSI code page, and the log is for diagnosis rather than the interface.
// $PID is a read-only automatic variable, hence $AppPid.
const installScript = `param(
  [Parameter(Mandatory=$true)][int]$AppPid,
  [Parameter(Mandatory=$true)][string]$Installer,
  [Parameter(Mandatory=$true)][string]$Exe,
  [Parameter(Mandatory=$true)][string]$Log
)

$ErrorActionPreference = 'Continue'
function Write-Log([string]$Text) {
  try {
    "$(Get-Date -Format o) $Text" | Out-File -FilePath $Log -Append -Encoding utf8
  } catch {
  }
}

Write-Log "waiting for process $AppPid"
try {
  [IO.File]::WriteAllText(($Log + '.ready'), '1')
} catch {
  Write-Log ("could not write the ready marker: " + $_.Exception.Message)
}

while (Get-Process -Id $AppPid -ErrorAction SilentlyContinue) { Start-Sleep -Milliseconds 200 }
Write-Log "application exited"

$ErrorActionPreference = 'Stop'
try {
  Write-Log "running installer silently"
  $p = Start-Process -FilePath $Installer -ArgumentList '/S' -Wait -PassThru
  # Reading ExitCode from an elevated child can itself be denied; an
  # unreadable code is not a failed install.
  $code = 0
  try { $code = $p.ExitCode } catch { Write-Log ("could not read the installer exit code: " + $_.Exception.Message) }
  Write-Log ("installer exit code " + $code)
  if ($code -ne 0) {
    throw ("the installer returned " + $code)
  }
} catch {
  Write-Log ("silent install failed: " + $_.Exception.Message)
  try {
    Write-Log "opening the installer interactively"
    Start-Process -FilePath $Installer -Wait
    Write-Log "interactive installer finished"
  } catch {
    Write-Log ("interactive install failed: " + $_.Exception.Message)
  }
}

Write-Log "starting $Exe"
try {
  Start-Process -FilePath $Exe
} catch {
  Write-Log ("could not start the application: " + $_.Exception.Message)
}
`

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
