package translate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"transdict/internal/netp"
	"transdict/internal/settings"
)

// Gemini talks to Google's generateContent endpoint. The key travels as a
// query parameter because that is what the REST surface documents; the
// x-goog-api-key header is accepted too but older proxies ignore it.
type gemini struct {
	BaseURL string
	APIKey  string
	Model   string
	Prompt  string

	httpc *http.Client
}

// Defaults are vars so a test can repoint the adapter at an httptest server
// without touching a settings.Provider, and so a future Google version bump is
// a one-line change.
var (
	geminiDefaultBaseURL = "https://generativelanguage.googleapis.com"
	geminiDefaultModel   = "gemini-2.0-flash"
	geminiAPIVersion     = "v1beta"
)

func init() {
	Register(settings.ServiceType{
		Kind: "gemini", Name: "Google Gemini",
		Note:     "generateContent 接口；个人密钥有免费额度",
		Multi:    true,
		Markdown: true,
		Prompt:   true,
		Fields: []settings.Field{
			{Key: settings.FieldAPIKey, Label: "API Key", Secret: true, Required: true},
			{Key: settings.FieldBaseURL, Label: "接口地址", Placeholder: geminiDefaultBaseURL},
			{Key: settings.FieldModel, Label: "模型", Placeholder: geminiDefaultModel},
			{Key: settings.FieldPrompt, Label: "附加指令", Text: true},
		},
		Default: settings.Provider{BaseURL: geminiDefaultBaseURL, Model: geminiDefaultModel},
	}, func(p settings.Provider) (Provider, error) {
		return newGemini(p.BaseURL, p.APIKey, p.Model, p.Prompt), nil
	})
}

func newGemini(baseURL, apiKey, model, prompt string) *gemini {
	if baseURL == "" {
		baseURL = geminiDefaultBaseURL
	}
	if model == "" {
		model = geminiDefaultModel
	}
	return &gemini{
		BaseURL: strings.TrimRight(baseURL, "/"),
		APIKey:  apiKey,
		Model:   model,
		Prompt:  prompt,
		httpc:   netp.Client(120 * time.Second),
	}
}

func (g *gemini) Label() string { return "Gemini · " + g.Model }

// WithPrompt carries the ad-hoc 润色/总结 instruction for a single call. The
// saved 附加指令 is dropped on purpose: two instructions in one system turn
// make the model merge them badly.
func (g *gemini) WithPrompt(prompt string) Provider {
	clone := *g
	clone.Prompt = prompt
	return &clone
}

func (g *gemini) systemPrompt(targetLang string) string {
	sb := strings.Builder{}
	sb.WriteString("你是资深译者。把用户给出的文本翻译成")
	sb.WriteString(LanguageName(targetLang))
	sb.WriteString("，输出行数必须与原文一一对应，不要合并或拆行；" +
		"数字、代码、专有名词和标点格式保持原样。只输出译文，不要解释，不要包在代码块里。")
	if strings.TrimSpace(g.Prompt) != "" {
		sb.WriteString("\n补充要求：" + g.Prompt)
	}
	return sb.String()
}

// endpoint puts the model in the path and the key in the query, both escaped:
// model ids contain dots and slashes that must not be reinterpreted.
func (g *gemini) endpoint() string {
	u := fmt.Sprintf("%s/%s/models/%s:generateContent",
		g.BaseURL, geminiAPIVersion, url.PathEscape(g.Model))
	return u + "?key=" + url.QueryEscape(g.APIKey)
}

func (g *gemini) Translate(ctx context.Context, text, sourceLang, targetLang string) (string, string, error) {
	body, err := json.Marshal(map[string]any{
		"systemInstruction": map[string]any{
			"parts": []map[string]string{{"text": g.systemPrompt(targetLang)}},
		},
		"contents": []map[string]any{
			{"role": "user", "parts": []map[string]string{{"text": text}}},
		},
		"generationConfig": map[string]any{
			"temperature":      0.2,
			"maxOutputTokens":  8192,
			"responseMimeType": "text/plain",
			"candidateCount":   1,
		},
	})
	if err != nil {
		return "", "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.endpoint(), bytes.NewReader(body))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.httpc.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return "", "", err
	}

	// Status first: an error page need not be JSON, so the envelope is only
	// decoded best-effort to recover Google's message.
	if resp.StatusCode != http.StatusOK {
		var errOnly struct {
			Error geminiErrBody `json:"error"`
		}
		_ = json.Unmarshal(raw, &errOnly)
		return "", "", fmt.Errorf("Gemini HTTP %d: %s", resp.StatusCode, geminiErrText(errOnly.Error, raw))
	}

	var payload struct {
		Candidates []struct {
			Content struct {
				Parts json.RawMessage `json:"parts"`
			} `json:"content"`
			FinishReason string `json:"finishReason"`
			BlockReason  string `json:"blockReason"`
		} `json:"candidates"`
		PromptFeedback struct {
			BlockReason string `json:"blockReason"`
		} `json:"promptFeedback"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", "", fmt.Errorf("Gemini 响应解析失败: %s", truncate(string(raw)))
	}
	if payload.PromptFeedback.BlockReason != "" {
		return "", "", fmt.Errorf("Gemini 拒绝了这段文本（%s）", payload.PromptFeedback.BlockReason)
	}
	if len(payload.Candidates) == 0 {
		return "", "", fmt.Errorf("Gemini 返回空结果: %s", truncate(string(raw)))
	}

	cand := payload.Candidates[0]
	if cand.BlockReason != "" {
		return "", "", fmt.Errorf("Gemini 结果被安全策略拦截（%s）", cand.BlockReason)
	}
	out, err := contentText(cand.Content.Parts)
	if err != nil {
		return "", "", err
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return "", "", fmt.Errorf("Gemini 未给出译文（finishReason=%s）", geminiFinish(cand.FinishReason))
	}
	// A length cut-off usually means the layout was clipped mid-sentence, which
	// screenshot translation cannot recover from, so surface it.
	if cand.FinishReason == "MAX_TOKENS" {
		return out, "", fmt.Errorf("Gemini 译文因长度上限被截断，请换用更大 maxOutputTokens 的模型或缩短原文")
	}
	if cand.FinishReason != "" && cand.FinishReason != "STOP" {
		return out, "", fmt.Errorf("Gemini 译文不完整（finishReason=%s）", cand.FinishReason)
	}
	return out, "", nil
}

// geminiErrBody is the error envelope Google returns for a rejected call.
type geminiErrBody struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Status  string `json:"status"`
}

func geminiErrText(e geminiErrBody, raw []byte) string {
	if e.Message != "" {
		if e.Status != "" {
			return e.Status + ": " + e.Message
		}
		return e.Message
	}
	return truncate(string(raw))
}

func geminiFinish(r string) string {
	if r == "" {
		return "unknown"
	}
	return r
}
