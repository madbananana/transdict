package gui

import (
	"testing"

	"transdict/internal/dict"
	"transdict/internal/history"
	"transdict/internal/l10n"
	"transdict/internal/query"
	"transdict/internal/settings"
	"transdict/internal/translate"
	"transdict/internal/tts"
)

// The interface language must not leak into stored data: a service row shows the
// English name of its kind while the store keeps the Chinese one it was created
// with, and a row the user renamed is never rewritten.
func TestProviderLabelsFollowTheInterfaceLanguage(t *testing.T) {
	defer l10n.Set("zh-CN")

	cfg := settings.Config{
		Dict: []settings.Provider{
			{ID: "dict-apple", Category: settings.CatDict, Kind: "apple", Label: "苹果词典"},
			{ID: "dict-mine", Category: settings.CatDict, Kind: "youdao", Label: "我的词典"},
		},
		// The built-in config names a service shorter than its registration does,
		// so the table has to cover the stored wording, not only the type name.
		Translate: []settings.Provider{
			{ID: "tr-openai", Category: settings.CatTranslate, Kind: "openai", Label: "AI 模型"},
		},
	}

	l10n.Set("en")
	got := localizedProviders(cfg).List(settings.CatDict)
	if got[0].Label != "Apple Dictionary" {
		t.Errorf("default label = %q, want the English name", got[0].Label)
	}
	if got[1].Label != "我的词典" {
		t.Errorf("renamed label = %q, want it left alone", got[1].Label)
	}
	if tr := localizedProviders(cfg).List(settings.CatTranslate)[0]; tr.Label != "AI model" {
		t.Errorf("translate label = %q, want the English name", tr.Label)
	}
	if cfg.Dict[0].Label != "苹果词典" {
		t.Error("localizedProviders wrote through to the caller's config")
	}

	l10n.Set("zh-CN")
	if got := localizedProviders(cfg).List(settings.CatDict)[0].Label; got != "苹果词典" {
		t.Errorf("chinese label = %q, want the source text", got)
	}
}

// A save arrives with whatever the window was showing, so an English row has to
// land back in the store as the Chinese default — otherwise switching language
// again would leave it looking like a custom name.
func TestNormalizeKeepsStoredLabelsCanonical(t *testing.T) {
	defer l10n.Set("zh-CN")
	l10n.Set("en")

	// Kinds that every build registers, so the expectations hold wherever the
	// test runs rather than only where the Apple adapters exist.
	rows := []settings.Provider{
		{Kind: "youdao", Label: "Youdao Dictionary"},
		{Kind: "youdao", Label: "My Dictionary"},
		{Kind: "youdao", Label: ""},
	}
	want := []string{"有道词典", "My Dictionary", "有道词典"}
	got := normalize(rows, settings.CatDict)
	for i, p := range got {
		if p.Label != want[i] {
			t.Errorf("row %d stored as %q, want %q", i, p.Label, want[i])
		}
	}

	back := normalize([]settings.Provider{{Kind: "openai", Label: "AI model"}}, settings.CatTranslate)
	if back[0].Label != "AI 模型" {
		t.Errorf("saved as %q, want the stored Chinese name back", back[0].Label)
	}
}

// A result is worded for the window that draws it, while the copy that goes into
// history keeps the wording it was answered with.
func TestResultsAndHistoryAreWordedOnTheWayOut(t *testing.T) {
	defer l10n.Set("zh-CN")
	l10n.Set("en")

	res := query.Response{
		Translate: []translate.Result{
			{Provider: "youdao", Label: "有道"},
			{Provider: "google", Label: "Google", Error: "Google 返回空译文"},
		},
		Dict:    []dict.Result{{Provider: "youdao", Label: "有道词典", Error: "有道词典：查询内容为空"}},
		Error:   "剪贴板为空",
		Skipped: []string{"百度通用文字识别"},
	}
	out := localizedResponse(res)
	if out.Translate[0].Label != "Youdao" || out.Dict[0].Label != "Youdao Dictionary" {
		t.Errorf("labels = %q, %q, want the English names", out.Translate[0].Label, out.Dict[0].Label)
	}
	// The line under a failed service is written by that service in Chinese, so an
	// English panel that skipped it would mix the two languages in one card.
	if out.Translate[1].Error != "Google returned an empty translation" {
		t.Errorf("service error = %q, want the English sentence", out.Translate[1].Error)
	}
	if out.Dict[0].Error != "Youdao Dictionary: nothing to look up" {
		t.Errorf("dictionary error = %q, want the English sentence", out.Dict[0].Error)
	}
	if res.Translate[1].Error != "Google 返回空译文" {
		t.Error("localizedResponse wrote through to the caller's translations")
	}
	if out.Skipped[0] != "Baidu General OCR" || res.Skipped[0] != "百度通用文字识别" {
		t.Errorf("Skipped = %q, stored %q: want the English name on the way out only",
			out.Skipped[0], res.Skipped[0])
	}
	if out.Translate[1].Label != "Google" {
		t.Errorf("latin label = %q, want it unchanged", out.Translate[1].Label)
	}
	if out.Error != "The clipboard is empty" {
		t.Errorf("error = %q, want the English sentence", out.Error)
	}
	if res.Translate[0].Label != "有道" {
		t.Error("localizedResponse wrote through to the caller's response")
	}

	entries := []history.Entry{{ID: "e1", Services: []string{"Google", "有道词典"}}}
	if got := localizedHistory(entries)[0].Services; got[1] != "Youdao Dictionary" {
		t.Errorf("history service = %q, want the English name", got[1])
	}
	if entries[0].Services[1] != "有道词典" {
		t.Error("localizedHistory wrote through to the stored entry")
	}
}

// A read-aloud failure is the one thing the panel can say about speech, so the
// provider's sentence has to arrive already worded for the interface language.
func TestSpeechIsWordedOnTheWayOut(t *testing.T) {
	defer l10n.Set("zh-CN")

	r := tts.Result{Provider: "youdao", Label: "有道朗读", Error: "有道朗读：待朗读文本为空"}

	l10n.Set("en")
	out := localizedSpeech(r)
	if out.Label != "Youdao speech" {
		t.Errorf("label = %q, want the English name", out.Label)
	}
	if out.Error != "Youdao speech: there is nothing to read aloud" {
		t.Errorf("error = %q, want the English sentence", out.Error)
	}
	if r.Error != "有道朗读：待朗读文本为空" {
		t.Error("localizedSpeech wrote through to the caller's result")
	}

	l10n.Set("zh-CN")
	if got := localizedSpeech(r); got.Error != r.Error {
		t.Errorf("chinese error = %q, want the source text", got.Error)
	}
}
