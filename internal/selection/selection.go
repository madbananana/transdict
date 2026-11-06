// Package selection reads text the user highlighted in another application,
// which is the input for 划词翻译. It offers two routes because neither is
// universally available: an accessibility read of the focused element (macOS)
// and a synthesized copy chord watched through the pasteboard (everywhere).
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
)

// copyTimeout bounds how long the pasteboard is watched after the copy chord. A
// remote desktop or a heavy Electron app can take a few hundred milliseconds
// before the pasteboard actually changes, so this is patience, not a fast poll.
var copyTimeout = 1200 * time.Millisecond

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
	postCopy      = Copy
	countChanges  = ChangeCount
	ownForeground = FrontmostIsSelf
	pollable      = Pollable
	waitFunc      = func(d time.Duration) { time.Sleep(d) }
)

// Grab returns the highlighted text. The accessibility read comes first because
// it has no side effects; the copy chord only runs when that read cannot answer,
// since it rewrites the user's pasteboard.
func Grab(pb Pasteboard) (string, error) {
	if text, ok := readNative(); ok {
		text = strings.TrimSpace(text)
		if text == "" {
			return "", ErrNothingSelected
		}
		return text, nil
	}
	if pb == nil {
		return "", ErrUnsupported
	}

	original, hadOriginal := pb.Read()
	before := countChanges()
	if err := postCopy(); err != nil {
		return "", err
	}

	deadline := time.Now().Add(copyTimeout)
	text := ""
	for {
		if changed(pb, original, before) {
			if candidate, ok := pb.Read(); ok {
				text = strings.TrimSpace(candidate)
			}
			break
		}
		if !time.Now().Before(deadline) {
			break
		}
		waitFunc(20 * time.Millisecond)
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
// the 自动鼠标划词 route, and it deliberately reads only the accessibility API:
// a background loop that synthesized Cmd+C would rewrite the user's pasteboard
// every time they moved a mouse. Platforms whose read needs the clipboard
// decline instead, which is why Pollable exists.
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
