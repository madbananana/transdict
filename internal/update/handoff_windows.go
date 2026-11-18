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
//
// version is the release being installed. The helper waits until the exe on
// disk reports it before starting the app: an elevated installer can outlive
// the process Start-Process waited for, and starting the old build is what
// makes the app come back and ask for the same update again.
func Launch(path, version string) (bool, string, error) {
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
		"-Version", version,
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
  [Parameter(Mandatory=$true)][string]$Log,
  [Parameter(Mandatory=$true)][string]$Version
)

$ErrorActionPreference = 'Continue'
function Write-Log([string]$Text) {
  try {
    "$(Get-Date -Format o) $Text" | Out-File -FilePath $Log -Append -Encoding utf8
  } catch {
  }
}

# The version resource is the proof that the file on disk really was replaced;
# reading it while the installer is still writing fails, which is why the
# wait below polls instead of asking once.
function Get-ExeVersion([string]$Path) {
  try {
    $info = (Get-Item -LiteralPath $Path).VersionInfo
    if ($info.ProductVersion) { return [string]$info.ProductVersion }
    return [string]$info.FileVersion
  } catch {
    return ''
  }
}

function Test-VersionAtLeast([string]$Have, [string]$Want) {
  $a = ($Have -replace '^[^0-9]*', '') -replace '[^0-9.].*$', ''
  $b = ($Want -replace '^[^0-9]*', '') -replace '[^0-9.].*$', ''
  if (-not $a -or -not $b) { return $false }
  try {
    return ([version]$a -ge [version]$b)
  } catch {
    return $false
  }
}

function Wait-Installed([string]$Path, [string]$Want, [int]$Seconds) {
  $deadline = (Get-Date).AddSeconds($Seconds)
  while ((Get-Date) -lt $deadline) {
    if (Test-VersionAtLeast (Get-ExeVersion $Path) $Want) { return $true }
    Start-Sleep -Milliseconds 500
  }
  return $false
}

Write-Log "waiting for process $AppPid"
try {
  [IO.File]::WriteAllText(($Log + '.ready'), '1')
} catch {
  Write-Log ("could not write the ready marker: " + $_.Exception.Message)
}

while (Get-Process -Id $AppPid -ErrorAction SilentlyContinue) { Start-Sleep -Milliseconds 200 }
$before = Get-ExeVersion $Exe
Write-Log ("application exited; version on disk is " + $before)

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

# The exit code says the installer was started, not that it finished: an
# elevated copy can still be copying when Start-Process returns. Starting the
# app before the replacement lands brings the old build back, which then
# offers the very update that was just installed.
if ($before -and -not (Wait-Installed $Exe $Version 120)) {
  Write-Log ("version on disk is still " + (Get-ExeVersion $Exe) + ", wanted " + $Version)
  try {
    Write-Log "re-running the installer interactively"
    Start-Process -FilePath $Installer -Wait
  } catch {
    Write-Log ("interactive re-run failed: " + $_.Exception.Message)
  }
  if (-not (Wait-Installed $Exe $Version 60)) {
    Write-Log ("still not installed: " + (Get-ExeVersion $Exe))
  }
}

Write-Log ("starting $Exe (" + (Get-ExeVersion $Exe) + ")")
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
