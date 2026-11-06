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

// Volcano Engine (字节/火山) text translation posts a JSON body to
// /api/translate/v1/translation and authenticates with a raw API key in the
// Authorization header (no per-request signature), so only the endpoint needs
// to be a package var for tests.
var volcBase = "https://trans.volcengine.com"

const volcPath = "/api/translate/v1/translation"

type volc struct {
	apiKey  string
	baseURL string
	httpc   *http.Client
}

func init() {
	Register(settings.ServiceType{
		Kind: "volc", Name: "火山引擎翻译",
		Note: "APIKey 直接放入 Authorization 头；单段≤5000字",
		Fields: []settings.Field{
			{Key: settings.FieldAPIKey, Label: "API Key", Secret: true, Required: true},
			{Key: settings.FieldBaseURL, Label: "接口地址（可留空）", Placeholder: volcBase},
		},
	}, func(p settings.Provider) (Provider, error) {
		return &volc{
			apiKey:  strings.TrimSpace(p.APIKey),
			baseURL: strings.TrimSpace(p.BaseURL),
			httpc:   netp.Client(20 * time.Second),
		}, nil
	})
}

func (v *volc) Label() string { return "火山引擎翻译" }

func (v *volc) endpoint() string {
	base := volcBase
	if v.baseURL != "" {
		base = strings.TrimRight(v.baseURL, "/")
	}
	if strings.HasSuffix(base, volcPath) {
		return base
	}
	return base + volcPath
}

// volcLang: an empty source means auto-detect per the documented semantics.
func volcLang(code string) string {
	switch code {
	case "", "auto":
		return ""
	case "zh-CN", "zh":
		return "zh"
	case "zh-TW":
		return "zh-Hant"
	default:
		return trBase(code)
	}
}

func (v *volc) Translate(ctx context.Context, text, sourceLang, targetLang string) (string, string, error) {
	src := volcLang(sourceLang)
	dst := volcLang(targetLang)
	return trFanout(ctx, text, 5000, func(chunk string) (string, string, error) {
		return v.once(ctx, chunk, src, dst)
	})
}

func (v *volc) once(ctx context.Context, q, src, dst string) (string, string, error) {
	body, err := json.Marshal(map[string]any{
		"source_language": src,
		"target_language": dst,
		"text":            q,
	})
	if err != nil {
		return "", "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, v.endpoint(), bytes.NewReader(body))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", v.apiKey)

	resp, err := v.httpc.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return "", "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("火山引擎翻译 HTTP %d: %s", resp.StatusCode, truncate(string(raw)))
	}

	var payload struct {
		Code    json.Number `json:"code"`
		Message string      `json:"message"`
		Data    struct {
			Translation            string `json:"translation"`
			DetectedSourceLanguage string `json:"detected_source_language"`
			TranslationList        []struct {
				Translation            string `json:"translation"`
				DetectedSourceLanguage string `json:"detected_source_language"`
			} `json:"translation_list"`
			TranslatedList       [][]string `json:"translated_list"`
			DetectedLanguageList []string   `json:"detected_language_list"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", "", fmt.Errorf("火山引擎翻译响应解析失败: %s", truncate(string(raw)))
	}

	// Tolerate the several documented response layouts.
	var out, detected string
	switch {
	case payload.Data.Translation != "":
		out, detected = payload.Data.Translation, payload.Data.DetectedSourceLanguage
	case len(payload.Data.TranslationList) > 0:
		parts := make([]string, 0, len(payload.Data.TranslationList))
		for _, t := range payload.Data.TranslationList {
			parts = append(parts, t.Translation)
		}
		out, detected = strings.Join(parts, "\n"), payload.Data.TranslationList[0].DetectedSourceLanguage
	case len(payload.Data.TranslatedList) > 0:
		var flat []string
		for _, group := range payload.Data.TranslatedList {
			flat = append(flat, group...)
		}
		if len(payload.Data.DetectedLanguageList) > 0 {
			detected = payload.Data.DetectedLanguageList[0]
		}
		out = strings.Join(flat, "\n")
	}
	if out == "" {
		code := payload.Code.String()
		if code == "" {
			code = "未知"
		}
		return "", "", fmt.Errorf("火山引擎翻译失败 code=%s message=%s: %s",
			code, payload.Message, truncate(string(raw)))
	}
	return out, detected, nil
}
