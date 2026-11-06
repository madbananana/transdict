// Package shortcut maps global accelerators onto application commands.
package shortcut

import (
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"

	"transdict/internal/settings"
)

// Actions are the commands a global accelerator can invoke. A nil handler means
// the build does not offer that action, so its shortcut is skipped rather than
// registered as a key press that silently does nothing.
type Actions struct {
	Capture   func()
	Input     func()
	Selection func()
	SilentOCR func()
	Mini      func()
	Clipboard func()
	Replace   func()
	ODict     func()
	Settings  func()
}

type registration struct {
	accel string
	run   func()
}

// Problem is one key the operating system would not reserve. It stays a value
// rather than a sentence so the preferences window can print it in the
// interface language; Err is the platform's own words, which are not ours to
// translate.
type Problem struct {
	// Accel is the accelerator that failed, empty when clearing the previous
	// set was what failed.
	Accel string `json:"accel"`
	Err   string `json:"err"`
}

// slots pairs each configured action with its handler. Order matters only for
// the problems list, which reads top-down like the settings page.
func slots(a Actions) []struct {
	slot string
	run  func()
} {
	return []struct {
		slot string
		run  func()
	}{
		{settings.SlotInput, a.Input},
		{settings.SlotSelection, a.Selection},
		{settings.SlotCapture, a.Capture},
		{settings.SlotSilentOCR, a.SilentOCR},
		{settings.SlotMini, a.Mini},
		{settings.SlotClipboard, a.Clipboard},
		{settings.SlotReplace, a.Replace},
		{settings.SlotODict, a.ODict},
		{settings.SlotSettings, a.Settings},
	}
}

// planned turns configuration into the list of handlers to install. Keeping it
// separate from Bind makes the mapping testable without a running app.
func planned(cfg settings.Config, a Actions) []registration {
	out := make([]registration, 0, len(settings.SlotLabels))
	for _, s := range slots(a) {
		if s.run == nil {
			continue
		}
		for _, accel := range cfg.Accel(s.slot) {
			accel = strings.TrimSpace(accel)
			if accel == "" {
				continue
			}
			out = append(out, registration{accel, s.run})
		}
	}
	return out
}

// Bind replaces the whole registration set and reports what failed, normally
// because another app already owns the combination.
func Bind(app *application.App, cfg settings.Config, a Actions) []Problem {
	var problems []Problem
	if err := app.GlobalShortcut.UnregisterAll(); err != nil {
		problems = append(problems, Problem{Err: err.Error()})
	}
	for _, r := range planned(cfg, a) {
		run := r.run
		if err := app.GlobalShortcut.Register(r.accel, func() { run() }); err != nil {
			problems = append(problems, Problem{Accel: r.accel, Err: err.Error()})
		}
	}
	return problems
}

// Conflicts reports accelerators bound to more than one action, which the
// settings page warns about before saving.
func Conflicts(cfg settings.Config) []string {
	seen := map[string]string{}
	var out []string
	for _, slot := range []string{settings.SlotInput, settings.SlotSelection, settings.SlotCapture,
		settings.SlotSilentOCR, settings.SlotMini, settings.SlotClipboard, settings.SlotReplace,
		settings.SlotSettings} {
		for _, accel := range cfg.Accel(slot) {
			accel = strings.ToLower(strings.TrimSpace(accel))
			if accel == "" {
				continue
			}
			if other, ok := seen[accel]; ok {
				out = append(out, Display(accel)+" 同时绑定在 "+other+" 与 "+slot)
				continue
			}
			seen[accel] = slot
		}
	}
	return out
}
