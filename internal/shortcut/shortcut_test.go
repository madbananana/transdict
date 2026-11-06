package shortcut

import (
	"strings"
	"testing"

	"transdict/internal/settings"
)

func TestPlannedSkipsUnboundActions(t *testing.T) {
	cfg := settings.Default()
	called := 0
	a := Actions{Capture: func() { called++ }}

	got := planned(cfg, a)
	if len(got) != len(cfg.Accel(settings.SlotCapture)) {
		t.Fatalf("planned = %d registrations, want only capture's", len(got))
	}
	for i, r := range got {
		if r.accel != cfg.Accel(settings.SlotCapture)[i] {
			t.Errorf("accel = %q, want %q", r.accel, cfg.Accel(settings.SlotCapture)[i])
		}
	}
	got[0].run()
	if called != 1 {
		t.Errorf("handler ran %d times, want 1", called)
	}
}

func TestPlannedSkipsBlankAccelerator(t *testing.T) {
	cfg := settings.Default()
	cfg.Shortcuts[settings.SlotCapture] = []string{"  "}
	want := len(cfg.Accel(settings.SlotInput))
	if n := len(planned(cfg, Actions{Capture: func() {}, Input: func() {}})); n != want {
		t.Errorf("planned = %d, want %d (only the input shortcuts survive)", n, want)
	}
}

func TestPlannedRegistersEveryAccelOfASlot(t *testing.T) {
	cfg := settings.Default()
	cfg.Shortcuts[settings.SlotCapture] = []string{"ctrl+alt+1", "ctrl+alt+2"}
	got := planned(cfg, Actions{Capture: func() {}})
	if len(got) != 2 {
		t.Fatalf("planned = %d, want 2", len(got))
	}
}

func TestConflictsNamesBothSlots(t *testing.T) {
	cfg := settings.Default()
	cfg.Shortcuts[settings.SlotInput] = []string{"alt+shift+t"}
	cfg.Shortcuts[settings.SlotCapture] = []string{"alt+shift+t"}

	problems := Conflicts(cfg)
	if len(problems) != 1 {
		t.Fatalf("Conflicts = %v, want one collision", problems)
	}
	for _, want := range []string{settings.SlotInput, settings.SlotCapture} {
		if !strings.Contains(problems[0], want) {
			t.Errorf("problem %q should name %q", problems[0], want)
		}
	}
}
