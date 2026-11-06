//go:build darwin

package dict

import (
	"context"
	"testing"
)

// stubProvider answers with one fixed entry, which is enough to check that the
// stage restates Chinese answers for a 繁体 target.
type stubProvider struct{ entry Entry }

func (s stubProvider) Label() string { return "stub" }
func (s stubProvider) Explain(context.Context, string, string, string) ([]Entry, error) {
	return []Entry{s.entry}, nil
}

func TestRunQueryConvertsDictionaryAnswersToTraditional(t *testing.T) {
	p := stubProvider{entry: Entry{
		Word: "苹果",
		Definitions: []Definition{{
			Pos:      "n.",
			Meanings: []string{"苹果；苹果树"},
			Examples: []string{"这个苹果很甜。"},
		}},
		Web:      []WebDefinition{{Value: "苹果公司", Conts: []string{"科技公司"}}},
		Examples: []Example{{Source: "An apple a day.", Translation: "一天一个苹果。"}},
	}}

	res := RunQuery(context.Background(), "stub", "Stub", p, "apple", "en", "zh-TW")
	if got := res.Entries[0].Word; got != "蘋果" {
		t.Errorf("词头 = %q, want 苹果 in 繁体", got)
	}
	if got := res.Entries[0].Definitions[0].Meanings[0]; got != "蘋果；蘋果樹" {
		t.Errorf("释义 = %q, want 繁体", got)
	}
	if got := res.Entries[0].Examples[0].Translation; got != "一天一個蘋果。" {
		t.Errorf("例句译文 = %q, want 繁体", got)
	}
	if got := res.Entries[0].Web[0].Value; got != "蘋果公司" {
		t.Errorf("网络释义 = %q, want 繁体", got)
	}

	// A 简体 target keeps the service's own wording, which is what it answered.
	res = RunQuery(context.Background(), "stub", "Stub", p, "apple", "en", "zh-CN")
	if got := res.Entries[0].Word; got != "苹果" {
		t.Errorf("简体目标下词头 = %q, want 苹果", got)
	}
}
