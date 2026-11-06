package translate

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"transdict/internal/netp"
	"transdict/internal/settings"
)

// Caiyun (彩云小译) through its gateway authenticates with the token carried in
// the X-Ca-Key header and takes the translation direction as a trans_type form
// field (e.g. en2zh, auto2zh). The token is stored in the APIKey field.
var caiyunEndpoint = "https://api.caiyunapp.com/v2.6/translation"

// caiyunRequestID is a form nonce used for replay tracing; it is a package var
// so tests can pin it and assert the exact request shape.
var caiyunRequestID = func() string { return strconv.FormatInt(time.Now().UnixNano(), 10) }

type caiyun struct {
	token   string
	baseURL string
	httpc   *http.Client
}

func init() {
	Register(settings.ServiceType{
		Kind: "caiyun", Name: "彩云小译",
		Note: "令牌放 APIKey（X-Ca-Key 头），表单带 trans_type；主要支持 中/英/日/韩 互译",
		Fields: []settings.Field{
			{Key: settings.FieldAPIKey, Label: "令牌 (X-Ca-Key)", Secret: true, Required: true},
			{Key: settings.FieldBaseURL, Label: "接口地址（可留空）", Placeholder: caiyunEndpoint},
		},
	}, func(p settings.Provider) (Provider, error) {
		return &caiyun{
			token:   strings.TrimSpace(p.APIKey),
			baseURL: strings.TrimSpace(p.BaseURL),
			httpc:   netp.Client(20 * time.Second),
		}, nil
	})
}

func (c *caiyun) Label() string { return "彩云小译" }

func (c *caiyun) endpoint() string {
	if c.baseURL != "" {
		return strings.TrimRight(c.baseURL, "/")
	}
	return caiyunEndpoint
}

func caiyunLang(code string) string {
	switch code {
	case "", "auto":
		return "auto"
	case "zh-CN", "zh":
		return "zh"
	case "zh-TW":
		return "zh-Hant"
	case "en":
		return "en"
	case "ja":
		return "ja"
	case "ko":
		return "ko"
	default:
		return trBase(code)
	}
}

func caiyunTransType(src, dst string) string {
	return caiyunLang(src) + "2" + caiyunLang(dst)
}

func (c *caiyun) Translate(ctx context.Context, text, sourceLang, targetLang string) (string, string, error) {
	transType := caiyunTransType(sourceLang, targetLang)
	return trFanout(ctx, text, 2000, func(chunk string) (string, string, error) {
		return c.once(ctx, chunk, transType)
	})
}

func (c *caiyun) once(ctx context.Context, q, transType string) (string, string, error) {
	form := url.Values{}
	form.Set("trans_type", transType)
	form.Set("source", q)
	form.Set("request_id", caiyunRequestID())

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint(),
		strings.NewReader(form.Encode()))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Ca-Key", c.token)

	resp, err := c.httpc.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return "", "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("彩云小译 HTTP %d: %s", resp.StatusCode, truncate(string(raw)))
	}

	var payload struct {
		Target      []string `json:"target"`
		Translation any      `json:"translation"`
		IsSuccess   *bool    `json:"is_success"`
		Reason      string   `json:"reason"`
		ErrorCode   string   `json:"error_code"`
		ErrorMsg    string   `json:"error_msg"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", "", fmt.Errorf("彩云小译响应解析失败: %s", truncate(string(raw)))
	}
	if payload.ErrorCode != "" {
		return "", "", fmt.Errorf("彩云小译错误 %s: %s", payload.ErrorCode, payload.ErrorMsg)
	}
	if payload.IsSuccess != nil && !*payload.IsSuccess {
		return "", "", fmt.Errorf("彩云小译失败: %s", payload.Reason)
	}

	out := strings.Join(payload.Target, "\n")
	if out == "" {
		out = caiyunFlatString(payload.Translation)
	}
	if out == "" {
		return "", "", fmt.Errorf("彩云小译返回空译文: %s", truncate(string(raw)))
	}
	return out, "", nil
}

// caiyunFlatString coerces a translation field that may be a string or a list
// of strings into a single newline-joined body.
func caiyunFlatString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case []any:
		parts := make([]string, 0, len(t))
		for _, e := range t {
			if s, ok := e.(string); ok {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, "\n")
	}
	return ""
}
