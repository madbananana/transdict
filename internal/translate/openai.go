package translate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"transdict/internal/netp"
	"transdict/internal/settings"
)

// OpenAI translates through any OpenAI-compatible chat endpoint. Layout
// preservation matters for screenshot translation, where lines must stay
// one-to-one with the OCR output above them.
type openAI struct {
	BaseURL string
	APIKey  string
	Model   string
	Prompt  string

	httpc *http.Client
}

func init() {
	Register(settings.ServiceType{
		Kind: "openai", Name: "AI 模型（OpenAI 兼容）", Note: "DeepSeek、通义、Kimi、硅基流动、Ollama 等均可",
		Multi: true, Markdown: true, Prompt: true,
		Fields: []settings.Field{
			{Key: settings.FieldBaseURL, Label: "接口地址", Placeholder: "https://api.openai.com/v1"},
			{Key: settings.FieldAPIKey, Label: "API Key", Secret: true, Required: true},
			{Key: settings.FieldModel, Label: "模型", Placeholder: "gpt-4o-mini"},
			{Key: settings.FieldPrompt, Label: "附加指令", Text: true},
		},
		Default: settings.Provider{BaseURL: "https://api.openai.com/v1", Model: "gpt-4o-mini"},
	}, func(p settings.Provider) (Provider, error) {
		return newOpenAI(p.BaseURL, p.APIKey, p.Model, p.Prompt), nil
	})
}

func newOpenAI(baseURL, apiKey, model, prompt string) *openAI {
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	if model == "" {
		model = "gpt-4o-mini"
	}
	return &openAI{
		BaseURL: strings.TrimRight(baseURL, "/"),
		APIKey:  apiKey,
		Model:   model,
		Prompt:  prompt,
		httpc:   netp.Client(120 * time.Second),
	}
}

func (o *openAI) Label() string { return "AI 模型 · " + o.Model }

// WithPrompt replaces the instruction for one call. The copy keeps the
// credentials but not the user's saved 附加指令, since an ad-hoc prompt such as
// 润色 would otherwise be diluted by it.
func (o *openAI) WithPrompt(prompt string) Provider {
	clone := *o
	clone.Prompt = prompt
	return &clone
}

func (o *openAI) systemPrompt(targetLang string) string {
	sb := strings.Builder{}
	sb.WriteString("你是专业译者。把用户提供的文本翻译成")
	sb.WriteString(LanguageName(targetLang))
	sb.WriteString("，逐行对应原文的换行，保留数字、代码、专有名词与标点格式。" +
		"只输出译文，不要解释、不要加引号或代码块。")
	if strings.TrimSpace(o.Prompt) != "" {
		sb.WriteString("\n补充要求：" + o.Prompt)
	}
	return sb.String()
}

func (o *openAI) Translate(ctx context.Context, text, sourceLang, targetLang string) (string, string, error) {
	body, _ := json.Marshal(map[string]any{
		"model": o.Model,
		"messages": []any{
			map[string]string{"role": "system", "content": o.systemPrompt(targetLang)},
			map[string]string{"role": "user", "content": text},
		},
		"temperature": 0.2,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.BaseURL+"/chat/completions",
		bytes.NewReader(body))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+o.APIKey)

	resp, err := o.httpc.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return "", "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("AI 翻译 HTTP %d: %s", resp.StatusCode, truncate(string(raw)))
	}

	var payload struct {
		Choices []struct {
			Message struct {
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", "", fmt.Errorf("AI 翻译响应解析失败: %s", truncate(string(raw)))
	}
	if len(payload.Choices) == 0 {
		if payload.Error.Message != "" {
			return "", "", fmt.Errorf("AI 翻译: %s", payload.Error.Message)
		}
		return "", "", fmt.Errorf("AI 翻译返回空结果: %s", truncate(string(raw)))
	}

	out, err := contentText(payload.Choices[0].Message.Content)
	if err != nil {
		return "", "", err
	}
	return strings.TrimSpace(out), "", nil
}

func contentText(raw json.RawMessage) (string, error) {
	if len(raw) == 0 {
		return "", nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s, nil
	}
	var parts []struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &parts); err != nil {
		return "", fmt.Errorf("AI 翻译 content 格式未知: %s", truncate(string(raw)))
	}
	var sb strings.Builder
	for _, p := range parts {
		sb.WriteString(p.Text)
	}
	return sb.String(), nil
}
