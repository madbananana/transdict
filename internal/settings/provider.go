package settings

import (
	"net/url"
	"strings"
)

// ProviderKind selects which adapter an entry refers to. Kinds are open-ended:
// a service package adds one by calling Register from its init, so no list in
// this file has to grow in step with the providers.
type ProviderKind = string

// Category is the interface a provider implements, and therefore the pipeline
// stage that runs it.
type Category = string

const (
	CatOCR       Category = "ocr"
	CatTranslate Category = "translate"
	CatDict      Category = "dict"
	CatTTS       Category = "tts"
)

// Field keys the settings UI renders for a service.
const (
	FieldAPIKey  = "apiKey"
	FieldSecret  = "secret"
	FieldAppID   = "appId"
	FieldBaseURL = "baseUrl"
	FieldModel   = "model"
	FieldPrompt  = "prompt"
	FieldExtra   = "extra"
)

// Provider holds everything one service instance needs. Which fields are used
// depends on Kind, so a single shape keeps the settings UI and bindings simple.
type Provider struct {
	ID       string       `json:"id"`
	Category Category     `json:"category"`
	Kind     ProviderKind `json:"kind"`
	Label    string       `json:"label"`
	Enabled  bool         `json:"enabled"`

	APIKey  string `json:"apiKey,omitempty"`
	Secret  string `json:"secret,omitempty"`
	AppID   string `json:"appId,omitempty"`
	BaseURL string `json:"baseUrl,omitempty"`
	Model   string `json:"model,omitempty"`
	Prompt  string `json:"prompt,omitempty"`

	// Extra carries anything a service needs that no other service shares,
	// e.g. a CLI provider's binary path or an MDX dictionary's folder.
	Extra map[string]string `json:"extra,omitempty"`
}

// Field describes one settings input for a service type.
type Field struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Placeholder string `json:"placeholder,omitempty"`
	Secret      bool   `json:"secret"`
	Required    bool   `json:"required"`
	Text        bool   `json:"text,omitempty"`
	// Folder asks for a directory picker next to the box, because typing a path
	// to a dictionary folder is the one thing nobody gets right first time.
	Folder bool `json:"folder,omitempty"`
}

// ServiceType is what the settings UI offers in its "add service" list. It is
// assembled from the registrations of the service packages, so the UI cannot
// drift from the code.
type ServiceType struct {
	Category Category     `json:"category"`
	Kind     ProviderKind `json:"kind"`
	Name     string       `json:"name"`
	Note     string       `json:"note,omitempty"`
	Free     bool         `json:"free"`
	Multi    bool         `json:"multi"`
	Fields   []Field      `json:"fields"`
	Default  Provider     `json:"default"`

	// Markdown says the answer arrives as prose with headings, lists and code
	// fences in it — true of the large-language services, false of the ones that
	// return a sentence. The panel renders instead of quoting only when set.
	Markdown bool `json:"markdown,omitempty"`

	// Prompt says the service accepts an ad-hoc instruction, which is what makes
	// the 润色/总结 shortcuts meaningful. A plain translator has nothing to do
	// with an instruction, so the panel only offers those shortcuts when one of
	// these services is enabled.
	Prompt bool `json:"prompt,omitempty"`

	// Site is where the service shows the same answer in its own page, with
	// {text}, {source} and {target} standing for the query. It is a link the user
	// can open when a card looks wrong, so a service whose page cannot take a
	// query declares nothing.
	Site string `json:"site,omitempty"`
}

// SiteURL expands Site for one query. Only the three placeholders above are
// replaced, each percent-encoded, so a template can be read as a URL without
// knowing which service wrote it. An empty template stays empty: the caller has
// no page to open.
func (t ServiceType) SiteURL(text, source, target string) string {
	if t.Site == "" {
		return ""
	}
	r := strings.NewReplacer(
		"{text}", url.QueryEscape(text),
		"{source}", url.QueryEscape(source),
		"{target}", url.QueryEscape(target),
	)
	return r.Replace(t.Site)
}

// Get/Set let generic code — the settings UI and the readiness check — address
// a field by the name its ServiceType declares, instead of switching on Kind.
func (p Provider) Get(key string) string {
	switch key {
	case FieldAPIKey:
		return p.APIKey
	case FieldSecret:
		return p.Secret
	case FieldAppID:
		return p.AppID
	case FieldBaseURL:
		return p.BaseURL
	case FieldModel:
		return p.Model
	case FieldPrompt:
		return p.Prompt
	}
	return p.Extra[key]
}

func (p *Provider) Set(key, value string) {
	switch key {
	case FieldAPIKey:
		p.APIKey = value
		return
	case FieldSecret:
		p.Secret = value
		return
	case FieldAppID:
		p.AppID = value
		return
	case FieldBaseURL:
		p.BaseURL = value
		return
	case FieldModel:
		p.Model = value
		return
	case FieldPrompt:
		p.Prompt = value
		return
	}
	if p.Extra == nil {
		p.Extra = map[string]string{}
	}
	p.Extra[key] = value
}

// IsReady gates a service out of a query rather than letting it fail, so the
// panel never shows an error for something the user never configured.
func (p Provider) IsReady(t ServiceType) bool {
	for _, f := range t.Fields {
		if f.Required && p.Get(f.Key) == "" {
			return false
		}
	}
	return true
}

// Label is one row of a settings table: a key the app stores and a name the
// user reads. Bound models need a named type, which is why the tables are not
// anonymous structs.
type Label struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}
