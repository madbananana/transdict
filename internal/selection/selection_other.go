//go:build !darwin && !windows

package selection

import "errors"

// Neither an accessibility read nor a verified input synthesis exists on this
// platform, so every entry point declines and Grab reports ErrUnsupported.

func Trusted() bool                { return false }
func RequestTrust() bool           { return false }
func SelectedText() (string, bool) { return "", false }

func Copy() error      { return ErrUnsupported }
func CopyMenu() error  { return ErrUnsupported }
func Paste() error     { return ErrUnsupported }
func SelectAll() error { return ErrUnsupported }

// BrowserSelectedText is macOS-only: no other platform routes selections
// through a browser's scripting interface.
func BrowserSelectedText() (string, bool) { return "", false }

// TypeText has no keystroke source here, so 兼容模式替换 cannot run.
func TypeText(text string) error {
	return errors.New("替换：当前系统不支持模拟键盘输入")
}

// ChangeCount has no portable equivalent; -1 tells Grab to compare text instead.
func ChangeCount() int64 { return -1 }

const Pollable = false

func FrontmostIsSelf() bool { return false }

// FrontmostBundleID has no answer without a platform integration.
func FrontmostBundleID() string { return "" }

// FocusedElementEditable is only consulted by the macOS ⌘A gesture.
func FocusedElementEditable() bool { return false }

// TrackFrontmostApps and ReactivateLastOther are macOS-only: there is no
// accessibility read on this platform to hand the keyboard back to.
func TrackFrontmostApps() {}

func ReactivateLastOther() bool { return false }
