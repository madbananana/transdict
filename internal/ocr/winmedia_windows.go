//go:build windows

package ocr

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
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
	// A one-line selection is below the size Windows OCR can read, so enlarge
	// the picture before the helper turns it into a bitmap.
	png, scale, err := upscaleForOCR(png)
	if err != nil {
		return nil, fmt.Errorf("Windows OCR：无法放大识别图片：%w", err)
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

	// The result goes to a file rather than stdout: a GUI process has no usable
	// console, and a file also keeps the Chinese text away from code pages.
	resultPath := filepath.Join(dir, "result.txt")
	args := []string{
		"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass",
		"-File", scriptPath, "-ImagePath", imgPath, "-OutputPath", resultPath,
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
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		reason := strings.TrimSpace(stderr.String())
		if reason == "" {
			reason = err.Error()
		}
		log.Printf("transdict: Windows OCR 调用失败: %v: %s", err, reason)
		return nil, fmt.Errorf("Windows OCR 调用失败：%s", reason)
	}
	result, err := os.ReadFile(resultPath)
	if err != nil {
		log.Printf("transdict: Windows OCR 没有写出结果: %v", err)
		return nil, fmt.Errorf("Windows OCR 没有返回结果：%w", err)
	}
	blocks, err := decodeWinOCR(result)
	if err != nil {
		// The preview shows this to the user; the log keeps the whole sentence
		// for a bug report, including the PowerShell line number.
		log.Printf("transdict: Windows OCR: %v", err)
	}
	if scale > 1 {
		// The boxes belong to the enlarged picture the engine read; the preview
		// draws them over the original crop, so map them back.
		shrinkBlocks(blocks, scale)
	}
	return blocks, err
}

// windowsOCRPowerShell awaits the WinRT async chain (file → stream → decoder →
// software bitmap → recognition), picks a recognizer for the requested or an
// installed language, and writes the lines with their pixel boxes to -OutputPath
// as a pipe-separated text protocol:
//
//	engine|<tag>
//	block|<x>|<y>|<w>|<h>|<score>|<text>
//	error|<message>
//
// Every free-text field is percent-encoded with Uri.EscapeDataString, so a
// newline inside a recognized line cannot split the record and the console code
// page cannot mangle Chinese. v0.1.2 serialized the whole payload with
// ConvertTo-Json on stdout and failed inside the GUI process at the
// Write-Payload call; the file protocol has no serializer and no console left
// to go wrong.
//
// The awaiter is picked from GetAwaiter and matched on the exact closed generic
// IAsyncOperation`1. The AsTask overloads differ in progress reporting, and
// feeding the wrong one to reflection Invoke raises "参数类型不匹配".
const windowsOCRPowerShell = `
param(
  [Parameter(Mandatory=$true)][string]$ImagePath,
  [Parameter(Mandatory=$true)][string]$OutputPath,
  [string]$Languages = ''
)

$ErrorActionPreference = 'Stop'

try {
  Add-Type -AssemblyName System.Runtime.WindowsRuntime | Out-Null
  [Windows.Storage.StorageFile, Windows.Storage, ContentType=WindowsRuntime] | Out-Null
  [Windows.Storage.Streams.RandomAccessStream, Windows.Storage.Streams, ContentType=WindowsRuntime] | Out-Null
  [Windows.Graphics.Imaging.BitmapDecoder, Windows.Graphics.Imaging, ContentType=WindowsRuntime] | Out-Null
  [Windows.Graphics.Imaging.SoftwareBitmap, Windows.Graphics.Imaging, ContentType=WindowsRuntime] | Out-Null
  [Windows.Globalization.Language, Windows.Globalization, ContentType=WindowsRuntime] | Out-Null
  [Windows.Media.Ocr.OcrEngine, Windows.Foundation, ContentType=WindowsRuntime] | Out-Null

  # GetAwaiter is generic over IAsyncOperation; 96 is the backtick that
  # separates the CLR name from its type argument (a literal one would close
  # the Go raw string this script lives in).
  $awaiter = [WindowsRuntimeSystemExtensions].GetMember('GetAwaiter', 'Method', 'Public,Static') |
    Where-Object {
      $_.GetParameters().Count -eq 1 -and $_.GetParameters()[0].ParameterType.Name -eq ('IAsyncOperation' + [char]96 + '1')
    } |
    Select-Object -First 1
  if ($null -eq $awaiter) { throw '无法初始化 WinRT 异步调用' }

  function Await([object]$operation, [Type]$result) {
    return $awaiter.MakeGenericMethod($result).Invoke($null, @($operation)).GetResult()
  }

  $file = Await ([Windows.Storage.StorageFile]::GetFileFromPathAsync($ImagePath)) ([Windows.Storage.StorageFile])
  $stream = Await ($file.OpenAsync([Windows.Storage.FileAccessMode]::Read)) ([Windows.Storage.Streams.IRandomAccessStream])
  $decoder = Await ([Windows.Graphics.Imaging.BitmapDecoder]::CreateAsync($stream)) ([Windows.Graphics.Imaging.BitmapDecoder])
  $bitmap = Await ($decoder.GetSoftwareBitmapAsync()) ([Windows.Graphics.Imaging.SoftwareBitmap])

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

  $out = New-Object 'System.Collections.Generic.List[string]'
  $tried = New-Object 'System.Collections.Generic.List[string]'
  $engineUsed = ''
  foreach ($tag in $candidates) {
    $engine = $null
    try {
      $language = [Windows.Globalization.Language]::new($tag)
      $engine = [Windows.Media.Ocr.OcrEngine]::TryCreateFromLanguage($language)
    } catch {
      $engine = $null
    }
    if ($null -eq $engine) {
      [void]$tried.Add("$tag 不可用")
      continue
    }

    # One recognizer refusing the bitmap (a missing language pack, an
    # unsupported pixel format) must not stop the next candidate.
    try {
      $result = Await ($engine.RecognizeAsync($bitmap)) ([Windows.Media.Ocr.OcrResult])
    } catch {
      [void]$tried.Add("$tag：" + $_.Exception.Message)
      continue
    }
    if ($null -eq $result) {
      [void]$tried.Add("$tag：无结果")
      continue
    }
    $lines = @($result.Lines)
    if ($lines.Count -eq 0) {
      [void]$tried.Add("$tag：0 行")
      continue
    }

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
      $escaped = [Uri]::EscapeDataString($text)
      [void]$out.Add("block|$x|$y|$w|$h|0|$escaped")
    }
    $engineUsed = $tag
    break
  }

  if ($out.Count -eq 0) {
    $detail = '没有可用的识别器'
    if ($tried.Count -gt 0) { $detail = $tried -join '；' }
    throw "没有识别到文字（$detail；图片 $($decoder.PixelWidth)x$($decoder.PixelHeight)）"
  }

  $out.Insert(0, "engine|" + [Uri]::EscapeDataString($engineUsed))
  [void][IO.File]::WriteAllText($OutputPath, ($out -join [Environment]::NewLine), [Text.Encoding]::UTF8)
} catch {
  $reason = $_.Exception.Message
  $inner = $_.Exception.InnerException
  if ($null -ne $inner -and $inner.Message -ne $reason) { $reason = "$reason：$($inner.Message)" }
  if ($_.InvocationInfo -and $_.InvocationInfo.ScriptLineNumber) {
    $reason = "$reason（脚本第 $($_.InvocationInfo.ScriptLineNumber) 行）"
  }
  # A failure report that names the PowerShell and the OCR languages saves a
  # round trip through the user for the two causes a wrong script cannot see.
  $diag = "PowerShell $($PSVersionTable.PSVersion) $([System.Environment]::OSVersion.VersionString)"
  try {
    $langs = @([Windows.Media.Ocr.OcrEngine]::AvailableRecognizerLanguages | ForEach-Object { [string]$_.LanguageTag })
    if ($langs.Count -gt 0) { $diag += "；已安装 OCR：" + ($langs -join ', ') } else { $diag += "；已安装 OCR：无" }
  } catch {
    $diag += "；OCR 引擎不可用"
  }
  $reason = "$reason [$diag]"
  [void][IO.File]::WriteAllText($OutputPath, ("error|" + [Uri]::EscapeDataString($reason)), [Text.Encoding]::UTF8)
}
`
