//go:build windows

package ocr

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"transdict/internal/settings"
)

// Windows ships an OCR engine with the OS (Windows.Media.Ocr), the same one
// PowerToys' Text Extractor and Pot call for their 系统 OCR option. It reads the
// picture offline and needs no key, so it takes Vision's place as the default
// screenshot reader on a fresh install.
//
// WinRT is reached through Windows PowerShell rather than cgo: the Go side stays
// pure and the build keeps CGO_ENABLED=0, while the script still returns word
// boxes so the OCR window can draw the same line overlays Vision supplies.
func init() {
	Register(settings.ServiceType{
		Kind: "windows",
		Name: "Windows 系统 OCR",
		Note: "调用 Windows 自带的 OCR（Windows.Media.Ocr），离线、免密钥；识别语言取决于系统已安装的语言包。",
		Free: true,
	}, func(settings.Provider) (Provider, error) { return windowsOCR{}, nil })
}

type windowsOCR struct{}

func (windowsOCR) Label() string { return "Windows 系统 OCR" }

func (windowsOCR) Recognize(ctx context.Context, png []byte) ([]Block, error) {
	if len(png) == 0 {
		return nil, errors.New("没有可识别的图片")
	}
	dir, err := os.MkdirTemp("", "transdict-ocr-")
	if err != nil {
		return nil, fmt.Errorf("Windows OCR：无法创建临时目录：%w", err)
	}
	defer os.RemoveAll(dir)

	// The picture goes through a file because StorageFile is the only WinRT
	// entry point that decodes every format the capture side can hand over.
	imgPath := filepath.Join(dir, "crop.png")
	if err := os.WriteFile(imgPath, png, 0o600); err != nil {
		return nil, fmt.Errorf("Windows OCR：无法写入临时图片：%w", err)
	}
	// Windows PowerShell 5.1 reads a script without a BOM as ANSI, which would
	// mangle the Chinese error text; the BOM keeps the literals intact.
	scriptPath := filepath.Join(dir, "ocr.ps1")
	if err := os.WriteFile(scriptPath, []byte("\ufeff"+windowsOCRPowerShell), 0o600); err != nil {
		return nil, fmt.Errorf("Windows OCR：无法写入识别脚本：%w", err)
	}

	args := []string{
		"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass",
		"-File", scriptPath, "-ImagePath", imgPath,
	}
	if hint := LangFromContext(ctx); hint != "" {
		args = append(args, "-Languages", hint)
	}
	// A GUI launch can inherit a PATH without the PowerShell directory, so the
	// absolute copy under System32 is the fallback rather than a hard failure.
	powershell := "powershell.exe"
	if _, lookErr := exec.LookPath(powershell); lookErr != nil {
		root := os.Getenv("SystemRoot")
		if root == "" {
			root = filepath.Join("C:", "Windows")
		}
		candidate := filepath.Join(root, "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
		if _, statErr := os.Stat(candidate); statErr == nil {
			powershell = candidate
		}
	}
	cmd := exec.CommandContext(ctx, powershell, args...)
	// The app is a GUI process; without this every capture would flash a
	// console window over the screenshot.
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		reason := strings.TrimSpace(stderr.String())
		if reason == "" {
			reason = err.Error()
		}
		return nil, fmt.Errorf("Windows OCR 调用失败：%s", reason)
	}
	return decodeWinOCR(stdout.Bytes())
}

// windowsOCRPowerShell awaits the WinRT async chain (file → stream → decoder →
// software bitmap → recognition), picks a recognizer for the requested or an
// installed language, and returns the lines with their pixel boxes as base64
// JSON. The script stays ASCII apart from its message literals, which the BOM
// above protects.
const windowsOCRPowerShell = `
param(
  [Parameter(Mandatory=$true)][string]$ImagePath,
  [string]$Languages = ''
)

$ErrorActionPreference = 'Stop'

function Write-Payload($payload) {
  $json = $payload | ConvertTo-Json -Compress -Depth 8
  $b64 = [Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes($json))
  [Console]::Out.Write($b64)
}

try {
  Add-Type -AssemblyName System.Runtime.WindowsRuntime | Out-Null
  [Windows.Storage.StorageFile, Windows.Storage, ContentType=WindowsRuntime] | Out-Null
  [Windows.Graphics.Imaging.BitmapDecoder, Windows.Graphics.Imaging, ContentType=WindowsRuntime] | Out-Null
  [Windows.Globalization.Language, Windows.Globalization, ContentType=WindowsRuntime] | Out-Null
  [Windows.Media.Ocr.OcrEngine, Windows.Media.Ocr, ContentType=WindowsRuntime] | Out-Null

  $asTask = ([System.WindowsRuntimeSystemExtensions].GetMethods() | Where-Object {
    $_.Name -eq 'AsTask' -and $_.GetParameters().Count -eq 1 -and $_.GetParameters()[0].ParameterType.Name -like 'IAsyncOperation*'
  })[0]
  if ($null -eq $asTask) { throw '无法初始化 WinRT 异步调用' }

  function Await($operation, [string]$typeName) {
    $generic = $asTask.MakeGenericMethod([Type]::GetType($typeName))
    return $generic.Invoke($null, @($operation)).GetAwaiter().GetResult()
  }

  $file = Await ([Windows.Storage.StorageFile]::GetFileFromPathAsync($ImagePath)) 'Windows.Storage.StorageFile, Windows.Storage, ContentType=WindowsRuntime'
  $stream = Await ($file.OpenReadAsync()) 'Windows.Storage.Streams.IRandomAccessStreamWithContentType, Windows.Storage.Streams, ContentType=WindowsRuntime'
  $decoder = Await ([Windows.Graphics.Imaging.BitmapDecoder]::CreateAsync($stream)) 'Windows.Graphics.Imaging.BitmapDecoder, Windows.Graphics.Imaging, ContentType=WindowsRuntime'
  $bitmap = Await ($decoder.GetSoftwareBitmapAsync()) 'Windows.Graphics.Imaging.SoftwareBitmap, Windows.Graphics.Imaging, ContentType=WindowsRuntime'

  $installed = @()
  foreach ($language in [Windows.Media.Ocr.OcrEngine]::AvailableRecognizerLanguages) {
    $installed += [string]$language.LanguageTag
  }
  if ($installed.Count -eq 0) {
    throw '系统没有安装任何 OCR 语言包；请在「设置 → 时间和语言 → 语言」中添加语言并勾选「光学字符识别」'
  }

  $candidates = New-Object 'System.Collections.Generic.List[string]'
  $seen = @{}
  function Add-Candidate([string]$tag) {
    if ([string]::IsNullOrWhiteSpace($tag)) { return }
    $key = $tag.Trim().ToLowerInvariant()
    if ($seen.ContainsKey($key)) { return }
    $seen[$key] = $true
    [void]$candidates.Add($tag.Trim())
  }

  foreach ($hint in ($Languages -split ',')) {
    $h = $hint.Trim()
    if ($h -eq '') { continue }
    $match = $installed | Where-Object { $_ -ieq $h -or $_ -like "$h-*" } | Select-Object -First 1
    if ($null -eq $match) {
      $prefix = ($h -split '-')[0]
      $match = $installed | Where-Object { ($_ -split '-')[0] -ieq $prefix } | Select-Object -First 1
    }
    if ($null -ne $match) { Add-Candidate ([string]$match) } else { Add-Candidate $h }
  }

  # 没有明确语种时先试非英语识别器：中文/日文引擎同样能读拉丁字母，反过来不行。
  foreach ($tag in $installed) {
    if ($tag -notmatch '^en($|-)') { Add-Candidate $tag }
  }
  foreach ($tag in $installed) { Add-Candidate $tag }

  $blocks = New-Object 'System.Collections.Generic.List[object]'
  $engineUsed = ''
  foreach ($tag in $candidates) {
    $engine = $null
    try {
      $language = [Windows.Globalization.Language]::new($tag)
      $engine = [Windows.Media.Ocr.OcrEngine]::TryCreateFromLanguage($language)
    } catch {
      $engine = $null
    }
    if ($null -eq $engine) { continue }

    $result = Await ($engine.RecognizeAsync($bitmap)) 'Windows.Media.Ocr.OcrResult, Windows.Media.Ocr, ContentType=WindowsRuntime'
    if ($null -eq $result) { continue }
    $lines = @($result.Lines)
    if ($lines.Count -eq 0) { continue }

    foreach ($line in $lines) {
      $text = ([string]$line.Text).Trim()
      if ($text -eq '') { continue }
      $minX = [double]::MaxValue
      $minY = [double]::MaxValue
      $maxX = [double]::MinValue
      $maxY = [double]::MinValue
      foreach ($word in $line.Words) {
        $rect = $word.BoundingRect
        if ($rect.Width -le 0 -or $rect.Height -le 0) { continue }
        if ($rect.X -lt $minX) { $minX = $rect.X }
        if ($rect.Y -lt $minY) { $minY = $rect.Y }
        if (($rect.X + $rect.Width) -gt $maxX) { $maxX = $rect.X + $rect.Width }
        if (($rect.Y + $rect.Height) -gt $maxY) { $maxY = $rect.Y + $rect.Height }
      }
      $x = 0
      $y = 0
      $w = 0
      $h = 0
      if ($maxX -gt $minX -and $maxY -gt $minY) {
        $x = [int][Math]::Round($minX)
        $y = [int][Math]::Round($minY)
        $w = [int][Math]::Round($maxX - $minX)
        $h = [int][Math]::Round($maxY - $minY)
      }
      [void]$blocks.Add([ordered]@{ text = $text; x = $x; y = $y; width = $w; height = $h; score = 0 })
    }
    $engineUsed = $tag
    break
  }

  Write-Payload ([ordered]@{ blocks = @($blocks); engine = $engineUsed })
} catch {
  Write-Payload ([ordered]@{ blocks = @(); engine = ''; error = $_.Exception.Message })
}
`
