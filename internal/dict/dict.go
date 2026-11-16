// Package dict covers the lookup-style services Easydict calls 词典服务:
// anything that explains a word or short phrase rather than rewriting a
// paragraph. Providers here are pure protocol code, so they run identically on
// macOS and Windows unless a kind is explicitly native.
package dict

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"transdict/internal/netp"
	"transdict/internal/settings"
	"transdict/internal/zhconv"
)

// Phonetic is one pronunciation of the headword.
type Phonetic struct {
	Lang   string `json:"lang,omitempty"`
	Symbol string `json:"symbol"`
	Audio  string `json:"audio,omitempty"`
}

// Definition is one part of speech with its meanings and example sentences.
type Definition struct {
	Pos      string   `json:"pos"`
	Meanings []string `json:"meanings"`
	Examples []string `json:"examples,omitempty"`
}

// WebDefinition is a bundled web-dictionary entry, e.g. 网络释义.
type WebDefinition struct {
	Value string   `json:"value"`
	Conts []string `json:"conts"`
}

// Thesaurus is one synonym group: the part of speech plus the words the
// dictionary groups under it and the gloss they share (有道 同义词).
type Thesaurus struct {
	Pos   string   `json:"pos,omitempty"`
	Tran  string   `json:"tran,omitempty"`
	Words []string `json:"words"`
}

// Phrase is a common collocation of the headword ("apple tree 苹果树").
type Phrase struct {
	Headword    string `json:"headword"`
	Translation string `json:"translation,omitempty"`
}

// Example is a bilingual sentence pulled from a corpus.
type Example struct {
	Source      string `json:"source"`
	Translation string `json:"translation,omitempty"`
}

// Entry is one headword. A query can return several when the text holds more
// than one word and the service disambiguates.
type Entry struct {
	Word        string          `json:"word"`
	Phonetics   []Phonetic      `json:"phonetics,omitempty"`
	Definitions []Definition    `json:"definitions,omitempty"`
	Exchange    []string        `json:"exchange,omitempty"`
	Web         []WebDefinition `json:"web,omitempty"`
	// Related holds the site's compound entries ("Hello Kitty") for this word.
	// They are lookup suggestions rather than senses of the headword, so they
	// sit below the definitions instead of replacing 网络释义.
	Related  []WebDefinition `json:"related,omitempty"`
	Exams    []string        `json:"exams,omitempty"`
	Examples []Example       `json:"examples,omitempty"`
	// The word-card sections 有道 returns beside the senses.
	Synonyms  []Thesaurus `json:"synonyms,omitempty"`
	Phrases   []Phrase    `json:"phrases,omitempty"`
	Etymology string      `json:"etymology,omitempty"`
	Text      string      `json:"text,omitempty"`
}

// Result carries either an explanation or the provider's error text. Errors
// travel as data because the UI shows every provider side by side.
type Result struct {
	Provider  string  `json:"provider"`
	Label     string  `json:"label"`
	Entries   []Entry `json:"entries"`
	ElapsedMs int64   `json:"elapsedMs"`
	Error     string  `json:"error,omitempty"`

	// Warning marks a lookup the service answered but had no entry for.
	// EasyDict's noResult: the card folds to a quiet row with ⊘ and no 重试,
	// because asking the same dictionary again cannot find the word.
	Warning bool `json:"warning,omitempty"`

	// Unsupported marks a query the dictionary never answers, such as a whole
	// sentence. EasyDict's unsupportedQueryType draws the same ⊘ but keeps the
	// card open and 重试 available, since another text might be a word.
	Unsupported bool `json:"unsupported,omitempty"`

	// Site is the headword on the dictionary's own page, when the service
	// declares one — the way out when a card looks truncated.
	Site string `json:"site,omitempty"`

	// OpenURL is the app scheme a local dictionary answers on, which is where
	// the link button goes for the Apple dictionary (dict://词). Site stays the
	// web page; a command-line app has no page to name.
	OpenURL string `json:"openUrl,omitempty"`
}

// NotFoundError is a dictionary saying "not in me", as opposed to failing to
// answer at all. The card draws the difference: a miss is a warning row, a
// network fault is a red error that 重试 can fix.
type NotFoundError struct{ msg string }

func (e *NotFoundError) Error() string { return e.msg }

// NotFound builds a miss with the service's own wording.
func NotFound(msg string) error { return &NotFoundError{msg: msg} }

// UnsupportedQueryError is a dictionary asked for something it never looks up,
// such as a paragraph. It is not a miss and not a fault: the service is fine,
// the query type is wrong.
type UnsupportedQueryError struct{ msg string }

func (e *UnsupportedQueryError) Error() string { return e.msg }

// UnsupportedQuery builds that refusal with the dictionary's own wording.
func UnsupportedQuery(msg string) error { return &UnsupportedQueryError{msg: msg} }

// Plain renders an entry as text for copying and for the mini window, which
// shows one line rather than a card.
func (e Entry) Plain() string {
	if strings.TrimSpace(e.Text) != "" {
		return e.Text
	}
	var sb strings.Builder
	if head := e.head(); head != "" {
		sb.WriteString(head)
	}
	for _, w := range e.Web {
		sb.WriteString("\n" + w.Value)
		for _, c := range w.Conts {
			sb.WriteString("\n  " + c)
		}
	}
	for _, w := range e.Related {
		sb.WriteString("\n" + w.Value)
		for _, c := range w.Conts {
			sb.WriteString("\n  " + c)
		}
	}
	for _, x := range e.Exchange {
		sb.WriteString("\n" + x)
	}
	return strings.TrimSpace(sb.String())
}

func (e Entry) head() string {
	var sb strings.Builder
	sb.WriteString(e.Word)
	for _, p := range e.Phonetics {
		if p.Symbol != "" {
			sb.WriteString(" " + p.Symbol)
		}
	}
	for _, d := range e.Definitions {
		if d.Pos != "" {
			sb.WriteString("\n" + d.Pos)
		}
		for i, m := range d.Meanings {
			sb.WriteString("\n  " + strconv.Itoa(i+1) + ". " + m)
		}
	}
	return strings.TrimSpace(sb.String())
}

type Provider interface {
	Label() string
	Explain(ctx context.Context, text, sourceLang, targetLang string) ([]Entry, error)
}

func RunQuery(ctx context.Context, name, label string, p Provider, text, src, dst string) Result {
	start := time.Now()
	entries, err := p.Explain(ctx, text, src, dst)
	if dst == "zh-TW" {
		entries = traditional(entries)
	}
	r := Result{Provider: name, Label: label, Entries: entries, ElapsedMs: time.Since(start).Milliseconds()}
	if err != nil {
		r.Error = netp.Reason(err)
		var miss *NotFoundError
		if errors.As(err, &miss) {
			r.Warning = true
		}
		var unsupported *UnsupportedQueryError
		r.Unsupported = errors.As(err, &unsupported)
	}
	return r
}

// traditional restates an answer in 繁体 when that is the target. Dictionary
// services gloss in 简体 regardless of what was asked — 有道's endpoint ignores
// the request — and 简体 glosses beside a 繁體 translation look half-converted.
// Headwords, examples and meanings all pass through; phonetics and audio are
// left alone.
func traditional(entries []Entry) []Entry {
	for i := range entries {
		e := &entries[i]
		e.Word = zhconv.Traditional(e.Word)
		e.Text = zhconv.Traditional(e.Text)
		for j := range e.Definitions {
			d := &e.Definitions[j]
			for k := range d.Meanings {
				d.Meanings[k] = zhconv.Traditional(d.Meanings[k])
			}
			for k := range d.Examples {
				d.Examples[k] = zhconv.Traditional(d.Examples[k])
			}
		}
		for j := range e.Exchange {
			e.Exchange[j] = zhconv.Traditional(e.Exchange[j])
		}
		for j := range e.Web {
			w := &e.Web[j]
			w.Value = zhconv.Traditional(w.Value)
			for k := range w.Conts {
				w.Conts[k] = zhconv.Traditional(w.Conts[k])
			}
		}
		for j := range e.Related {
			w := &e.Related[j]
			w.Value = zhconv.Traditional(w.Value)
			for k := range w.Conts {
				w.Conts[k] = zhconv.Traditional(w.Conts[k])
			}
		}
		for j := range e.Examples {
			e.Examples[j].Source = zhconv.Traditional(e.Examples[j].Source)
			e.Examples[j].Translation = zhconv.Traditional(e.Examples[j].Translation)
		}
	}
	return entries
}

// Factory builds a provider from its persisted configuration.
type Factory func(p settings.Provider) (Provider, error)

type regEntry struct {
	meta settings.ServiceType
	make Factory
}

var registry = map[settings.ProviderKind]regEntry{}

func Register(meta settings.ServiceType, f Factory) {
	meta.Category = settings.CatDict
	registry[meta.Kind] = regEntry{meta: meta, make: f}
}

func Types() []settings.ServiceType {
	out := make([]settings.ServiceType, 0, len(registry))
	for _, e := range registry {
		out = append(out, e.meta)
	}
	return out
}

func Meta(kind settings.ProviderKind) (settings.ServiceType, bool) {
	e, ok := registry[kind]
	if !ok {
		return settings.ServiceType{}, false
	}
	return e.meta, true
}

func New(p settings.Provider) (Provider, error) {
	e, ok := registry[p.Kind]
	if !ok {
		return nil, &UnknownKind{Kind: p.Kind}
	}
	return e.make(p)
}

type UnknownKind struct{ Kind string }

func (e *UnknownKind) Error() string { return "未知的词典服务：" + e.Kind }

func Ready(p settings.Provider) bool {
	m, ok := Meta(p.Kind)
	return ok && p.Enabled && p.IsReady(m)
}
