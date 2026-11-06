// Package tts reads text aloud. Providers either hand back a streamable URL or
// the audio bytes; the GUI plays whichever arrives without knowing which.
package tts

import (
	"context"
	"encoding/base64"
	"time"

	"transdict/internal/netp"
	"transdict/internal/settings"
)

// Clip is one utterance. Exactly one of URL or Data is normally set.
type Clip struct {
	MIME string `json:"mime"`
	URL  string `json:"url,omitempty"`
	Data []byte `json:"-"`
}

// DataURL is what a webview needs when the audio came as bytes.
func (c Clip) DataURL() string {
	if len(c.Data) == 0 {
		return ""
	}
	mime := c.MIME
	if mime == "" {
		mime = "audio/mpeg"
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(c.Data)
}

func (c Clip) Source() string {
	if c.URL != "" {
		return c.URL
	}
	return c.DataURL()
}

type Provider interface {
	Label() string
	Speak(ctx context.Context, text, lang string, slow bool) (*Clip, error)
}

type Result struct {
	Provider  string `json:"provider"`
	Label     string `json:"label"`
	Clip      Clip   `json:"clip"`
	ElapsedMs int64  `json:"elapsedMs"`
	Error     string `json:"error,omitempty"`
}

// Playable is what the window receives. A provider that answered with bytes
// carries them in Data, which the JSON bridge does not send; without this
// conversion every voice except the URL-streaming one would reach the webview
// as an empty clip and the button would report "没有可用的朗读服务".
func (r Result) Playable() Result {
	if r.Clip.URL == "" && len(r.Clip.Data) > 0 {
		r.Clip.URL = r.Clip.DataURL()
		r.Clip.Data = nil
	}
	return r
}

func RunQuery(ctx context.Context, name, label string, p Provider, text, lang string, slow bool) Result {
	start := time.Now()
	clip, err := p.Speak(ctx, text, lang, slow)
	r := Result{Provider: name, Label: label, ElapsedMs: time.Since(start).Milliseconds()}
	if err != nil {
		r.Error = netp.Reason(err)
		return r
	}
	if clip != nil {
		r.Clip = *clip
	}
	return r
}

type Factory func(p settings.Provider) (Provider, error)

type regEntry struct {
	meta settings.ServiceType
	make Factory
}

var registry = map[settings.ProviderKind]regEntry{}

func Register(meta settings.ServiceType, f Factory) {
	meta.Category = settings.CatTTS
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

func (e *UnknownKind) Error() string { return "未知的朗读服务：" + e.Kind }

func Ready(p settings.Provider) bool {
	m, ok := Meta(p.Kind)
	return ok && p.Enabled && p.IsReady(m)
}
