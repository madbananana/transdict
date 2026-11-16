package selection

import (
	"context"
	"testing"
	"time"
)

func TestPointerTrackerGestures(t *testing.T) {
	t.Run("a click selects nothing", func(t *testing.T) {
		var tr pointerTracker
		if g := tr.press(1, false); g != selectNone {
			t.Fatalf("press = %v, want none", g)
		}
		tr.dragged()
		tr.dragged()
		if g := tr.release(); g != selectNone {
			t.Fatalf("release after two drags = %v, want none", g)
		}
	})

	t.Run("a drag selects on release", func(t *testing.T) {
		var tr pointerTracker
		tr.press(1, false)
		tr.dragged()
		tr.dragged()
		tr.dragged()
		if g := tr.release(); g != selectDrag {
			t.Fatalf("release = %v, want drag", g)
		}
	})

	t.Run("double and triple clicks", func(t *testing.T) {
		var tr pointerTracker
		if g := tr.press(2, false); g != selectDoubleClick {
			t.Fatalf("double press = %v, want double", g)
		}
		if g := tr.press(3, false); g != selectTripleClick {
			t.Fatalf("triple press = %v, want triple", g)
		}
	})

	t.Run("a shift click extends", func(t *testing.T) {
		var tr pointerTracker
		if g := tr.press(1, true); g != selectShiftClick {
			t.Fatalf("shift press = %v, want shift", g)
		}
	})

	t.Run("the drag count does not survive a release", func(t *testing.T) {
		var tr pointerTracker
		tr.press(1, false)
		tr.dragged()
		tr.dragged()
		tr.dragged()
		tr.release()
		tr.press(1, false)
		if g := tr.release(); g != selectNone {
			t.Fatalf("second release = %v, want none", g)
		}
	})
}

func TestSelectReadDelay(t *testing.T) {
	cases := map[selectGesture]time.Duration{
		selectDrag:        0,
		selectDoubleClick: 200 * time.Millisecond,
		selectTripleClick: 100 * time.Millisecond,
		selectShiftClick:  100 * time.Millisecond,
		selectAll:         100 * time.Millisecond,
	}
	for g, want := range cases {
		if got := selectReadDelay(g); got != want {
			t.Errorf("selectReadDelay(%v) = %v, want %v", g, got, want)
		}
	}
}

func TestIsSelectAll(t *testing.T) {
	cases := []struct {
		name    string
		keyCode int
		flags   uint64
		want    bool
	}{
		{"⌘A", keyCodeA, pointerFlagCommand, true},
		{"a alone", keyCodeA, 0, false},
		{"another key with ⌘", 8, pointerFlagCommand, false},
		{"⌘⇧A is not select all", keyCodeA, pointerFlagCommand | pointerFlagShift, false},
		{"⌘⌥A is not select all", keyCodeA, pointerFlagCommand | pointerFlagAlternate, false},
	}
	for _, c := range cases {
		if got := isSelectAll(c.keyCode, c.flags); got != c.want {
			t.Errorf("%s: isSelectAll(%d, %d) = %v, want %v", c.name, c.keyCode, c.flags, got, c.want)
		}
	}
}

func TestPointerWatchReportsTheSelection(t *testing.T) {
	withPoll(t, func() (string, bool) { return "  highlighted  ", true }, true, func() bool { return false })

	out := &reports{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := newPointerWatch(ctx, out.add)

	w.press(1, false)
	w.dragged()
	w.dragged()
	w.dragged()
	w.release()
	w.wait()

	got := out.all()
	if len(got) != 1 || got[0] != "highlighted" {
		t.Fatalf("reports = %q, want the trimmed selection", got)
	}
}

func TestPointerWatchSkipsOurOwnWindow(t *testing.T) {
	withPoll(t, func() (string, bool) { return "our own result", true }, true, func() bool { return true })

	out := &reports{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := newPointerWatch(ctx, out.add)

	w.press(1, false)
	w.dragged()
	w.dragged()
	w.dragged()
	w.release()
	w.wait()

	if got := out.all(); len(got) != 0 {
		t.Fatalf("reports = %q, want none while our own window is frontmost", got)
	}
}

func TestPointerWatchDropsAStaleDelayedRead(t *testing.T) {
	withPoll(t, func() (string, bool) { return "late", true }, true, func() bool { return false })

	out := &reports{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := newPointerWatch(ctx, out.add)

	// A triple click waits out its delay, then a drag replaces it; only the
	// newer gesture may answer.
	w.press(3, false)
	w.release()
	w.fire(selectDrag)
	w.wait()

	got := out.all()
	if len(got) != 1 {
		t.Fatalf("reports = %q, want exactly the drag read", got)
	}
}
