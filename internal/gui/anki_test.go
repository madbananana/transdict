package gui

import (
	"strings"
	"testing"

	"transdict/internal/anki"
	"transdict/internal/settings"
)

// 预览 needs a word and a dictionary: an empty box or a stale provider id is a
// settings-page mistake, and both must be named instead of querying nothing.
func TestAnkiPreviewNeedsAWordAndADictionary(t *testing.T) {
	svc := &Service{cfg: testStore(t)}

	if _, err := svc.AnkiPreview("dict-apple", "   "); err == nil || !strings.Contains(err.Error(), "请输入要预览的单词") {
		t.Errorf("空单词 = %v, want 请输入要预览的单词", err)
	}
	if _, err := svc.AnkiPreview("missing", "hello"); err == nil || !strings.Contains(err.Error(), "请选择一个词典服务") {
		t.Errorf("未知服务 = %v, want 请选择一个词典服务", err)
	}
}

// 添加到 Anki refuses a half-filled destination instead of letting a cleared
// deck or model fall back to "Default"/"Basic": EasyDict names the incomplete
// setup so the user does not wonder which deck the card went to.
func TestAddToAnkiRefusesAnIncompleteSetup(t *testing.T) {
	store := testStore(t)
	svc := &Service{cfg: store}

	if err := svc.AddToAnki(anki.Note{Word: "hello"}); err == nil || !strings.Contains(err.Error(), "Anki Connect 未启用") {
		t.Fatalf("未启用 = %v, want Anki Connect 未启用", err)
	}
	edit := func(fn func(*settings.Config)) {
		t.Helper()
		if _, err := store.Update(fn); err != nil {
			t.Fatalf("Update: %v", err)
		}
	}
	edit(func(c *settings.Config) { c.AnkiEnabled = true; c.AnkiDeck = "  " })
	if err := svc.AddToAnki(anki.Note{Word: "hello"}); err == nil || !strings.Contains(err.Error(), "Anki 设置不完整") {
		t.Fatalf("空牌组 = %v, want Anki 设置不完整", err)
	}
	edit(func(c *settings.Config) { c.AnkiDeck = "Default"; c.AnkiModel = "" })
	if err := svc.AddToAnki(anki.Note{Word: "hello"}); err == nil || !strings.Contains(err.Error(), "Anki 设置不完整") {
		t.Fatalf("空笔记类型 = %v, want Anki 设置不完整", err)
	}
	edit(func(c *settings.Config) { c.AnkiModel = "Basic"; c.AnkiMappings = nil })
	if err := svc.AddToAnki(anki.Note{Word: "hello"}); err == nil || !strings.Contains(err.Error(), "Anki 设置不完整") {
		t.Fatalf("空字段映射 = %v, want Anki 设置不完整", err)
	}
}
