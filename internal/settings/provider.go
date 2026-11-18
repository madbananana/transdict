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

	// FieldCookie is EasyDict's Cookie Key（可选）: the browser cookie a network
	// that blocks scripted requests needs before the translator answers.
	FieldCookie = "cookie"

	// FieldSupportedModels is EasyDict's 支持的模型: the list a card's model
	// menu switches between. Only services that name a model have it.
	FieldSupportedModels = "supportedModels"

	// FieldReasoningEffort is the OpenAI-compatible 思考程度 picker. EasyDict
	// offers 关闭/高/最大 and forwards the choice as reasoning_effort.
	FieldReasoningEffort = "reasoningEffort"

	// FieldHideThink mirrors EasyDict's 隐藏 <think> 标签内容: reasoning models
	// that return their scratchpad inline should not show it in the card.
	FieldHideThink = "hideThink"

	// FieldEffort is the Claude Code CLI's 思考程度 picker. EasyDict offers a
	// CLI-default plus 低/中/高/极高/最大, forwarded as claude --effort.
	FieldEffort = "effort"

	// The AI prompt configuration mirrors EasyDict's OpenAI page: a custom
	// system/user prompt pair that replaces the built-in instructions, and one
	// switch per query type the service may answer.
	FieldEnableCustomPrompt = "enableCustomPrompt"
	FieldSystemPrompt       = "systemPrompt"
	FieldUserPrompt         = "userPrompt"

	// FieldUsageStatus is EasyDict's 使用状态: follow the window selection,
	// always answer, or never answer on its own.
	FieldUsageStatus = "usageStatus"

	// FieldStream is EasyDict's 启用流式传输: an endpoint that buffers or rejects
	// streamed requests can be switched to the one-shot path without losing the
	// service. Empty means on.
	FieldStream = "stream"

	// FieldTranslation, FieldSentence and FieldDictionary are the per-task
	// switches; a service answers 翻译/句子分析/查单词 only while its switch is on.
	FieldTranslation = "translation"
	FieldSentence    = "sentence"
	FieldDictionary  = "dictionary"
)

// Usage status values. Empty is EasyDict's 默认: the window's checked list
// decides, as it does for every service that does not set one.
const (
	UsageDefault   = ""
	UsageAlwaysOn  = "alwaysOn"
	UsageAlwaysOff = "alwaysOff"
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
	// Options turns the box into a picker. Empty means free text.
	Options []FieldOption `json:"options,omitempty"`
	// Note is the small line under the box, EasyDict's footnote: it explains a
	// switch or lists the variables a prompt accepts.
	Note string `json:"note,omitempty"`
	// Advanced keeps a field out of the form's first view: the settings page
	// shows what a service needs to work and folds the rest (endpoints, model
	// lists, extra instructions) under 高级设置.
	Advanced bool `json:"advanced,omitempty"`
}

// FieldOption is one row of a Field picker. Value is what the provider stores,
// Label what the user reads.
type FieldOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
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

	// Models is the model list a service offers out of the box. A configured
	// 支持的模型 replaces it; the card's model menu switches between the rows.
	Models []string `json:"models,omitempty"`

	// FetchModels marks a service whose model list can be read from the
	// endpoint, which is what puts EasyDict's 从云端获取模型 button on its page.
	FetchModels bool `json:"fetchModels,omitempty"`

	// FetchLocalModels is the same button worded for a daemon on this machine:
	// EasyDict's 获取本地模型 reads what Ollama has already pulled.
	FetchLocalModels bool `json:"fetchLocalModels,omitempty"`

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
