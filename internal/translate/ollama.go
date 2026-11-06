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

// Ollama runs models locally, so there is no key and no quota: /api/chat
// answers as soon as the model is loaded, which is why the client timeout is
// long — a cold first load of a large model is slow but not an error.
type ollama struct {
	BaseURL string
	Model   string
	Prompt  string

	httpc *http.Client
}

var (
	ollamaDefaultBaseURL = "http://localhost:11434"
	ollamaDefaultModel   = "qwen2.5:7b"
)

func init() {
	Register(settings.ServiceType{
		Kind: "ollama", Name: "Ollama（本地模型）",
		Note: "免密钥本地推理；先执行 ollama pull 拉取所选模型",
		Free: true, Multi: true, Markdown: true, Prompt: true,
		Fields: []settings.Field{
			{Key: settings.FieldBaseURL, Label: "服务地址", Placeholder: ollamaDefaultBaseURL},
			{Key: settings.FieldModel, Label: "模型", Placeholder: ollamaDefaultModel},
			{Key: settings.FieldPrompt, Label: "附加指令", Text: true},
		},
		Default: settings.Provider{BaseURL: ollamaDefaultBaseURL, Model: ollamaDefaultModel},
	}, func(p settings.Provider) (Provider, error) {
		return newOllama(p.BaseURL, p.Model, p.Prompt), nil
	})
}

func newOllama(baseURL, model, prompt string) *ollama {
	if baseURL == "" {
		baseURL = ollamaDefaultBaseURL
	}
	if model == "" {
		model = ollamaDefaultModel
	}
	return &ollama{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Model:   model,
		Prompt:  prompt,
		httpc:   netp.Client(120 * time.Second),
	}
}

func (o *ollama) Label() string { return "Ollama · " + o.Model }

func (o *ollama) WithPrompt(prompt string) Provider {
	clone := *o
	clone.Prompt = prompt
	return &clone
}

func (o *ollama) systemPrompt(targetLang string) string {
	sb := strings.Builder{}
	sb.WriteString("把下面的文本翻译成")
	sb.WriteString(LanguageName(targetLang))
	sb.WriteString("。逐行翻译，行数与原文保持一致，保留其中的数字、代码与专有名词。" +
		"不要解释，不要把结果包进代码块，只输出译文。")
	if strings.TrimSpace(o.Prompt) != "" {
		sb.WriteString("\n附加要求：" + o.Prompt)
	}
	return sb.String()
}

func (o *ollama) Translate(ctx context.Context, text, sourceLang, targetLang string) (string, string, error) {
	body, err := json.Marshal(map[string]any{
		"model":  o.Model,
		"stream": false,
		"options": map[string]any{
			"temperature": 0.2,
		},
		"messages": []map[string]string{
			{"role": "system", "content": o.systemPrompt(targetLang)},
			{"role": "user", "content": text},
		},
	})
	if err != nil {
		return "", "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.BaseURL+"/api/chat",
		bytes.NewReader(body))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := o.httpc.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return "", "", err
	}

	// Ollama reports "model not found" as a 404 with a plain {"error":...},
	// so the status is examined before the success payload is decoded.
	if resp.StatusCode != http.StatusOK {
		var errOnly struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(raw, &errOnly)
		return "", "", fmt.Errorf("Ollama HTTP %d: %s", resp.StatusCode, ollamaErrText(errOnly.Error, raw))
	}

	var payload struct {
		Message struct {
			Content json.RawMessage `json:"content"`
		} `json:"message"`
		Done       bool   `json:"done"`
		DoneReason string `json:"done_reason"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", "", fmt.Errorf("Ollama 响应解析失败: %s", truncate(string(raw)))
	}

	out, err := contentText(payload.Message.Content)
	if err != nil {
		return "", "", err
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return "", "", fmt.Errorf("Ollama 返回空译文，本地模型 %s 可能不支持该任务", o.Model)
	}
	if payload.DoneReason == "length" {
		return out, "", fmt.Errorf("Ollama 译文因达到长度上限被截断（num_predict 太小）")
	}
	return out, "", nil
}

func ollamaErrText(msg string, raw []byte) string {
	if msg != "" {
		if strings.Contains(msg, "not found") {
			return msg + "（请先 ollama pull）"
		}
		return msg
	}
	return truncate(string(raw))
}
