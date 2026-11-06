package translate

import (
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

// NiuTrans (小牛翻译) authenticates with an api key sent as a form field and
// returns the translation in tgt_text; there is no per-request signature.
var niutransEndpoint = "https://api.niutrans.com/NiuTransServer/translation"

type niutrans struct {
	apiKey  string
	baseURL string
	httpc   *http.Client
}

func init() {
	Register(settings.ServiceType{
		Kind: "niutrans", Name: "小牛翻译",
		Note: "apikey 作为表单字段；返回 tgt_text，支持 400+ 语种",
		Fields: []settings.Field{
			{Key: settings.FieldAPIKey, Label: "API Key", Secret: true, Required: true},
			{Key: settings.FieldBaseURL, Label: "接口地址（可留空）", Placeholder: niutransEndpoint},
		},
	}, func(p settings.Provider) (Provider, error) {
		return &niutrans{
			apiKey:  strings.TrimSpace(p.APIKey),
			baseURL: strings.TrimSpace(p.BaseURL),
			httpc:   netp.Client(20 * time.Second),
		}, nil
	})
}

func (n *niutrans) Label() string { return "小牛翻译" }

func (n *niutrans) endpoint() string {
	if n.baseURL != "" {
		return strings.TrimRight(n.baseURL, "/")
	}
	return niutransEndpoint
}

func niuLang(code string) string {
	switch code {
	case "", "auto":
		return "auto"
	case "zh-CN", "zh":
		return "zh"
	case "zh-TW":
		return "zh"
	default:
		return trBase(code)
	}
}

func (n *niutrans) Translate(ctx context.Context, text, sourceLang, targetLang string) (string, string, error) {
	src := niuLang(sourceLang)
	dst := niuLang(targetLang)
	return trFanout(ctx, text, 1000, func(chunk string) (string, string, error) {
		return n.once(ctx, chunk, src, dst)
	})
}

func (n *niutrans) once(ctx context.Context, q, src, dst string) (string, string, error) {
	form := url.Values{}
	form.Set("from", src)
	form.Set("to", dst)
	form.Set("apikey", n.apiKey)
	form.Set("src_text", q)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.endpoint(),
		strings.NewReader(form.Encode()))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := n.httpc.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return "", "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("小牛翻译 HTTP %d: %s", resp.StatusCode, truncate(string(raw)))
	}

	var payload struct {
		TgtText     string `json:"tgt_text"`
		Translation string `json:"translation"`
		From        string `json:"from"`
		ErrorCode   string `json:"error_code"`
		ErrorMsg    string `json:"error_msg"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", "", fmt.Errorf("小牛翻译响应解析失败: %s", truncate(string(raw)))
	}
	if payload.ErrorCode != "" {
		return "", "", fmt.Errorf("小牛翻译错误 %s: %s", payload.ErrorCode, payload.ErrorMsg)
	}
	out := payload.TgtText
	if out == "" {
		out = payload.Translation
	}
	if out == "" {
		return "", "", fmt.Errorf("小牛翻译返回空译文: %s", truncate(string(raw)))
	}
	return out, payload.From, nil
}
