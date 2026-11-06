package translate

import (
	"fmt"

	"transdict/internal/settings"
)

// openAIPreset is one vendor that speaks the OpenAI chat/completions protocol
// well enough to reuse the adapter verbatim: only the default host and model
// differ. Each still gets its own kind so the settings screen, the readiness
// check and a saved provider entry stay vendor-specific.
type openAIPreset struct {
	kind        string
	name        string
	note        string
	baseURL     string
	model       string
	keyRequired bool
	// hostRequired is set when there is no shipped default host, so an empty
	// baseUrl cannot silently fall back to api.openai.com.
	hostRequired bool
}

var openAIPresets = []openAIPreset{
	{kind: "deepseek", name: "DeepSeek", note: "OpenAI 兼容接口，deepseek-chat / deepseek-reasoner",
		baseURL: "https://api.deepseek.com/v1", model: "deepseek-chat", keyRequired: true},
	{kind: "moonshot", name: "Moonshot Kimi", note: "OpenAI 兼容接口，国内可达",
		baseURL: "https://api.moonshot.cn/v1", model: "moonshot-v1-8k", keyRequired: true},
	{kind: "zhipu", name: "智谱 GLM", note: "OpenAI 兼容端点（paas/v4），非官方 Python SDK 协议",
		baseURL: "https://open.bigmodel.cn/api/paas/v4", model: "glm-4-flash", keyRequired: true},
	{kind: "groq", name: "Groq", note: "OpenAI 兼容接口，Llama 系列速度快",
		baseURL: "https://api.groq.com/openai/v1", model: "llama-3.3-70b-versatile", keyRequired: true},
	{kind: "minimax", name: "MiniMax", note: "OpenAI 兼容接口，文本模型 MiniMax-Text",
		baseURL: "https://api.minimax.chat/v1", model: "MiniMax-Text-01", keyRequired: true},
	{kind: "siliconflow", name: "硅基流动", note: "OpenAI 兼容接口，聚合开源模型",
		baseURL: "https://api.siliconflow.cn/v1", model: "deepseek-ai/DeepSeek-V3", keyRequired: true},
	{kind: "xai", name: "xAI Grok", note: "OpenAI 兼容接口，Grok 系列",
		baseURL: "https://api.x.ai/v1", model: "grok-3", keyRequired: true},
	{kind: "mistral", name: "Mistral", note: "OpenAI 兼容接口，Mistral 系列",
		baseURL: "https://api.mistral.ai/v1", model: "mistral-large-latest", keyRequired: true},
	{kind: "custom", name: "自定义 OpenAI 兼容服务",
		note:         "任何实现 chat/completions 的服务：vLLM、Ollama /v1、One-API、企业网关",
		hostRequired: true, keyRequired: false},
}

func init() {
	for _, preset := range openAIPresets {
		Register(preset.serviceType(), func(p settings.Provider) (Provider, error) {
			if preset.hostRequired && p.BaseURL == "" {
				return nil, fmt.Errorf("自定义 AI 服务需要填写接口地址（形如 https://host/v1）")
			}
			return preset.build(p), nil
		})
	}
}

// serviceType describes the preset to the settings UI. The placeholders carry
// the defaults so a blank field visibly means "use these".
func (s openAIPreset) serviceType() settings.ServiceType {
	t := settings.ServiceType{
		Kind:     s.kind,
		Name:     s.name,
		Note:     s.note,
		Multi:    true,
		Markdown: true,
		Prompt:   true,
		Fields: []settings.Field{
			{Key: settings.FieldBaseURL, Label: "接口地址", Placeholder: s.baseURL},
			{Key: settings.FieldAPIKey, Label: "API Key", Secret: true, Required: s.keyRequired},
			{Key: settings.FieldModel, Label: "模型", Placeholder: s.model},
			{Key: settings.FieldPrompt, Label: "附加指令", Text: true},
		},
		Default: settings.Provider{BaseURL: s.baseURL, Model: s.model},
	}
	if s.hostRequired {
		// A blank host is an error rather than a silent api.openai.com default,
		// so the shipped entry carries no address at all.
		t.Default = settings.Provider{Model: s.model}
		t.Fields[0].Required = true
	}
	return t
}

// build applies the vendor defaults only where the user left a field empty,
// so an explicit 接口地址 (a proxy, a regional endpoint) always wins.
func (s openAIPreset) build(p settings.Provider) *openAI {
	baseURL, model := s.baseURL, s.model
	if p.BaseURL != "" {
		baseURL = p.BaseURL
	}
	if p.Model != "" {
		model = p.Model
	}
	return newOpenAI(baseURL, p.APIKey, model, p.Prompt)
}
