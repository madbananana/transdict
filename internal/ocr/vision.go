package ocr

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"transdict/internal/netp"
	"transdict/internal/settings"
)

// Vision uses any OpenAI-compatible chat endpoint with image input.
// It doubles as the fallback for rotated or densely typeset screenshots that
// classical OCR mangles.
type vision struct {
	BaseURL string
	APIKey  string
	Model   string

	httpc *http.Client
}

const visionPrompt = "逐行提取图片中的所有文字，保持原始阅读顺序与换行。" +
	"只输出文字本身，不要翻译、不要解释、不要使用列表符号或代码块。"

func init() {
	Register(settings.ServiceType{
		Kind: "vision", Name: "视觉大模型", Note: "任意 OpenAI 兼容接口，适合旋转/密集排版的截图",
		Multi: true,
		Fields: []settings.Field{
			{Key: settings.FieldBaseURL, Label: "接口地址", Placeholder: "https://api.openai.com/v1"},
			{Key: settings.FieldAPIKey, Label: "API Key", Secret: true, Required: true},
			{Key: settings.FieldModel, Label: "模型", Placeholder: "gpt-4o-mini"},
		},
		Default: settings.Provider{BaseURL: "https://api.openai.com/v1", Model: "gpt-4o-mini"},
	}, func(p settings.Provider) (Provider, error) {
		return newVision(p.BaseURL, p.APIKey, p.Model), nil
	})
}

func newVision(baseURL, apiKey, model string) *vision {
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	if model == "" {
		model = "gpt-4o-mini"
	}
	return &vision{BaseURL: strings.TrimRight(baseURL, "/"), APIKey: apiKey, Model: model,
		httpc: netp.Client(90 * time.Second)}
}

func (v *vision) Label() string { return "视觉模型 · " + v.Model }

func (v *vision) Recognize(ctx context.Context, png []byte) ([]Block, error) {
	body, _ := json.Marshal(map[string]any{
		"model": v.Model,
		"messages": []any{
			map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{"type": "text", "text": visionPrompt},
					map[string]any{
						"type": "image_url",
						"image_url": map[string]string{
							"url": "data:image/png;base64," + base64.StdEncoding.EncodeToString(png),
						},
					},
				},
			},
		},
		"temperature": 0,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, v.BaseURL+"/chat/completions",
		bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+v.APIKey)

	resp, err := v.httpc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("视觉模型 HTTP %d: %s", resp.StatusCode, snippet(raw))
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
		return nil, fmt.Errorf("视觉模型响应解析失败: %s", snippet(raw))
	}
	if len(payload.Choices) == 0 {
		if payload.Error.Message != "" {
			return nil, fmt.Errorf("视觉模型: %s", payload.Error.Message)
		}
		return nil, fmt.Errorf("视觉模型返回空结果: %s", snippet(raw))
	}

	text, err := flattenContent(payload.Choices[0].Message.Content)
	if err != nil {
		return nil, err
	}
	blocks := make([]Block, 0, 16)
	for _, line := range strings.Split(text, "\n") {
		if t := strings.TrimSpace(line); t != "" {
			blocks = append(blocks, Block{Text: t})
		}
	}
	return blocks, nil
}

// flattenContent accepts both the string and the content-part array shape,
// which OpenAI-compatible gateways split between.
func flattenContent(raw json.RawMessage) (string, error) {
	if len(raw) == 0 {
		return "", nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s, nil
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &parts); err != nil {
		return "", fmt.Errorf("视觉模型 content 格式未知: %s", snippet(raw))
	}
	var sb strings.Builder
	for _, p := range parts {
		sb.WriteString(p.Text)
	}
	return sb.String(), nil
}
