// Package selection reads text the user highlighted in another application,
// which is the input for 划词翻译. It offers three routes because none is
// universally available: an accessibility read of the focused element (macOS),
// the browser's own AppleScript interface (Safari and Chromium browsers), and a
// synthesized copy watched through the pasteboard (everywhere).
package selection

import (
	"context"
	"errors"
	"strings"
	"time"
)

var (
	// ErrNothingSelected means the read worked but nothing was highlighted.
	ErrNothingSelected = errors.New("没有选中内容")
	// ErrTimeout is the copy chord landing nowhere: no pasteboard change.
	ErrTimeout = errors.New("未读取到选区，请在目标应用中重新选择文字")
	// ErrUnsupported means this platform offers neither route.
	ErrUnsupported = errors.New("取词：当前系统不支持读取其他应用的选区")
	// ErrNoAccess is an app the accessibility API cannot read while 强制取词 is
	// switched off: without it there is no second route to fall back to.
	ErrNoAccess = errors.New("取词：当前应用不支持读取选区；可开启「启用强制取词」再试")
)

// copyTimeout bounds how long the pasteboard is watched after the copy chord. A
// remote desktop or a heavy Electron app can take a few hundred milliseconds
// before the pasteboard actually changes, so this is patience, not a fast poll.
var copyTimeout = 1200 * time.Millisecond

// menuTimeout is the shorter wait given to the menu route: when the app has no
// Copy item the press itself already failed, and when it has one but nothing is
// selected the chord below is still worth a try.
var menuTimeout = 700 * time.Millisecond

// forceCopy, forceKind and preferScript are 启用强制取词, 强制取词类型 and
// 优先使用 AppleScript API, installed by the app from the settings store. They
// are package variables for the same reason as the hooks below: the
// orchestration has to be testable without a window server.
var (
	forceCopy    = true
	forceKind    = "menu"
	preferScript = true
)

// Configure installs 启用强制取词, 强制取词类型 and 优先使用 AppleScript API.
// kind is "menu" (the app's own Copy command) or "shortcut" (the synthesized
// chord).
func Configure(force bool, kind string, preferAppleScript bool) {
	forceCopy = force
	forceKind = kind
	preferScript = preferAppleScript
}

// Pasteboard is the caller's view of the system clipboard. Selection stays free
// of the GUI toolkit by taking it as an argument, which is also what makes the
// fallback path testable.
type Pasteboard interface {
	Read() (string, bool)
	Write(text string) bool
}

// Hooks are variables so the orchestration below is testable without a window
// server, a foreground application or a real pasteboard.
var (
	readNative    = SelectedText
	readScript    = BrowserSelectedText
	postCopy      = Copy
	pressCopyMenu = CopyMenu
	countChanges  = ChangeCount
	ownForeground = FrontmostIsSelf
	pollable      = Pollable
	waitFunc      = func(d time.Duration) { time.Sleep(d) }
)

// Grab returns the highlighted text. The accessibility read comes first because
// it has no side effects; when it cannot answer, the browser's scripting
// interface gets a turn — Safari does not expose a selection to accessibility at
// all. Only then does 强制取词 rewrite the pasteboard, which is why that last
// route never runs in the background watcher.
func Grab(pb Pasteboard) (string, error) {
	if text, ok := readNative(); ok {
		text = strings.TrimSpace(text)
		if text == "" {
			return "", ErrNothingSelected
		}
		return text, nil
	}
	// A browser that answers its script has spoken for the selection, including
	// when the answer is empty: that is a page with no highlight, not a route
	// that failed. A script error means the route is unavailable (non-browser,
	// no Automation permission, or JavaScript from Apple Events is switched off)
	// and the copy routes take over. 优先使用 AppleScript API switches the route
	// off entirely; the copy routes then answer as they always did.
	if preferScript {
		if text, ok := readScript(); ok {
			text = strings.TrimSpace(text)
			if text == "" {
				return "", ErrNothingSelected
			}
			return text, nil
		}
	}
	if pb == nil {
		return "", ErrUnsupported
	}
	if !forceCopy {
		return "", ErrNoAccess
	}

	original, hadOriginal := pb.Read()
	before := countChanges()

	text := ""
	if forceKind == "menu" {
		if err := pressCopyMenu(); err == nil {
			text = awaitPasteboard(pb, original, before, menuTimeout)
		}
	}
	if text == "" {
		if err := postCopy(); err != nil {
			return "", err
		}
		text = awaitPasteboard(pb, original, before, copyTimeout)
	}

	// Put the user's own copy back. Restoring before returning keeps the window
	// between the chord and the restore too short to paste into by accident.
	if hadOriginal && original != "" {
		pb.Write(original)
	}
	if text == "" {
		return "", ErrTimeout
	}
	return text, nil
}

// awaitPasteboard watches for the copy to land. A pasteboard sequence counter is
// preferred because it also fires when the selection was the same text that was
// already stored; comparing the text is the fallback for platforms whose
// counter is unavailable.
func awaitPasteboard(pb Pasteboard, original string, before int64, limit time.Duration) string {
	deadline := time.Now().Add(limit)
	for {
		if changed(pb, original, before) {
			candidate, ok := pb.Read()
			if !ok {
				return ""
			}
			return strings.TrimSpace(candidate)
		}
		if !time.Now().Before(deadline) {
			return ""
		}
		waitFunc(20 * time.Millisecond)
	}
}

// changed detects that the copy actually landed. A pasteboard sequence counter
// is preferred because it also fires when the selection was the same text that
// was already stored; comparing the text is the fallback for platforms whose
// counter is unavailable.
func changed(pb Pasteboard, original string, before int64) bool {
	if now := countChanges(); before >= 0 && now >= 0 {
		return now != before
	}
	candidate, ok := pb.Read()
	return ok && strings.TrimSpace(candidate) != strings.TrimSpace(original)
}

// tracker turns a stream of polls into reports. A highlight is reported only
// after it has survived one whole interval unchanged: dragging a selection
// passes through partial reads ("D", "ap", "nnect" on a slow pull) and asking
// every service about each of them costs money and fills 历史 with noise.
type tracker struct {
	last     string
	reported string
}

// observe records what this poll read and answers whether it should be
// translated now. Clearing the highlight resets both fields, so selecting the
// same words again after a click elsewhere asks again.
func (t *tracker) observe(text string) bool {
	if text == "" {
		t.last, t.reported = "", ""
		return false
	}
	if text != t.last {
		t.last = text
		return false
	}
	if text == t.reported {
		return false
	}
	t.reported = text
	return true
}

// Watch reports each newly highlighted text once, until ctx is cancelled. It is
// the 自动鼠标划词 route, and it deliberately reads without side effects: the
// accessibility API first, then a browser's scripting interface. A background
// loop that synthesized Cmd+C would rewrite the user's pasteboard every time
// they moved a mouse. Platforms whose read needs the clipboard decline instead,
// which is why Pollable exists.
func Watch(ctx context.Context, interval time.Duration, report func(string)) error {
	if !pollable {
		return ErrUnsupported
	}
	if interval <= 0 {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var seen tracker
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			// Our own panel is not a source: highlighting a result would open a
			// new query about the text the query just produced.
			if ownForeground() {
				continue
			}
			text, ok := readNative()
			if !ok && preferScript {
				// Safari never answers the accessibility read; its page scripts
				// can, and that is the only way 自动划词 works there.
				text, ok = readScript()
			}
			if !ok {
				continue
			}
			text = strings.TrimSpace(text)
			if seen.observe(text) {
				report(text)
			}
		}
	}
}
