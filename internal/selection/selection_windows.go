//go:build windows

package selection

import (
	"errors"
	"log"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	ole32    = syscall.NewLazyDLL("ole32.dll")
	oleaut32 = syscall.NewLazyDLL("oleaut32.dll")

	procSendInput              = user32.NewProc("SendInput")
	procOpenClipboard          = user32.NewProc("OpenClipboard")
	procCloseClipboard         = user32.NewProc("CloseClipboard")
	procGetClipboardSequenceNo = user32.NewProc("GetClipboardSequenceNumber")
	procGetForegroundWindow    = user32.NewProc("GetForegroundWindow")
	procGetWindowThreadProcess = user32.NewProc("GetWindowThreadProcessId")

	procOpenProcess          = kernel32.NewProc("OpenProcess")
	procCloseHandle          = kernel32.NewProc("CloseHandle")
	procQueryFullProcessName = kernel32.NewProc("QueryFullProcessImageNameW")
	procGetCurrentProcessID  = kernel32.NewProc("GetCurrentProcessId")

	procCoInitializeEx   = ole32.NewProc("CoInitializeEx")
	procCoUninitialize   = ole32.NewProc("CoUninitialize")
	procCoCreateInstance = ole32.NewProc("CoCreateInstance")
	procSysStringLen     = oleaut32.NewProc("SysStringLen")
	procSysFreeString    = oleaut32.NewProc("SysFreeString")
	procVariantClear     = oleaut32.NewProc("VariantClear")
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

// SelectedText asks UI Automation for the focused element's selection. This is
// the same route the macOS build takes through the accessibility API: it reads
// without touching the clipboard, so 自动划词 can poll it.
func SelectedText() (string, bool) { return uiaSelection() }

// Pollable is true because the UI Automation read has no side effects.
const Pollable = true

// FrontmostIsSelf keeps the watcher from asking about a highlight made inside
// our own result window.
func FrontmostIsSelf() bool {
	hwnd, _, _ := procGetForegroundWindow.Call()
	if hwnd == 0 {
		return false
	}
	var pid uint32
	procGetWindowThreadProcess.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	self, _, _ := procGetCurrentProcessID.Call()
	return uintptr(pid) == self
}

// FrontmostBundleID answers the executable name of the foreground window
// ("keepass.exe"). 禁止名单 compares that string, so a Windows entry stores the
// same lowercased name.
func FrontmostBundleID() string {
	hwnd, _, _ := procGetForegroundWindow.Call()
	if hwnd == 0 {
		return ""
	}
	var pid uint32
	procGetWindowThreadProcess.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	return processExeName(pid)
}

// processExeName resolves one pid to its lowercased executable name.
func processExeName(pid uint32) string {
	const queryLimited = 0x1000
	handle, _, _ := procOpenProcess.Call(queryLimited, 0, uintptr(pid))
	if handle == 0 {
		return ""
	}
	defer procCloseHandle.Call(handle)
	buf := make([]uint16, 1024)
	size := uint32(len(buf))
	if r, _, _ := procQueryFullProcessName.Call(handle, 0,
		uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size))); r == 0 {
		return ""
	}
	return strings.ToLower(filepath.Base(syscall.UTF16ToString(buf[:size])))
}

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

// UI Automation is the Windows counterpart of the macOS accessibility read:
// the focused element exposes a TextPattern whose selection is the highlighted
// text. The interfaces are called through their vtables because only a handful
// of methods are needed and a COM binding would be a dependency for them.
const (
	uiaTextPatternID = 10014
	uiaIsPasswordID  = 30002

	// Vtable slots, IUnknown's three included.
	uiaGetFocusedElement  = 8
	uiaGetCurrentPattern  = 16
	uiaGetCurrentProperty = 10
	uiaGetSelection       = 5
	uiaRangeCount         = 3
	uiaRangeAt            = 4
	uiaRangeText          = 12
	comReleaseSlot        = 2
	clsctxInprocServer    = 1
	coinitMultithreaded   = 0
	uiaAllText            = ^uintptr(0) // GetText's -1
	variantSize           = 3 * unsafe.Sizeof(uint64(0))
	variantTypeBool       = 11
)

// comGUID is the 16-byte layout CoCreateInstance reads.
type comGUID struct {
	data1 uint32
	data2 uint16
	data3 uint16
	data4 [8]byte
}

var (
	clsidCUIAutomation = comGUID{0xFF48DBA4, 0x60EF, 0x4201, [8]byte{0xAA, 0x87, 0x54, 0x10, 0x3E, 0xEF, 0x59, 0x4E}}
	iidIUIAutomation   = comGUID{0x30CBE57D, 0xD9D0, 0x452A, [8]byte{0xAB, 0x13, 0x7A, 0xC5, 0xAC, 0x48, 0x25, 0xEE}}
)

// comCall invokes one vtable slot and answers the HRESULT.
func comCall(unk unsafe.Pointer, slot uintptr, args ...uintptr) int32 {
	vtable := *(*unsafe.Pointer)(unk)
	fn := *(*uintptr)(unsafe.Add(vtable, slot*unsafe.Sizeof(uintptr(0))))
	argv := make([]uintptr, 0, len(args)+1)
	argv = append(argv, uintptr(unk))
	argv = append(argv, args...)
	r, _, _ := syscall.SyscallN(fn, argv...)
	return int32(r)
}

func comRelease(unk unsafe.Pointer) {
	if unk != nil {
		comCall(unk, comReleaseSlot)
	}
}

// bstrString copies a BSTR into a Go string; the caller still frees it.
func bstrString(p *uint16) string {
	n, _, _ := procSysStringLen.Call(uintptr(unsafe.Pointer(p)))
	if n == 0 {
		return ""
	}
	return string(utf16.Decode(unsafe.Slice(p, int(n))))
}

// automation is the one UI Automation client. Everything it returns is released
// before the next call, so a long-lived poll costs no objects.
type automation struct {
	root unsafe.Pointer
}

func newAutomation() (*automation, error) {
	var root unsafe.Pointer
	hr, _, _ := procCoCreateInstance.Call(
		uintptr(unsafe.Pointer(&clsidCUIAutomation)),
		0,
		clsctxInprocServer,
		uintptr(unsafe.Pointer(&iidIUIAutomation)),
		uintptr(unsafe.Pointer(&root)),
	)
	if int32(hr) < 0 || root == nil {
		return nil, errors.New("无法创建 UI Automation 客户端")
	}
	return &automation{root: root}, nil
}

// selection reads the highlighted text of the focused element. ok=false means
// the answer is unknown — no focused element, or one that carries no text
// pattern (a browser toolbar, a canvas) — and the watcher must not treat that
// as "the selection was cleared".
func (a *automation) selection() (string, bool) {
	var element unsafe.Pointer
	if comCall(a.root, uiaGetFocusedElement, uintptr(unsafe.Pointer(&element))) < 0 || element == nil {
		return "", false
	}
	defer comRelease(element)
	// A password field must never reach a translation service, and a masked
	// value would be useless to the user anyway.
	if a.isPassword(element) {
		return "", true
	}
	var pattern unsafe.Pointer
	if comCall(element, uiaGetCurrentPattern, uintptr(uiaTextPatternID),
		uintptr(unsafe.Pointer(&pattern))) < 0 || pattern == nil {
		return "", false
	}
	defer comRelease(pattern)
	var ranges unsafe.Pointer
	if comCall(pattern, uiaGetSelection, uintptr(unsafe.Pointer(&ranges))) < 0 || ranges == nil {
		return "", false
	}
	defer comRelease(ranges)
	var count int32
	if comCall(ranges, uiaRangeCount, uintptr(unsafe.Pointer(&count))) < 0 || count <= 0 {
		return "", true
	}
	var text strings.Builder
	for i := int32(0); i < count; i++ {
		var textRange unsafe.Pointer
		if comCall(ranges, uiaRangeAt, uintptr(i), uintptr(unsafe.Pointer(&textRange))) < 0 || textRange == nil {
			continue
		}
		var bstr *uint16
		if comCall(textRange, uiaRangeText, uiaAllText, uintptr(unsafe.Pointer(&bstr))) >= 0 && bstr != nil {
			text.WriteString(bstrString(bstr))
			procSysFreeString.Call(uintptr(unsafe.Pointer(bstr)))
		}
		comRelease(textRange)
	}
	return text.String(), true
}

// isPassword answers UIA_IsPasswordPropertyId. The VARIANT is cleared by the
// API's own clear, which is the only thing that knows what it filled in.
func (a *automation) isPassword(element unsafe.Pointer) bool {
	var variant [variantSize / unsafe.Sizeof(uint64(0))]uint64
	if comCall(element, uiaGetCurrentProperty, uintptr(uiaIsPasswordID),
		uintptr(unsafe.Pointer(&variant[0]))) < 0 {
		return false
	}
	defer procVariantClear.Call(uintptr(unsafe.Pointer(&variant[0])))
	if uint16(variant[0]) != variantTypeBool {
		return false
	}
	return int16(variant[1]) != 0
}

// uiaAnswer is one read, addressed back to the caller that asked.
type uiaAnswer struct {
	text string
	ok   bool
}

// uiaReader owns the automation client on a thread of its own. COM objects are
// apartment bound, so the object may not be touched from another thread, and a
// call into a busy application must not block the polling loop for good: the
// caller gives up after uiaTimeout and the reader catches up on the next tick.
type uiaReader struct {
	once sync.Once
	reqs chan chan uiaAnswer
}

const uiaTimeout = 2 * time.Second

var uia = &uiaReader{reqs: make(chan chan uiaAnswer, 1)}

func (r *uiaReader) start() {
	r.once.Do(func() { go r.loop() })
}

func (r *uiaReader) loop() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if hr, _, _ := procCoInitializeEx.Call(0, coinitMultithreaded); int32(hr) < 0 {
		log.Printf("transdict: 自动划词: UI Automation 初始化失败 (0x%08X)", uint32(hr))
		return
	}
	defer procCoUninitialize.Call()
	client, err := newAutomation()
	if err != nil {
		log.Printf("transdict: 自动划词: %v", err)
		return
	}
	for reply := range r.reqs {
		text, ok := client.selection()
		reply <- uiaAnswer{text: text, ok: ok}
	}
}

// uiaSelection is the one call SelectedText makes. A read already in flight
// means the previous answer has not arrived yet, which is reported as unknown
// rather than queueing behind it.
func uiaSelection() (string, bool) {
	uia.start()
	reply := make(chan uiaAnswer, 1)
	select {
	case uia.reqs <- reply:
	default:
		return "", false
	}
	select {
	case answer := <-reply:
		return answer.text, answer.ok
	case <-time.After(uiaTimeout):
		return "", false
	}
}
