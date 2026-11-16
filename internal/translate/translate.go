package translate

import (
	"context"
	"strings"
	"time"

	"transdict/internal/netp"
	"transdict/internal/settings"
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

	// Streaming marks a row that is readable but has not finished writing yet,
	// so 停止生成 is offered on that card alone. Only progress updates set it.
	Streaming bool `json:"streaming,omitempty"`

	// Warning marks an answer that is empty rather than broken — the service
	// replied, there was just nothing to translate. EasyDict's noResult warning:
	// the card folds to a quiet row, keeps its ⊘ mark and hides 重试, which
	// cannot fix a question the service had no answer for.
	Warning bool `json:"warning,omitempty"`

	// Stream says the service writes its answer in pieces. EasyDict only treats
	// an empty answer as noResult for services that cannot stream; a streamer
	// that stayed silent is a fault, so its card keeps the red mark and 重试.
	// Never sent to the frontend: the row's own Streaming flag is what draws.
	Stream bool `json:"-"`

	// Markdown says the answer came from a service whose reply is prose — with
	// headings and code fences in it — so the panel renders it instead of quoting
	// it. Site is the same query on the service's own page, for when a card looks
	// wrong and the user wants to see the source. Both come from the registration,
	// never from the reply, so a service cannot change how it is presented.
	Markdown bool   `json:"markdown,omitempty"`
	Site     string `json:"site,omitempty"`

	// Model names the model an AI service answered with, and Models is what its
	// card menu offers — EasyDict's 支持的模型 list, or the kind's shipped
	// defaults. Plain translators leave both empty and draw no chip.
	Model  string   `json:"model,omitempty"`
	Models []string `json:"models,omitempty"`

	// PromptTitle and PromptURL are EasyDict's promptURL: a page where the user
	// can fix what the error is about. Only a refused key or exhausted quota
	// carries one, because that is the failure a settings page cannot explain.
	PromptTitle string `json:"promptTitle,omitempty"`
	PromptURL   string `json:"promptUrl,omitempty"`
}

// ModelInfo is the model chip a service's card shows. A kind without a 模型
// field answers nothing: its card would offer a one-item menu of itself.
func ModelInfo(p settings.Provider) (string, []string) {
	meta, ok := Meta(p.Kind)
	if !ok {
		return "", nil
	}
	hasModel := false
	for _, f := range meta.Fields {
		if f.Key == settings.FieldModel {
			hasModel = true
			break
		}
	}
	if !hasModel {
		return "", nil
	}
	current := strings.TrimSpace(p.Model)
	if current == "" {
		current = meta.Default.Model
	}
	choices := modelChoices(p.Get(settings.FieldSupportedModels), meta.Models, current)
	return current, choices
}

// modelChoices is 支持的模型 when set, else the kind's shipped list, with the
// saved model always present so the chip's name and the menu's check mark agree.
func modelChoices(configured string, shipped []string, current string) []string {
	var raw []string
	if s := strings.TrimSpace(configured); s != "" {
		raw = strings.Split(strings.ReplaceAll(s, "，", ","), ",")
	} else {
		raw = shipped
	}
	seen := map[string]bool{}
	var out []string
	for _, m := range raw {
		m = strings.TrimSpace(m)
		if m == "" || seen[m] {
			continue
		}
		seen[m] = true
		out = append(out, m)
	}
	if current != "" && !seen[current] {
		out = append([]string{current}, out...)
	}
	return out
}

type Provider interface {
	Label() string
	Translate(ctx context.Context, text, sourceLang, targetLang string) (string, string, error)
}

// Streamer is implemented by services whose answer arrives in pieces, which is
// what lets a long AI translation fill its card while the rest of the query is
// still running. onText receives the whole answer so far, not the new delta, so
// a caller can replace the visible text without accumulating anything itself.
type Streamer interface {
	TranslateStream(ctx context.Context, text, sourceLang, targetLang string,
		onText func(partial string)) (string, string, error)
}

// StreamToggler is implemented by services whose 启用流式传输 switch can turn the
// streaming path off even though the adapter knows how to stream. When it says
// no, the pipeline takes the one-shot path and the result is not a stream: an
// endpoint that ignores `stream` would otherwise be reported as a failed stream
// when it answers with nothing.
type StreamToggler interface {
	StreamingEnabled() bool
}

// ModelLister is implemented by AI services that can read their model catalog
// from the endpoint — EasyDict's 从云端获取模型. The settings page offers the
// fetch sheet only for kinds that declare it.
type ModelLister interface {
	ListModels(ctx context.Context) ([]string, error)
}

// Promptable is implemented by services that accept an ad-hoc instruction,
// which is how 润色 and 总结 reuse an AI provider without a second pipeline.
// The returned provider must carry the prompt for one call only.
type Promptable interface {
	WithPrompt(prompt string) Provider
}

// FirstLanguage is implemented by prompt-driven services whose custom prompt
// may contain ${{firstLanguage}}. The engine supplies 第一语言 on every run; the
// returned provider must carry it for one call only.
type FirstLanguage interface {
	WithFirstLanguage(lang string) Provider
}

// PairChecker is implemented by services that only cover some language pairs.
// A pair it refuses is not a failure to show as an error card — the service has
// nothing to say about 中文→日语 — so the engine leaves it out of that query.
// The source may still be "auto": a service that supports every source into the
// target can answer, and one that never translates into the target cannot.
type PairChecker interface {
	SupportsPair(sourceLang, targetLang string) bool
}

// QueryTyper is implemented by services whose 翻译/句子分析/查单词 switches bound
// what they answer. A query no enabled task covers leaves the service out
// rather than drawing an error for a task the user turned off.
type QueryTyper interface {
	SupportsQuery(text, sourceLang, targetLang string) bool
}

func RunQuery(ctx context.Context, name, label string, p Provider, text, src, dst string) Result {
	return runQuery(ctx, name, label, p, text, src, dst, nil)
}

// RunQueryStream is RunQuery with a progress callback. A provider that cannot
// stream simply answers once and the callback never fires, so the caller does
// not have to know which services support it.
func RunQueryStream(ctx context.Context, name, label string, p Provider, text, src, dst string,
	onText func(string)) Result {
	return runQuery(ctx, name, label, p, text, src, dst, onText)
}

func runQuery(ctx context.Context, name, label string, p Provider, text, src, dst string,
	onText func(string)) Result {
	start := time.Now()
	var out, detected string
	var err error
	s, isStream := p.(Streamer)
	if t, ok := p.(StreamToggler); ok {
		isStream = isStream && t.StreamingEnabled()
	}
	if isStream && onText != nil {
		out, detected, err = s.TranslateStream(ctx, text, src, dst, onText)
	} else {
		out, detected, err = p.Translate(ctx, text, src, dst)
	}
	r := Result{Provider: name, Label: label, Text: out, Detected: detected,
		ElapsedMs: time.Since(start).Milliseconds(), Stream: isStream}
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
