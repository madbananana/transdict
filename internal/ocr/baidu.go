package ocr

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"transdict/internal/netp"
	"transdict/internal/settings"
)

// baidu implements 通用文字识别（高精度版）.
// APIKey carries the API Key and Secret carries the Secret Key.
type baidu struct {
	APIKey string
	Secret string

	mu       sync.Mutex
	token    string
	tokenExp time.Time
	httpc    *http.Client
}

const baiduEndpoint = "https://aip.baidubce.com/rest/2.0/ocr/v1/accurate_basic"

func init() {
	Register(settings.ServiceType{
		Kind: "baidu", Name: "百度通用文字识别", Note: "开放平台「通用文字识别（高精度版）」应用",
		Fields: []settings.Field{
			{Key: settings.FieldAPIKey, Label: "API Key", Secret: true, Required: true},
			{Key: settings.FieldSecret, Label: "Secret Key", Secret: true, Required: true},
		},
	}, func(p settings.Provider) (Provider, error) {
		return &baidu{APIKey: p.APIKey, Secret: p.Secret, httpc: netp.Client(30 * time.Second)}, nil
	})
}

func (b *baidu) Label() string { return "百度 OCR" }

func (b *baidu) accessToken(ctx context.Context) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.token != "" && time.Until(b.tokenExp) > time.Minute {
		return b.token, nil
	}

	q := url.Values{}
	q.Set("grant_type", "client_credentials")
	q.Set("client_id", b.APIKey)
	q.Set("client_secret", b.Secret)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://aip.baidubce.com/oauth/2.0/token", strings.NewReader(q.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := b.httpc.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var payload struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
		Error       string `json:"error_description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	if payload.AccessToken == "" {
		return "", fmt.Errorf("百度鉴权失败: %s", payload.Error)
	}
	b.token = payload.AccessToken
	b.tokenExp = time.Now().Add(time.Duration(payload.ExpiresIn) * time.Second)
	return b.token, nil
}

func (b *baidu) Recognize(ctx context.Context, png []byte) ([]Block, error) {
	token, err := b.accessToken(ctx)
	if err != nil {
		return nil, err
	}

	form := url.Values{}
	form.Set("image", base64.StdEncoding.EncodeToString(png))
	form.Set("language_type", "CHN_ENG")

	endpoint := baiduEndpoint + "?access_token=" + url.QueryEscape(token)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint,
		strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := b.httpc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("百度 OCR HTTP %d: %s", resp.StatusCode, snippet(body))
	}

	var payload struct {
		WordsResult []struct {
			Words string `json:"words"`
		} `json:"words_result"`
		ErrorCode string `json:"error_code"`
		ErrorMsg  string `json:"error_msg"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("百度 OCR 响应解析失败: %s", snippet(body))
	}
	if payload.ErrorCode != "" {
		return nil, fmt.Errorf("百度 OCR %s: %s", payload.ErrorCode, payload.ErrorMsg)
	}

	blocks := make([]Block, 0, len(payload.WordsResult))
	for _, w := range payload.WordsResult {
		if t := strings.TrimSpace(w.Words); t != "" {
			blocks = append(blocks, Block{Text: t})
		}
	}
	return blocks, nil
}

func snippet(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 200 {
		return s[:200]
	}
	return s
}
