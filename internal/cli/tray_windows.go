//go:build windows

package cli

import (
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/wailsapp/wails/v3/pkg/w32"

	"transdict/internal/gui"
)

// Wails v3.0.0 sets NOTIFYICON_VERSION_4, where a left click arrives as
// NIN_SELECT in LOWORD(lParam), but its tray routine only matches
// WM_LBUTTONUP. OnClick therefore never runs on Windows and the left button
// looks dead. Upstream (checked through v3.2.0-nightly) still has this, so the
// hidden tray window is subclassed here: the click is recovered, and every
// other message — including the right-click menu — still goes to Wails.
const (
	ninSelect     = 0x0400
	ninKeySelect  = 0x0401
	wmUserSystray = uint32(w32.WM_USER + 1)
	// wndClassName is application.Options.Windows.WndClass's default; this app
	// does not change it.
	wndClassName = "WailsWebviewWindow"
)

var (
	trayMu           sync.Mutex
	trayClickFn      func()
	trayOldProcs     = map[uintptr]uintptr{}
	findWindowEx     = syscall.NewLazyDLL("user32.dll").NewProc("FindWindowExW")
	getDoubleClickMs = syscall.NewLazyDLL("user32.dll").NewProc("GetDoubleClickTime")
)

var trayWndProc = syscall.NewCallback(func(hwnd, msg, wparam, lparam uintptr) uintptr {
	if uint32(msg) == wmUserSystray {
		switch uint16(lparam) {
		case ninSelect, ninKeySelect:
			trayMu.Lock()
			fn := trayClickFn
			trayMu.Unlock()
			if fn != nil {
				fn()
			}
		}
	}
	trayMu.Lock()
	old := trayOldProcs[hwnd]
	trayMu.Unlock()
	if old == 0 {
		return 0
	}
	return w32.CallWindowProc(old, w32.HWND(hwnd), uint32(msg), wparam, lparam)
})

func installTrayClick(svc *gui.Service) {
	trayMu.Lock()
	trayClickFn = trayClickHandler(svc)
	trayMu.Unlock()
	// The tray window is created by Wails when the application starts, which is
	// after this recipe runs. Poll briefly instead of racing that sequence.
	go func() {
		for range 50 {
			if hookTrayWindow() {
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
	}()
}

func hookTrayWindow() bool {
	class, err := syscall.UTF16PtrFromString(wndClassName)
	if err != nil {
		return false
	}
	parent := uintptr(w32.HWND_MESSAGE)
	var after uintptr
	hooked := false
	for {
		hwnd, _, _ := findWindowEx.Call(parent, after, uintptr(unsafe.Pointer(class)), 0)
		if hwnd == 0 {
			break
		}
		after = hwnd
		trayMu.Lock()
		_, seen := trayOldProcs[hwnd]
		trayMu.Unlock()
		if seen {
			hooked = true
			continue
		}
		old := w32.GetWindowLongPtr(w32.HWND(hwnd), w32.GWLP_WNDPROC)
		if old == 0 {
			continue
		}
		// Record the old procedure before the swap: a message delivered between
		// the two calls then still finds its way back to Wails.
		trayMu.Lock()
		trayOldProcs[hwnd] = old
		trayMu.Unlock()
		w32.SetWindowLongPtr(w32.HWND(hwnd), w32.GWLP_WNDPROC, trayWndProc)
		hooked = true
	}
	return hooked
}

// trayClickHandler separates the two answers Windows sends through the same
// NIN_SELECT event. A single click waits out the double-click window before
// opening the main window; a second click inside it opens 设置 instead.
func trayClickHandler(svc *gui.Service) func() {
	// Honor the user's double-click speed; Windows defaults to 500 ms.
	delay := 500 * time.Millisecond
	if v, _, _ := getDoubleClickMs.Call(); v > 0 {
		delay = time.Duration(v) * time.Millisecond
	}
	var mu sync.Mutex
	var last time.Time
	var pending *time.Timer
	var generation uint64
	return func() {
		now := time.Now()
		mu.Lock()
		generation++
		gen := generation
		double := !last.IsZero() && now.Sub(last) < delay
		last = now
		if double {
			last = time.Time{}
			if pending != nil {
				pending.Stop()
				pending = nil
			}
			mu.Unlock()
			svc.OpenSettings()
			return
		}
		if pending != nil {
			pending.Stop()
		}
		pending = time.AfterFunc(delay, func() {
			mu.Lock()
			if gen != generation {
				mu.Unlock()
				return
			}
			pending = nil
			mu.Unlock()
			svc.OpenMain()
		})
		mu.Unlock()
	}
}
