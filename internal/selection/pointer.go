package selection

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"
)

// CGEventFlags, from <CoreGraphics/CGEventTypes.h>.
const (
	pointerFlagShift     uint64 = 1 << 17
	pointerFlagControl   uint64 = 1 << 18
	pointerFlagAlternate uint64 = 1 << 19
	pointerFlagCommand   uint64 = 1 << 20
	pointerFlagFunction  uint64 = 1 << 23
)

// kVK_ANSI_A: the select-all key.
const keyCodeA = 0

// pointerDragEvents is how many drag events a release needs before it counts as
// a selection drag. A twitch of the hand is not one, and selecting a word with a
// double click is not one either.
const pointerDragEvents = 3

// selectGesture is the gesture that asked for a read.
type selectGesture int

const (
	selectNone selectGesture = iota
	selectDrag
	selectDoubleClick
	selectTripleClick
	selectShiftClick
	selectAll
)

// pointerTracker turns raw pointer events into at most one gesture per press.
// EasyDict's trigger evaluator makes the same calls: a release is a drag only
// once the events before it were drags, and the click count on the press says
// whether the app is selecting a word (2), a paragraph (3) or extending a
// selection (shift).
type pointerTracker struct {
	pressed bool
	drags   int
}

// press records a left mouse down and answers the click gestures it starts.
func (t *pointerTracker) press(clicks int, shift bool) selectGesture {
	t.pressed, t.drags = true, 0
	switch {
	case clicks >= 3:
		return selectTripleClick
	case clicks == 2:
		return selectDoubleClick
	case shift:
		return selectShiftClick
	}
	return selectNone
}

// dragged counts one drag event; only the fact that the pointer moved with the
// button held matters here.
func (t *pointerTracker) dragged() { t.drags++ }

// release answers whether the press turned into a drag selection.
func (t *pointerTracker) release() selectGesture {
	drags := t.drags
	t.pressed, t.drags = false, 0
	if drags >= pointerDragEvents {
		return selectDrag
	}
	return selectNone
}

// A Chromium application builds its accessibility tree on the first request
// and answers nothing until it is up, so a selection made just after a browser
// launched reads empty. The retries are short and only run after a failed read:
// a normal app answers on the first one.
var readRetryDelay = 250 * time.Millisecond

const readRetryCount = 8

// selectReadDelay is the beat a gesture gets before the selection is read. A
// click makes the app select the word, so reading at the click itself would
// still find the previous selection.
func selectReadDelay(g selectGesture) time.Duration {
	switch g {
	case selectDoubleClick:
		return 200 * time.Millisecond
	case selectTripleClick, selectShiftClick, selectAll:
		return 100 * time.Millisecond
	}
	return 0
}

// isSelectAll reports a plain ⌘A, the chord EasyDict treats as a selection
// gesture. Shift or option alongside it mean something else (⌘⇧A is 取消选择 in
// its rule), so they do not count.
func isSelectAll(keyCode int, flags uint64) bool {
	if keyCode != keyCodeA || flags&pointerFlagCommand == 0 {
		return false
	}
	return flags&(pointerFlagShift|pointerFlagControl|pointerFlagAlternate|pointerFlagFunction) == 0
}

// pointerWatch reads the selection one gesture at a time. A later gesture
// invalidates an earlier read that is still waiting out its delay, so a slow
// double click never answers with the text a newer drag picked.
type pointerWatch struct {
	ctx    context.Context
	report func(string)

	mu   sync.Mutex
	edge pointerTracker
	gen  uint64
	// wg counts the reads still in flight, so a stopping watcher can wait for
	// them instead of racing the globals they call into.
	wg sync.WaitGroup
	// unreadable names the applications a gesture already failed to read, so
	// the log names the app instead of repeating the same line on every
	// selection.
	unreadable map[string]bool
}

func newPointerWatch(ctx context.Context, report func(string)) *pointerWatch {
	return &pointerWatch{ctx: ctx, report: report, unreadable: map[string]bool{}}
}

func (w *pointerWatch) press(clicks int, shift bool) {
	w.mu.Lock()
	g := w.edge.press(clicks, shift)
	w.mu.Unlock()
	w.fire(g)
}

func (w *pointerWatch) dragged() {
	w.mu.Lock()
	w.edge.dragged()
	w.mu.Unlock()
}

func (w *pointerWatch) release() {
	w.mu.Lock()
	g := w.edge.release()
	w.mu.Unlock()
	w.fire(g)
}

func (w *pointerWatch) fire(g selectGesture) {
	if g == selectNone {
		return
	}
	w.mu.Lock()
	w.gen++
	gen := w.gen
	w.mu.Unlock()
	delay := selectReadDelay(g)
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		if delay > 0 {
			timer := time.NewTimer(delay)
			defer timer.Stop()
			select {
			case <-timer.C:
			case <-w.ctx.Done():
				return
			}
		}
		w.mu.Lock()
		stale := gen != w.gen
		w.mu.Unlock()
		// Our own windows are not a source: highlighting a result would ask
		// about the text that query just produced.
		if stale || ownForeground() {
			return
		}
		source := FrontmostBundleID()
		text, ok := w.read(gen)
		if !ok {
			w.noteUnreadable(source)
			return
		}
		if text == "" {
			return
		}
		w.report(text)
	}()
}

// noteUnreadable records, once per application, that a gesture produced no
// readable selection. The line is what turns "划词没反应" into an application
// name the next time someone reads the log.
func (w *pointerWatch) noteUnreadable(id string) {
	if id == "" {
		return
	}
	w.mu.Lock()
	seen := w.unreadable[id]
	w.unreadable[id] = true
	w.mu.Unlock()
	if !seen {
		log.Printf("transdict: 自动划词: 读不到 %s 的选区", id)
	}
}

// read answers the gesture: accessibility first, then the browser's scripting
// interface, which is the only route Safari has. A failed accessibility read is
// retried briefly, because a Chromium app that has just started builds its
// accessibility tree on demand and answers nothing until it exists.
func (w *pointerWatch) read(gen uint64) (string, bool) {
	if text, ok := readNative(); ok {
		return strings.TrimSpace(text), true
	}
	if preferScript {
		if text, ok := readScript(); ok {
			return strings.TrimSpace(text), true
		}
	}
	for range readRetryCount {
		select {
		case <-w.ctx.Done():
			return "", false
		case <-time.After(readRetryDelay):
		}
		w.mu.Lock()
		stale := gen != w.gen
		w.mu.Unlock()
		if stale {
			return "", false
		}
		if text, ok := readNative(); ok {
			return strings.TrimSpace(text), true
		}
	}
	return "", false
}

// wait blocks until every read in flight has finished; the watcher's tests use
// it to read the verdict without sleeping.
func (w *pointerWatch) wait() { w.wg.Wait() }
