//go:build darwin

package settings

import "testing"

func TestAppleServicesLeadTheirStages(t *testing.T) {
	cfg := Default()
	for _, c := range []struct {
		category Category
		list     []Provider
		id       string
	}{
		{CatOCR, cfg.OCR, "ocr-apple"},
		{CatDict, cfg.Dict, "dict-apple"},
		{CatTTS, cfg.TTS, "tts-apple"},
	} {
		if c.list[0].ID != c.id || c.list[0].Kind != "apple" {
			t.Errorf("%s 首位 = %s(%s), want %s(apple)", c.category, c.list[0].ID, c.list[0].Kind, c.id)
		}
		if !c.list[0].Enabled {
			t.Errorf("%s 默认应开启本机服务", c.category)
		}
	}
	for _, context := range []struct{ context, id string }{
		{ContextMain, "dict-apple"},
		{ContextScreenshot, "ocr-apple"},
		{ContextSilentOCR, "ocr-apple"},
		{ContextODict, "dict-apple"},
	} {
		ids := cfg.Selections[context.context]
		if len(ids) == 0 || ids[0] != context.id {
			t.Errorf("%s 勾选 = %v, want %s 在首位", context.context, ids, context.id)
		}
	}
}

// The platform hook writes into lists the defaults already hold, so it has to
// add rather than replace: dropping 有道词典 from 主窗口 would take away the one
// dictionary that needs no configuration.
func TestAppleServicesKeepTheOnlineDefaults(t *testing.T) {
	cfg := Default()
	if !containsID(ids(cfg.Dict), "dict-youdao") {
		t.Errorf("词典 = %v, want 苹果词典与有道词典并存", ids(cfg.Dict))
	}
	if !containsID(cfg.Selections[ContextMain], "dict-youdao") {
		t.Errorf("主窗口勾选 = %v, want 仍包含 dict-youdao", cfg.Selections[ContextMain])
	}
}

func containsID(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
