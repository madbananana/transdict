//go:build windows

package cli

import (
	"log"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/w32"

	"transdict/internal/gui"
)

// Wails v3.0.0 sets NOTIFYICON_VERSION_4, where a left click arrives as
// NIN_SELECT in LOWORD(lParam), but its tray routine only matches
// WM_LBUTTONUP. OnClick therefore never runs on Windows and the left button
// looks dead. Upstream (checked through v3.2.0-nightly) still has this, so the
// hidden tray window is subclassed here: the click is recovered, and every
// other message — including the right-click menu — still goes to Wails.
//
// WM_LBUTTONUP is also matched: NIM_SETVERSION can fail, in which case the
// shell stays in legacy mode and sends the old message instead of NIN_SELECT.
// A double click under version 4 is a WM_LBUTTONDBLCLK of its own, not a second
// NIN_SELECT, so it needs a separate branch; waiting for two clicks never sees
// it.
const (
	ninSelect       = 0x0400
	ninKeySelect    = 0x0401
	wmLButtonUp     = 0x0202
	wmLButtonDblClk = 0x0203
	wmUserSystray   = uint32(w32.WM_USER + 1)
)

var (
	trayMu            sync.Mutex
	trayClickFn       func()
	trayDoubleClickFn func()
	trayOldProcs      = map[uintptr]uintptr{}
	findWindowEx      = syscall.NewLazyDLL("user32.dll").NewProc("FindWindowExW")
	getDoubleClickMs  = syscall.NewLazyDLL("user32.dll").NewProc("GetDoubleClickTime")
)

var trayWndProc = syscall.NewCallback(func(hwnd, msg, wparam, lparam uintptr) uintptr {
	if uint32(msg) == wmUserSystray {
		var fn func()
		trayMu.Lock()
		switch uint16(lparam) {
		case ninSelect, ninKeySelect, wmLButtonUp:
			fn = trayClickFn
		case wmLButtonDblClk:
			fn = trayDoubleClickFn
		}
		trayMu.Unlock()
		if fn != nil {
			fn()
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

func installTrayClick(tray *application.SystemTray, svc *gui.Service) {
	// Registered as well: in legacy mode Wails' own WM_LBUTTONUP and
	// WM_LBUTTONDBLCLK handlers fire too, and the duplicate guards merge them.
	clicker := newTrayClicker(svc)
	tray.OnClick(clicker.click)
	tray.OnDoubleClick(clicker.doubleClick)

	trayMu.Lock()
	trayClickFn = clicker.click
	trayDoubleClickFn = clicker.doubleClick
	trayMu.Unlock()
	// The tray window is created by Wails when the application starts, which is
	// after this recipe runs. Poll briefly instead of racing that sequence.
	go func() {
		for range 100 {
			if hookTrayWindows() {
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
		log.Printf("transdict: 没有找到托盘消息窗口，左键点击将不可用")
	}()
}

// hookTrayWindows subclasses every message-only window whose class comes from
// Wails. The class name is not hard-coded: it comes from
// Options.Windows.WndClass, and matching on the default only would silently
// stop working if that changes.
func hookTrayWindows() bool {
	parent := uintptr(w32.HWND_MESSAGE)
	var after uintptr
	var hooked []string
	for {
		hwnd, _, _ := findWindowEx.Call(parent, after, 0, 0)
		if hwnd == 0 {
			break
		}
		after = hwnd
		class := w32.GetClassName(w32.HWND(hwnd))
		if !strings.Contains(strings.ToLower(class), "wails") {
			continue
		}
		trayMu.Lock()
		_, seen := trayOldProcs[hwnd]
		trayMu.Unlock()
		if seen {
			hooked = append(hooked, class)
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
		hooked = append(hooked, class)
	}
	if len(hooked) > 0 {
		log.Printf("transdict: 已接管托盘窗口 %v", hooked)
	}
	return len(hooked) > 0
}

// trayClicker separates the reports Windows sends for one gesture. A single
// click waits out the double-click window before opening the main window; a
// second click inside it, or a version-4 double-click notification, opens 设置
// instead. Duplicates from the hook and Wails' own handlers are dropped.
type trayClicker struct {
	svc   *gui.Service
	delay time.Duration

	mu            sync.Mutex
	last          time.Time
	suppressUntil time.Time
	settingsAt    time.Time
	pending       *time.Timer
	generation    uint64
}

func newTrayClicker(svc *gui.Service) *trayClicker {
	// Honor the user's double-click speed; Windows defaults to 500 ms.
	delay := 500 * time.Millisecond
	if v, _, _ := getDoubleClickMs.Call(); v > 0 {
		delay = time.Duration(v) * time.Millisecond
	}
	return &trayClicker{svc: svc, delay: delay}
}

func (c *trayClicker) click() {
	now := time.Now()
	c.mu.Lock()
	if now.Before(c.suppressUntil) {
		c.mu.Unlock()
		return
	}
	// The same physical click can arrive through more than one channel: the
	// hook, Wails' legacy handler, and (for a double click) a following button
	// up. Only the first report counts.
	if !c.last.IsZero() && now.Sub(c.last) < 50*time.Millisecond {
		c.mu.Unlock()
		return
	}
	c.generation++
	gen := c.generation
	if !c.last.IsZero() && now.Sub(c.last) < c.delay {
		c.last = time.Time{}
		c.suppressUntil = now.Add(min(c.delay, 600*time.Millisecond))
		c.stopPending()
		c.mu.Unlock()
		c.openSettings()
		return
	}
	c.last = now
	c.stopPending()
	c.pending = time.AfterFunc(c.delay, func() {
		c.mu.Lock()
		if gen != c.generation {
			c.mu.Unlock()
			return
		}
		c.pending = nil
		c.mu.Unlock()
		c.svc.OpenMain()
	})
	c.mu.Unlock()
}

// doubleClick is the version-4 report of a double click. It cancels the single
// click that the first half already scheduled and quiets the trailing button-up
// messages a legacy shell still sends.
func (c *trayClicker) doubleClick() {
	now := time.Now()
	c.mu.Lock()
	c.generation++
	c.last = time.Time{}
	c.suppressUntil = now.Add(min(c.delay, 600*time.Millisecond))
	c.stopPending()
	c.mu.Unlock()
	c.openSettings()
}

func (c *trayClicker) stopPending() {
	if c.pending != nil {
		c.pending.Stop()
		c.pending = nil
	}
}

// openSettings drops a second 设置 request from the same gesture; the hook and
// Wails can both report one double click.
func (c *trayClicker) openSettings() {
	now := time.Now()
	c.mu.Lock()
	if !c.settingsAt.IsZero() && now.Sub(c.settingsAt) < 300*time.Millisecond {
		c.mu.Unlock()
		return
	}
	c.settingsAt = now
	c.mu.Unlock()
	c.svc.OpenSettings()
}
