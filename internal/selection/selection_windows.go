//go:build windows

package selection

import (
	"errors"
	"syscall"
	"time"
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

	vkControl = 0x11
	vkC       = 0x43
	vkV       = 0x56
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

// Copy synthesizes ctrl+C for the foreground window.
func Copy() error { return chord(vkC) }

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
