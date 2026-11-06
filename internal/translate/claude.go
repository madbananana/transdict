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

// Claude uses the Anthropic Messages API. Anthropic differs from the
// OpenAI-shaped adapters in three ways that matter here: the key is a header
// rather than a Bearer token, the system instruction is a top-level field
// instead of a message, and max_tokens is mandatory.
type claude struct {
	BaseURL string
	APIKey  string
	Model   string
	Prompt  string

	httpc *http.Client
}

var (
	claudeDefaultBaseURL  = "https://api.anthropic.com"
	claudeDefaultModel    = "claude-sonnet-4-5"
	claudeAPIVersion      = "2023-06-01"
	claudeDefaultMaxToken = 4096
)

func init() {
	Register(settings.ServiceType{
		Kind: "claude", Name: "Anthropic Claude",
		Note:     "Messages 接口，长文与排版保持能力好",
		Multi:    true,
		Markdown: true,
		Prompt:   true,
		Fields: []settings.Field{
			{Key: settings.FieldAPIKey, Label: "API Key", Secret: true, Required: true},
			{Key: settings.FieldBaseURL, Label: "接口地址", Placeholder: claudeDefaultBaseURL},
			{Key: settings.FieldModel, Label: "模型", Placeholder: claudeDefaultModel},
			{Key: settings.FieldPrompt, Label: "附加指令", Text: true},
		},
		Default: settings.Provider{BaseURL: claudeDefaultBaseURL, Model: claudeDefaultModel},
	}, func(p settings.Provider) (Provider, error) {
		return newClaude(p.BaseURL, p.APIKey, p.Model, p.Prompt), nil
	})
}

func newClaude(baseURL, apiKey, model, prompt string) *claude {
	if baseURL == "" {
		baseURL = claudeDefaultBaseURL
	}
	if model == "" {
		model = claudeDefaultModel
	}
	return &claude{
		BaseURL: strings.TrimRight(baseURL, "/"),
		APIKey:  apiKey,
		Model:   model,
		Prompt:  prompt,
		httpc:   netp.Client(120 * time.Second),
	}
}

func (c *claude) Label() string { return "Claude · " + c.Model }

// WithPrompt returns a copy holding only the ad-hoc instruction, leaving the
// registered provider untouched so the next plain translation is unaffected.
func (c *claude) WithPrompt(prompt string) Provider {
	clone := *c
	clone.Prompt = prompt
	return &clone
}

func (c *claude) systemPrompt(targetLang string) string {
	sb := strings.Builder{}
	sb.WriteString("你是专业文学与实务译者。将用户提供的文本翻译成")
	sb.WriteString(LanguageName(targetLang))
	sb.WriteString("。保持原文的换行结构，一行对一行，不增删行；数字、代码、单位、" +
		"品牌名与标点样式照原样保留。仅输出译文正文，不要前言、注释、引号或 Markdown 代码块。")
	if strings.TrimSpace(c.Prompt) != "" {
		sb.WriteString("\n额外要求：" + c.Prompt)
	}
	return sb.String()
}

func (c *claude) Translate(ctx context.Context, text, sourceLang, targetLang string) (string, string, error) {
	body, err := json.Marshal(map[string]any{
		"model":      c.Model,
		"max_tokens": claudeDefaultMaxToken,
		"system":     c.systemPrompt(targetLang),
		"messages": []map[string]any{
			{"role": "user", "content": text},
		},
		"temperature": 0.2,
	})
	if err != nil {
		return "", "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/v1/messages",
		bytes.NewReader(body))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.APIKey)
	req.Header.Set("anthropic-version", claudeAPIVersion)

	resp, err := c.httpc.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return "", "", err
	}

	// A failure body is checked before anything else: a proxy or gateway may
	// answer HTML, and the status is the reliable part of that signal.
	if resp.StatusCode != http.StatusOK {
		var errOnly struct {
			Error claudeErrBody `json:"error"`
		}
		_ = json.Unmarshal(raw, &errOnly)
		return "", "", fmt.Errorf("Claude HTTP %d: %s", resp.StatusCode, claudeErrText(errOnly.Error, raw))
	}

	var payload struct {
		Type       string          `json:"type"`
		Content    json.RawMessage `json:"content"`
		StopReason string          `json:"stop_reason"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", "", fmt.Errorf("Claude 响应解析失败: %s", truncate(string(raw)))
	}
	if payload.Type == "error" {
		return "", "", fmt.Errorf("Claude 返回错误: %s", truncate(string(raw)))
	}

	out, err := contentText(payload.Content)
	if err != nil {
		return "", "", err
	}
	out = strings.TrimSpace(out)
	if out == "" {
		if payload.StopReason == "refusal" {
			return "", "", fmt.Errorf("Claude 拒绝翻译这段文本")
		}
		return "", "", fmt.Errorf("Claude 未给出译文（stop_reason=%s）", claudeStopName(payload.StopReason))
	}
	switch payload.StopReason {
	case "", "end_turn", "stop_sequence":
		// Normal completion; stop_sequence still leaves usable text above.
	case "max_tokens":
		return out, "", fmt.Errorf("Claude 译文被 max_tokens 截断，请缩短原文或提高输出上限")
	case "refusal":
		return out, "", fmt.Errorf("Claude 拒绝翻译这段文本")
	default:
		return out, "", fmt.Errorf("Claude 译文不完整（stop_reason=%s）", payload.StopReason)
	}
	return out, "", nil
}

// claudeErrBody is the {"error":{"type","message"}} envelope.
type claudeErrBody struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

func claudeErrText(e claudeErrBody, raw []byte) string {
	if e.Message != "" {
		if e.Type != "" {
			return e.Type + ": " + e.Message
		}
		return e.Message
	}
	return truncate(string(raw))
}

func claudeStopName(s string) string {
	if s == "" {
		return "none"
	}
	return s
}
