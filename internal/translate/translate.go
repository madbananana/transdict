package translate

import (
	"context"
	"strings"
	"time"

	"transdict/internal/netp"
)

type Result struct {
	Provider  string `json:"provider"`
	Label     string `json:"label"`
	Text      string `json:"text"`
	Detected  string `json:"detected,omitempty"`
	ElapsedMs int64  `json:"elapsedMs"`
	Error     string `json:"error,omitempty"`

	// Pending reserves a service's place in a partial answer while it is still
	// working: the panel can draw the row at once without moving it when the
	// answer lands. Only progress updates set it.
	Pending bool `json:"pending,omitempty"`

	// Markdown says the answer came from a service whose reply is prose — with
	// headings and code fences in it — so the panel renders it instead of quoting
	// it. Site is the same query on the service's own page, for when a card looks
	// wrong and the user wants to see the source. Both come from the registration,
	// never from the reply, so a service cannot change how it is presented.
	Markdown bool   `json:"markdown,omitempty"`
	Site     string `json:"site,omitempty"`
}

type Provider interface {
	Label() string
	Translate(ctx context.Context, text, sourceLang, targetLang string) (string, string, error)
}

// Promptable is implemented by services that accept an ad-hoc instruction,
// which is how 润色 and 总结 reuse an AI provider without a second pipeline.
// The returned provider must carry the prompt for one call only.
type Promptable interface {
	WithPrompt(prompt string) Provider
}

// PairChecker is implemented by services that only cover some language pairs.
// A pair it refuses is not a failure to show as an error card — the service has
// nothing to say about 中文→日语 — so the engine leaves it out of that query.
// The source may still be "auto": a service that supports every source into the
// target can answer, and one that never translates into the target cannot.
type PairChecker interface {
	SupportsPair(sourceLang, targetLang string) bool
}

func RunQuery(ctx context.Context, name, label string, p Provider, text, src, dst string) Result {
	start := time.Now()
	out, detected, err := p.Translate(ctx, text, src, dst)
	r := Result{Provider: name, Label: label, Text: out, Detected: detected,
		ElapsedMs: time.Since(start).Milliseconds()}
	if err != nil {
		r.Error = netp.Reason(err)
	}
	return r
}

// Canonical codes used across the app. Every provider maps from these.
var langNames = map[string]string{
	"zh-CN": "简体中文", "zh-TW": "繁体中文", "en": "英语", "ja": "日语", "ko": "韩语",
	"fr": "法语", "de": "德语", "es": "西班牙语", "ru": "俄语", "it": "意大利语",
	"pt": "葡萄牙语", "th": "泰语", "vi": "越南语", "ar": "阿拉伯语",
}

func LanguageName(code string) string {
	if n, ok := langNames[code]; ok {
		return n
	}
	return code
}

// DeepLCode converts a canonical code to DeepL's target_lang vocabulary.
func DeepLCode(code string) string {
	base := code
	if i := strings.IndexAny(base, "-_"); i >= 0 {
		base = base[:i]
	}
	return strings.ToUpper(base)
}
