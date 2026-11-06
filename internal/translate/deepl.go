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

// DeepL v2. Free-plan keys end in ":fx" and must go to the free host.
type deepl struct {
	APIKey string
	httpc  *http.Client
}

func init() {
	Register(settings.ServiceType{
		Kind: "deepl", Name: "DeepL", Note: "欧洲语言质量最好；:fx 结尾的密钥走免费版主机",
		Fields: []settings.Field{
			{Key: settings.FieldAPIKey, Label: "API Key", Secret: true, Required: true},
		},
	}, func(p settings.Provider) (Provider, error) {
		return &deepl{APIKey: p.APIKey, httpc: netp.Client(30 * time.Second)}, nil
	})
}

func (d *deepl) Label() string { return "DeepL" }

func (d *deepl) endpoint() string {
	if strings.HasSuffix(d.APIKey, ":fx") {
		return "https://api-free.deepl.com/v2/translate"
	}
	return "https://api.deepl.com/v2/translate"
}

func (d *deepl) Translate(ctx context.Context, text, sourceLang, targetLang string) (string, string, error) {
	form := url.Values{}
	form.Set("auth_key", d.APIKey)
	form.Set("target_lang", DeepLCode(targetLang))
	if sourceLang != "" && sourceLang != "auto" {
		form.Set("source_lang", DeepLCode(sourceLang))
	}
	form.Add("text", text)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.endpoint(),
		strings.NewReader(form.Encode()))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := d.httpc.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return "", "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("DeepL HTTP %d: %s", resp.StatusCode, truncate(string(raw)))
	}

	var payload struct {
		Translations []struct {
			Text                   string `json:"text"`
			DetectedSourceLanguage string `json:"detected_source_language"`
		} `json:"translations"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", "", fmt.Errorf("DeepL 响应解析失败: %s", truncate(string(raw)))
	}
	if len(payload.Translations) == 0 {
		return "", "", fmt.Errorf("DeepL: %s", payload.Message)
	}

	var sb strings.Builder
	for _, t := range payload.Translations {
		sb.WriteString(t.Text)
	}
	detected := payload.Translations[0].DetectedSourceLanguage
	return strings.TrimSpace(sb.String()), strings.ToLower(detected), nil
}
