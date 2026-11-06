package selection

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakePasteboard struct {
	text    string
	written []string
}

func (f *fakePasteboard) Read() (string, bool) { return f.text, f.text != "" }
func (f *fakePasteboard) Write(text string) bool {
	f.written = append(f.written, text)
	f.text = text
	return true
}

// withHooks swaps the platform calls for the duration of one test.
func withHooks(t *testing.T, native func() (string, bool), cp func() error, count func() int64) {
	t.Helper()
	originalNative, originalCopy, originalCount, originalWait := readNative, postCopy, countChanges, waitFunc
	readNative, postCopy, countChanges = native, cp, count
	waitFunc = func(time.Duration) {}
	t.Cleanup(func() {
		readNative, postCopy, countChanges, waitFunc = originalNative, originalCopy, originalCount, originalWait
	})
}

func TestGrabPrefersAccessibility(t *testing.T) {
	withHooks(t, func() (string, bool) { return "  hello  ", true },
		func() error { t.Fatal("must not touch the pasteboard"); return nil },
		func() int64 { return 0 })

	text, err := Grab(&fakePasteboard{})
	if err != nil {
		t.Fatalf("Grab: %v", err)
	}
	if text != "hello" {
		t.Errorf("text = %q, want the trimmed accessibility answer", text)
	}
}

func TestGrabEmptySelectionIsAnAnswer(t *testing.T) {
	withHooks(t, func() (string, bool) { return "", true },
		func() error { t.Fatal("must not synthesize a copy"); return nil },
		func() int64 { return 0 })

	if _, err := Grab(&fakePasteboard{}); !errors.Is(err, ErrNothingSelected) {
		t.Fatalf("err = %v, want ErrNothingSelected", err)
	}
}

func TestGrabFallsBackToCopyAndRestores(t *testing.T) {
	pb := &fakePasteboard{text: "old note"}
	count := int64(1)
	withHooks(t,
		func() (string, bool) { return "", false },
		func() error { pb.text = "chosen words"; count = 2; return nil },
		func() int64 { return count })

	text, err := Grab(pb)
	if err != nil {
		t.Fatalf("Grab: %v", err)
	}
	if text != "chosen words" {
		t.Errorf("text = %q", text)
	}
	if len(pb.written) != 1 || pb.written[0] != "old note" {
		t.Errorf("restore = %v, want the original pasteboard back", pb.written)
	}
}

func TestGrabWithoutSequenceCounterComparesText(t *testing.T) {
	pb := &fakePasteboard{text: ""}
	steps := 0
	withHooks(t,
		func() (string, bool) { return "", false },
		func() error { steps++; pb.text = "typed selection"; return nil },
		func() int64 { return -1 })

	text, err := Grab(pb)
	if err != nil || text != "typed selection" {
		t.Fatalf("Grab = %q, %v", text, err)
	}
}

func TestGrabTimesOutWhenNothingChanged(t *testing.T) {
	pb := &fakePasteboard{text: "unchanged"}
	withHooks(t,
		func() (string, bool) { return "", false },
		func() error { return nil },
		func() int64 { return 7 })

	defer func(window time.Duration) { copyTimeout = window }(copyTimeout)
	copyTimeout = time.Millisecond

	if _, err := Grab(pb); !errors.Is(err, ErrTimeout) {
		t.Fatalf("err = %v, want ErrTimeout", err)
	}
	if pb.text != "unchanged" {
		t.Errorf("pasteboard = %q, want it untouched", pb.text)
	}
}

func TestGrabReportsCopyFailure(t *testing.T) {
	withHooks(t,
		func() (string, bool) { return "", false },
		func() error { return errors.New("sendinput rejected") },
		func() int64 { return 1 })

	if _, err := Grab(&fakePasteboard{}); err == nil || !strings.Contains(err.Error(), "sendinput") {
		t.Fatalf("err = %v", err)
	}
}

// selector replays a scripted list of accessibility answers and then reports an
// empty selection forever, which is what a user does after dragging a highlight.
type selector struct {
	mu      sync.Mutex
	answers []string
	i       int
}

func (s *selector) read() (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.i < len(s.answers) {
		s.i++
		return s.answers[s.i-1], true
	}
	return "", true
}

type reports struct {
	mu   sync.Mutex
	text []string
	owns int
}

func (r *reports) add(text string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.text = append(r.text, text)
}

func (r *reports) all() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.text...)
}

func withPoll(t *testing.T, sel func() (string, bool), canPoll bool, isSelf func() bool) {
	t.Helper()
	originalNative, originalPollable, originalSelf := readNative, pollable, ownForeground
	readNative, pollable, ownForeground = sel, canPoll, isSelf
	t.Cleanup(func() {
		readNative, pollable, ownForeground = originalNative, originalPollable, originalSelf
	})
}

func TestWatchReportsEachNewSelectionOnce(t *testing.T) {
	// Each highlight is read twice: the watcher waits a whole interval for the
	// selection to settle before it spends a query on it.
	sel := &selector{answers: []string{"alpha", "alpha", "beta", "beta", "", "beta", "beta"}}
	out := &reports{}
	withPoll(t, sel.read, true, func() bool { return false })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = Watch(ctx, time.Millisecond, out.add) }()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && len(out.all()) < 3 {
		time.Sleep(time.Millisecond)
	}
	if got := out.all(); len(got) != 3 || got[0] != "alpha" || got[1] != "beta" || got[2] != "beta" {
		t.Fatalf("reports = %q, want alpha, beta, beta", got)
	}
}

func TestWatchStaysQuietWhileOwnAppIsFrontmost(t *testing.T) {
	sel := &selector{answers: []string{"alpha", "beta", "gamma"}}
	out := &reports{}
	withPoll(t, sel.read, true, func() bool {
		out.mu.Lock()
		defer out.mu.Unlock()
		out.owns++
		return true
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = Watch(ctx, time.Millisecond, out.add) }()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		out.mu.Lock()
		owns := out.owns
		out.mu.Unlock()
		if owns >= 2 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if got := out.all(); len(got) != 0 {
		t.Fatalf("reports = %q, want none: a highlight in our own panel is not a query", got)
	}
}

func TestWatchDeclinesWherePollingNeedsTheClipboard(t *testing.T) {
	withPoll(t, func() (string, bool) { return "alpha", true }, false, func() bool { return false })

	err := Watch(context.Background(), time.Millisecond, func(string) {})
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("err = %v, want ErrUnsupported", err)
	}
}

func TestWatchReturnsWhenCancelled(t *testing.T) {
	sel := &selector{answers: []string{"alpha", "beta"}}
	withPoll(t, sel.read, true, func() bool { return false })

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Watch(ctx, time.Millisecond, func(string) {}) }()
	time.Sleep(5 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("err = %v, want a clean stop", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Watch ignored cancellation")
	}
}
