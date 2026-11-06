//go:build darwin

package dict

import (
	"strings"
	"testing"

	"transdict/internal/settings"
)

// These tests talk to the macOS Dictionary Services engine, which is offline and
// needs no network. A machine without any system dictionary installed answers
// nothing, and that case skips rather than fails.

func TestAppleRegisteredAsDict(t *testing.T) {
	meta, ok := Meta("apple")
	if !ok {
		t.Fatal("apple 未注册")
	}
	if !meta.Free || meta.Multi || meta.Category != settings.CatDict {
		t.Errorf("meta = %+v, 期望免费、非多实例、词典分类", meta)
	}
	if !strings.Contains(meta.Note, "仅单词") {
		t.Errorf("note = %q, 期望说明仅支持单词", meta.Note)
	}
	if len(meta.Fields) != 0 {
		t.Errorf("fields = %+v, 系统词典不应要求任何配置", meta.Fields)
	}
	if _, err := New(settings.Provider{ID: "dict-apple", Category: settings.CatDict,
		Kind: "apple", Enabled: true}); err != nil {
		t.Fatalf("New(apple) 失败: %v", err)
	}
}

func newAppleProvider(t *testing.T) (Provider, error) {
	t.Helper()
	return New(settings.Provider{ID: "dict-apple", Category: settings.CatDict,
		Kind: "apple", Enabled: true})
}

func TestAppleRejectsBadQueries(t *testing.T) {
	p, err := newAppleProvider(t)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Explain(t.Context(), "   ", "en", "zh-CN"); err == nil ||
		!strings.Contains(errText(err), "空") {
		t.Errorf("空查询应报错, 实际: %v", err)
	}
	long := strings.Repeat("词", appleMaxQueryRunes+1)
	if _, err := p.Explain(t.Context(), long, "en", "zh-CN"); err == nil ||
		!strings.Contains(errText(err), "仅支持单词") {
		t.Errorf("超长查询应直接拒绝, 实际: %v", err)
	}
}

// A pinyin coincidence made the Chinese dictionary answer an English query:
// "run" came back as 瞤 (rún). The answer must be recognised as the wrong word
// rather than shown as if it belonged to the query.
func TestAppleRejectsAnAnswerInTheWrongScript(t *testing.T) {
	if !wrongScript("run", "瞤 rún （瞤）〈文〉①动 眼皮跳动。 ②动 肌肉抽搐。") {
		t.Error("英文查询收到中文词条时应判定为不匹配")
	}
	if wrongScript("hello", "hello | BrE həˈləʊ, hɛˈləʊ | A. noun 问候") {
		t.Error("正常的英文词条不应被拒绝")
	}
	if wrongScript("世界", "世界 shìjiè ①名 佛教指无限的时间和空间。") {
		t.Error("中文查询不应套用英文规则")
	}
	if wrongScript("привет", "привет | приветствие") {
		t.Error("其他文字系统的查询不应被拒绝")
	}
}

func TestAppleExplainsWord(t *testing.T) {
	p, err := newAppleProvider(t)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := p.Explain(t.Context(), "  Apple  ", "en", "zh-CN")
	if err != nil {
		if strings.Contains(errText(err), "未收录") {
			t.Skipf("本机系统词典不可用: %v", err)
		}
		t.Fatalf("Explain 失败: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("条目数 = %d, 期望 1", len(entries))
	}
	entry := entries[0]
	if entry.Word != "Apple" {
		t.Errorf("Word = %q, 期望查询被修剪后回填", entry.Word)
	}
	if strings.TrimSpace(entry.Text) == "" {
		t.Error("Text 为空")
	}
	// The DCS interop must survive punctuation that is not ASCII, otherwise the
	// CFString conversion is broken rather than the dictionary being empty.
	if _, err := p.Explain(t.Context(), "茶", "zh-CN", "en"); err != nil &&
		!strings.Contains(errText(err), "未收录") {
		t.Errorf("中文查询报错异常: %v", err)
	}
}
