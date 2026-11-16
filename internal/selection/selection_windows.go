//go:build windows

package selection

import (
	"errors"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"
)

var (
	user32                     = syscall.NewLazyDLL("user32.dll")
	procSendInput              = user32.NewProc("SendInput")
	procOpenClipboard          = user32.NewProc("OpenClipboard")
	procCloseClipboard         = user32.NewProc("CloseClipboard")
	procGetClipboardSequenceNo = user32.NewProc("GetClipboardSequenceNumber")
)

const (
	inputKeyboard = 1
	keyUp         = 2
	keyUnicode    = 4

	vkControl = 0x11
	vkC       = 0x43
	vkV       = 0x56
	vkA       = 0x41
)

// keybdInput mirrors KEYBDINPUT. The padding after Type is not decoration: the
// union inside INPUT is 8-byte aligned on amd64, so a struct without it makes
// SendInput read past the buffer.
type keybdInput struct {
	Vk       uint16
	Scan     uint16
	Flags    uint32
	Reserved uint32
	Extra    uintptr
}

type inputEvent struct {
	Type uint32
	_    uint32
	Kb   keybdInput
}

func key(vk uint16, down bool) inputEvent {
	e := inputEvent{Type: inputKeyboard, Kb: keybdInput{Vk: vk}}
	if !down {
		e.Kb.Flags = keyUp
	}
	return e
}

// Trusted: Windows grants UIPI-level input synthesis to any ordinary process, so
// there is no permission to ask for.
func Trusted() bool      { return true }
func RequestTrust() bool { return true }

// SelectedText has no answer here. Reading a foreign selection means UI
// Automation, which is a COM client the app does not carry; the clipboard route
// below covers the same user intent.
func SelectedText() (string, bool) { return "", false }

// Pollable is false because the only route this platform has writes to the
// clipboard, and 自动划词 must not do that on a timer.
const Pollable = false

// FrontmostIsSelf is never reached while Pollable is false.
func FrontmostIsSelf() bool { return false }

// FrontmostBundleID has no portable answer here; nothing on this platform
// rewrites a selection based on the source application.
func FrontmostBundleID() string { return "" }

// FocusedElementEditable is only consulted by the macOS ⌘A gesture.
func FocusedElementEditable() bool { return false }

// TrackFrontmostApps is macOS-only: without an accessibility read there is no
// reason to hand the keyboard anywhere.
func TrackFrontmostApps() {}

// ReactivateLastOther has no equivalent here; 划词 goes through the clipboard.
func ReactivateLastOther() bool { return false }

// Copy synthesizes ctrl+C for the foreground window.
func Copy() error { return chord(vkC) }

// CopyMenu has no platform equivalent worth carrying: a foreign app's menu is
// not reachable without UI Automation, so 强制取词 runs the chord route here.
func CopyMenu() error { return ErrUnsupported }

// BrowserSelectedText is macOS-only: no browser scripting interface exists here.
func BrowserSelectedText() (string, bool) { return "", false }

func chord(vk uint16) error {
	events := []inputEvent{key(vkControl, true), key(vk, true), key(vk, false), key(vkControl, false)}
	size := int32(unsafe.Sizeof(inputEvent{}))
	for _, e := range events {
		// SendInput honours a size mismatch by rejecting the event outright, so
		// the struct size has to be passed exactly as the API sees it.
		r, _, callErr := procSendInput.Call(1, uintptr(unsafe.Pointer(&e)), uintptr(size))
		if r != 1 {
			if callErr != nil && callErr != syscall.Errno(0) {
				return callErr
			}
			return errors.New("取词：模拟 Ctrl+C 失败")
		}
		time.Sleep(8 * time.Millisecond)
	}
	return nil
}

// Paste synthesizes ctrl+V for the foreground window.
func Paste() error { return chord(vkV) }

// SelectAll synthesizes ctrl+A for the foreground window.
func SelectAll() error { return chord(vkA) }

// TypeText types the answer with Unicode scan codes instead of pasting it,
// which is 兼容模式替换.
func TypeText(text string) error {
	size := int32(unsafe.Sizeof(inputEvent{}))
	for _, unit := range utf16.Encode([]rune(text)) {
		for _, down := range []bool{true, false} {
			e := inputEvent{Type: inputKeyboard, Kb: keybdInput{Scan: unit, Flags: keyUnicode}}
			if !down {
				e.Kb.Flags |= keyUp
			}
			r, _, callErr := procSendInput.Call(1, uintptr(unsafe.Pointer(&e)), uintptr(size))
			if r != 1 {
				if callErr != nil && callErr != syscall.Errno(0) {
					return callErr
				}
				return errors.New("替换：模拟键盘输入失败")
			}
		}
	}
	return nil
}

// ChangeCount returns -1 while the number is meaningless, which is until this
// process has opened the clipboard at least once.
func ChangeCount() int64 {
	if r, _, _ := procOpenClipboard.Call(0); r != 0 {
		procCloseClipboard.Call()
	}
	n, _, _ := procGetClipboardSequenceNo.Call()
	if n == 0 {
		return -1
	}
	return int64(n)
}
