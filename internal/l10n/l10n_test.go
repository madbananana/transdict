package l10n

import "testing"

// restore leaves the package in the state the rest of the suite expects, since
// lang is process-wide and these tests deliberately move it.
func restore(t *testing.T) {
	t.Helper()
	before := Lang()
	t.Cleanup(func() { Set(before) })
}

func TestSetNormalizesAndReportsChange(t *testing.T) {
	restore(t)
	Set(zhCN)

	if Set("english") {
		t.Error("already Chinese, so an unrecognised name should not count as a change")
	}
	if Lang() != zhCN {
		t.Fatalf("Lang() = %q, want %q", Lang(), zhCN)
	}
	if !Set("en") {
		t.Fatal("switching out of the source language should report a change")
	}
	if Lang() != "en" {
		t.Fatalf("Lang() = %q, want en", Lang())
	}
	// Anything Go does not recognise is the source language, so a typo cannot
	// leave the app half-translated — here that means a real change back to Chinese.
	if !Set("english") {
		t.Error("an unrecognised name should fall back to Chinese, which is a change")
	}
	if Lang() != zhCN {
		t.Fatalf("Lang() = %q, want %q", Lang(), zhCN)
	}
	if Set(zhCN) {
		t.Error("re-asserting the language already in use is not a change")
	}
}

func TestTFallsBackToTheSourceText(t *testing.T) {
	restore(t)
	Set(zhCN)
	if got := T("苹果词典"); got != "苹果词典" {
		t.Errorf("Chinese mode T(%q) = %q, want it untouched", "苹果词典", got)
	}

	Set("en")
	if got := T("苹果词典"); got != "Apple Dictionary" {
		t.Errorf("English mode T(%q) = %q, want %q", "苹果词典", got, "Apple Dictionary")
	}
	// A name the user typed is not in the table, so it must survive rather than
	// turn into a blank label.
	if got := T("Google"); got != "Google" {
		t.Errorf("unknown text T(%q) = %q, want it untouched", "Google", got)
	}
}

func TestZhIsTBackwards(t *testing.T) {
	restore(t)

	// Outside English mode nothing was ever translated, so nothing to undo: a
	// settings save has to be able to tell that apart from a real label.
	Set(zhCN)
	if got := Zh("Apple Dictionary"); got != "" {
		t.Errorf("Zh() in Chinese mode = %q, want empty", got)
	}

	Set("en")
	for _, zh := range []string{"苹果词典", "苹果 OCR", "AI 模型", "苹果朗读"} {
		english := T(zh)
		if english == zh {
			t.Fatalf("%q has no English entry, so the round trip proves nothing", zh)
		}
		if back := Zh(english); back != zh {
			t.Errorf("Zh(T(%q)) = %q, want %q", zh, back, zh)
		}
	}
	if got := Zh("Google"); got != "" {
		t.Errorf("Zh() for text that was never ours = %q, want empty", got)
	}
}

func TestOnChangeRunsOnlyForRealChanges(t *testing.T) {
	restore(t)
	Set(zhCN)

	runs := 0
	OnChange(func() { runs++ })

	Set("en")
	if runs != 1 {
		t.Fatalf("runs = %d, want 1 after a real switch", runs)
	}
	Set("en")
	if runs != 1 {
		t.Fatalf("runs = %d, want no redraw for the language already shown", runs)
	}
	Set(zhCN)
	if runs != 2 {
		t.Fatalf("runs = %d, want 2 after switching back", runs)
	}
}
