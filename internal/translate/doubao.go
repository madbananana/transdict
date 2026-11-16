package translate

import (
	"bufio"
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

// Doubao is EasyDict's 豆包翻译: a translation-specific model on 火山方舟,
// not a chat endpoint. The request names the language pair instead of putting
// an instruction in a message, and the answer streams back as
// response.output_text.delta events.
const (
	doubaoDefaultBaseURL = "https://ark.cn-beijing.volces.com/api/v3/responses"
	doubaoDefaultModel   = "doubao-seed-translation-250915"
)

type doubao struct {
	BaseURL string
	APIKey  string
	Model   string
	httpc   *http.Client
}

func init() {
	Register(settings.ServiceType{
		Kind: "doubao", Name: "豆包翻译",
		Note:     "火山方舟的专用翻译模型，需 API Key；支持流式输出",
		Multi:    false,
		Markdown: false,
		Models:   []string{doubaoDefaultModel},
		Fields: []settings.Field{
			{Key: settings.FieldBaseURL, Label: "接口地址", Placeholder: doubaoDefaultBaseURL},
			{Key: settings.FieldAPIKey, Label: "API Key", Secret: true, Required: true},
			{Key: settings.FieldModel, Label: "模型", Placeholder: doubaoDefaultModel},
			{Key: settings.FieldSupportedModels, Label: "支持的模型", Placeholder: doubaoDefaultModel},
			usageStatusField(),
		},
		Default: settings.Provider{BaseURL: doubaoDefaultBaseURL, Model: doubaoDefaultModel},
	}, func(p settings.Provider) (Provider, error) {
		return newDoubao(p), nil
	})
}

func newDoubao(p settings.Provider) *doubao {
	baseURL := strings.TrimSpace(p.BaseURL)
	if baseURL == "" {
		baseURL = doubaoDefaultBaseURL
	}
	model := strings.TrimSpace(p.Model)
	if model == "" {
		model = doubaoDefaultModel
	}
	return &doubao{
		BaseURL: baseURL,
		APIKey:  strings.TrimSpace(p.APIKey),
		Model:   model,
		httpc:   netp.Client(120 * time.Second),
	}
}

func (d *doubao) Label() string { return "豆包翻译" }

// doubaoLangs is the model's own vocabulary: 简体 is zh, 繁体 is zh-Hant, and
// 挪威语 is Bokmål. A pair it does not name is left out rather than failing.
var doubaoLangs = map[string]string{
	"auto": "auto", "zh-CN": "zh", "zh-TW": "zh-Hant",
	"en": "en", "ja": "ja", "ko": "ko", "de": "de", "fr": "fr", "es": "es",
	"it": "it", "pt": "pt", "ru": "ru", "th": "th", "vi": "vi", "ar": "ar",
	"cs": "cs", "da": "da", "fi": "fi", "hr": "hr", "hu": "hu", "id": "id",
	"ms": "ms", "no": "nb", "nl": "nl", "pl": "pl", "ro": "ro", "sv": "sv",
	"tr": "tr", "uk": "uk",
}

func (d *doubao) SupportsPair(sourceLang, targetLang string) bool {
	_, sourceOK := doubaoLangs[sourceLang]
	_, targetOK := doubaoLangs[targetLang]
	return sourceOK && targetOK && targetLang != "auto"
}

// Translate collects the stream, because this endpoint has no non-streaming
// reply; the adapter is a Streamer either way.
func (d *doubao) Translate(ctx context.Context, text, sourceLang, targetLang string) (string, string, error) {
	return d.TranslateStream(ctx, text, sourceLang, targetLang, nil)
}

func (d *doubao) TranslateStream(ctx context.Context, text, sourceLang, targetLang string,
	onText func(string)) (string, string, error) {
	source, target := doubaoLangs[sourceLang], doubaoLangs[targetLang]
	if source == "" || target == "" {
		return "", "", fmt.Errorf("豆包翻译不支持该语言对：%s → %s", sourceLang, targetLang)
	}

	payload := map[string]any{
		"model":  d.Model,
		"stream": true,
		"input": []any{map[string]any{
			"role": "user",
			"content": []any{map[string]any{
				"type": "input_text",
				"text": text,
				"translation_options": map[string]string{
					"source_language": source,
					"target_language": target,
				},
			}},
		}},
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.BaseURL, bytes.NewReader(body))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+d.APIKey)
	resp, err := d.httpc.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
		return "", "", fmt.Errorf("豆包翻译 HTTP %d: %s", resp.StatusCode, truncate(string(raw)))
	}
	if !strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		return "", "", fmt.Errorf("豆包翻译返回了非流式响应：%s", truncate(string(raw)))
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64<<10), 1<<20)
	var answer strings.Builder
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}
		var event struct {
			Delta string `json:"delta"`
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}
		if event.Error.Message != "" {
			return "", "", fmt.Errorf("豆包翻译：%s", event.Error.Message)
		}
		if event.Delta == "" {
			continue
		}
		answer.WriteString(event.Delta)
		if onText != nil {
			onText(answer.String())
		}
	}
	if err := scanner.Err(); err != nil {
		return "", "", err
	}
	out := strings.TrimSpace(answer.String())
	if out == "" {
		return "", "", fmt.Errorf("豆包翻译返回空结果")
	}
	if onText != nil {
		onText(out)
	}
	return out, "", nil
}
