package translate

import (
	"fmt"
	"strings"

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
	models      []string
	keyRequired bool
	// hostRequired is set when there is no shipped default host, so an empty
	// baseUrl cannot silently fall back to api.openai.com.
	hostRequired bool
}

var openAIPresets = []openAIPreset{
	{kind: "deepseek", name: "DeepSeek", note: "OpenAI 兼容接口，deepseek-chat / deepseek-reasoner",
		baseURL: "https://api.deepseek.com/v1", model: "deepseek-chat",
		models: []string{"deepseek-chat", "deepseek-reasoner"}, keyRequired: true},
	{kind: "moonshot", name: "Moonshot Kimi", note: "OpenAI 兼容接口，国内可达",
		baseURL: "https://api.moonshot.cn/v1", model: "moonshot-v1-8k",
		models: []string{"moonshot-v1-8k", "moonshot-v1-32k", "moonshot-v1-128k"}, keyRequired: true},
	{kind: "zhipu", name: "智谱 GLM", note: "OpenAI 兼容端点（paas/v4），非官方 Python SDK 协议",
		baseURL: "https://open.bigmodel.cn/api/paas/v4", model: "glm-4-flash",
		models: []string{"glm-4-flash", "glm-4-plus"}, keyRequired: true},
	{kind: "groq", name: "Groq", note: "OpenAI 兼容接口，Llama 系列速度快",
		baseURL: "https://api.groq.com/openai/v1", model: "llama-3.3-70b-versatile",
		models: []string{"llama-3.3-70b-versatile", "llama-3.1-8b-instant"}, keyRequired: true},
	{kind: "minimax", name: "MiniMax", note: "OpenAI 兼容接口，文本模型 MiniMax-Text",
		baseURL: "https://api.minimax.chat/v1", model: "MiniMax-Text-01",
		models: []string{"MiniMax-Text-01"}, keyRequired: true},
	{kind: "siliconflow", name: "硅基流动", note: "OpenAI 兼容接口，聚合开源模型",
		baseURL: "https://api.siliconflow.cn/v1", model: "deepseek-ai/DeepSeek-V3",
		models: []string{"deepseek-ai/DeepSeek-V3", "Qwen/Qwen2.5-72B-Instruct"}, keyRequired: true},
	{kind: "xai", name: "xAI Grok", note: "OpenAI 兼容接口，Grok 系列",
		baseURL: "https://api.x.ai/v1", model: "grok-3",
		models: []string{"grok-3", "grok-3-mini"}, keyRequired: true},
	{kind: "mistral", name: "Mistral", note: "OpenAI 兼容接口，Mistral 系列",
		baseURL: "https://api.mistral.ai/v1", model: "mistral-large-latest",
		models: []string{"mistral-large-latest", "mistral-small-latest"}, keyRequired: true},
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
		Kind:        s.kind,
		Name:        s.name,
		Note:        s.note,
		Multi:       true,
		Markdown:    true,
		Prompt:      true,
		Models:      s.models,
		FetchModels: true,
		Fields: []settings.Field{
			{Key: settings.FieldBaseURL, Label: "接口地址", Placeholder: s.baseURL, Advanced: true},
			{Key: settings.FieldAPIKey, Label: "API Key", Secret: true, Required: s.keyRequired},
			{Key: settings.FieldModel, Label: "模型", Placeholder: s.model},
			{Key: settings.FieldSupportedModels, Label: "支持的模型", Placeholder: strings.Join(s.models, ", "), Advanced: true},
			{Key: settings.FieldPrompt, Label: "附加指令", Text: true, Advanced: true},
			reasoningEffortField(),
			hideThinkField(),
			customPromptField(),
			systemPromptField(),
			userPromptField(),
			usageStatusField(),
			taskField(settings.FieldTranslation, "翻译"),
			taskField(settings.FieldSentence, "句子分析"),
			taskField(settings.FieldDictionary, "查单词"),
			streamField(),
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
	if p.BaseURL == "" {
		p.BaseURL = s.baseURL
	}
	if p.Model == "" {
		p.Model = s.model
	}
	return newOpenAI(p)
}
