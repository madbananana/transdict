package translate

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
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

// Tencent Cloud TMT uses API v3 with the TC3-HMAC-SHA256 signature. The
// timestamp drives both the credential scope and the signing-key derivation, so
// it is an injectable package var to keep signatures reproducible under test.
var tencentEndpoint = "https://tmt.tencentcloudapi.com"

var tencentTimestamp = func() int64 { return time.Now().Unix() }

type tencent struct {
	secretID  string
	secretKey string
	region    string
	baseURL   string
	httpc     *http.Client
}

func init() {
	Register(settings.ServiceType{
		Kind: "tencent", Name: "腾讯云翻译",
		Note: "API v3 TC3-HMAC-SHA256 签名；SecretId 填应用ID、SecretKey 填密钥；可自定义接口地址",
		Fields: []settings.Field{
			{Key: settings.FieldAppID, Label: "SecretId", Required: true},
			{Key: settings.FieldSecret, Label: "SecretKey", Secret: true, Required: true},
			{Key: "region", Label: "地域 (region)", Placeholder: "ap-guangzhou"},
			{Key: settings.FieldBaseURL, Label: "接口地址（可留空）", Placeholder: tencentEndpoint},
		},
		Default: settings.Provider{Extra: map[string]string{"region": "ap-guangzhou"}},
	}, func(p settings.Provider) (Provider, error) {
		region := strings.TrimSpace(p.Get("region"))
		if region == "" {
			region = "ap-guangzhou"
		}
		return &tencent{
			secretID:  strings.TrimSpace(p.AppID),
			secretKey: strings.TrimSpace(p.Secret),
			region:    region,
			baseURL:   strings.TrimSpace(p.BaseURL),
			httpc:     netp.Client(20 * time.Second),
		}, nil
	})
}

func (t *tencent) Label() string { return "腾讯云翻译" }

func (t *tencent) endpoint() string {
	if t.baseURL != "" {
		return strings.TrimRight(t.baseURL, "/")
	}
	return tencentEndpoint
}

func tencentLang(code string) string {
	switch code {
	case "", "auto":
		return "auto"
	case "zh-CN", "zh":
		return "zh"
	case "zh-TW":
		return "zh-TW"
	default:
		return trBase(code)
	}
}

var tencentDetected = map[string]string{
	"chinese": "zh-CN", "english": "en", "japanese": "ja", "korean": "ko",
	"french": "fr", "german": "de", "russian": "ru", "spanish": "es",
	"italian": "it", "portuguese": "pt", "thai": "th", "vietnamese": "vi",
	"arabic": "ar",
}

func tencentHMAC(key []byte, data string) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(data))
	return h.Sum(nil)
}

func tencentSHA256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func tencentHost(ep string) string {
	if u, err := url.Parse(ep); err == nil && u.Host != "" {
		return u.Host
	}
	return "tmt.tencentcloudapi.com"
}

// tencentAuthorization builds the full Authorization header value for a JSON
// POST to ep at unix time ts. It is a free function so the test can call the
// identical code path to assert determinism.
func tencentAuthorization(secretID, secretKey, service, host string, ts int64, body []byte) string {
	date := time.Unix(ts, 0).UTC().Format("2006-01-02")
	hashedPayload := tencentSHA256Hex(body)
	canonicalHeaders := "content-type:application/json; charset=utf-8\nhost:" + host + "\n"
	signedHeaders := "content-type;host"
	canonicalRequest := strings.Join([]string{"POST", "/", "", canonicalHeaders, signedHeaders, hashedPayload}, "\n")
	hashedCanonical := tencentSHA256Hex([]byte(canonicalRequest))

	credentialScope := date + "/" + service + "/tc3_request"
	stringToSign := strings.Join([]string{"TC3-HMAC-SHA256", strconv.FormatInt(ts, 10), credentialScope, hashedCanonical}, "\n")

	secretDate := tencentHMAC([]byte("TC3"+secretKey), date)
	secretService := tencentHMAC(secretDate, service)
	secretSigning := tencentHMAC(secretService, "tc3_request")
	signature := hex.EncodeToString(tencentHMAC(secretSigning, stringToSign))

	return fmt.Sprintf("TC3-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		secretID, credentialScope, signedHeaders, signature)
}

func (t *tencent) Translate(ctx context.Context, text, sourceLang, targetLang string) (string, string, error) {
	src := tencentLang(sourceLang)
	dst := tencentLang(targetLang)
	return trFanout(ctx, text, 3500, func(chunk string) (string, string, error) {
		return t.once(ctx, chunk, src, dst)
	})
}

func (t *tencent) once(ctx context.Context, q, src, dst string) (string, string, error) {
	body, err := json.Marshal(map[string]any{
		"SourceText": q,
		"Source":     src,
		"Target":     dst,
		"ProjectId":  0,
	})
	if err != nil {
		return "", "", err
	}

	ts := tencentTimestamp()
	ep := t.endpoint()
	auth := tencentAuthorization(t.secretID, t.secretKey, "tmt", tencentHost(ep), ts, body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ep, bytes.NewReader(body))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("X-TC-Action", "TextTranslate")
	req.Header.Set("X-TC-Version", "2018-03-21")
	req.Header.Set("X-TC-Timestamp", strconv.FormatInt(ts, 10))
	req.Header.Set("X-TC-Region", t.region)
	req.Header.Set("Authorization", auth)

	resp, err := t.httpc.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return "", "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("腾讯云翻译 HTTP %d: %s", resp.StatusCode, truncate(string(raw)))
	}

	var payload struct {
		Response struct {
			TargetText  string `json:"TargetText"`
			Translation struct {
				Text         string `json:"Text"`
				DetectedLang string `json:"DetectedLang"`
			} `json:"Translation"`
			TranslationList []struct {
				Translation string `json:"Translation"`
				Source      string `json:"Source"`
			} `json:"TranslationList"`
			Error struct {
				Code    string `json:"Code"`
				Message string `json:"Message"`
			} `json:"Error"`
		} `json:"Response"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", "", fmt.Errorf("腾讯云翻译响应解析失败: %s", truncate(string(raw)))
	}
	r := payload.Response
	if r.Error.Code != "" {
		return "", "", fmt.Errorf("腾讯云翻译错误 %s: %s", r.Error.Code, r.Error.Message)
	}

	out := r.TargetText
	detected := ""
	if out == "" && len(r.TranslationList) > 0 {
		out = r.TranslationList[0].Translation
		detected = r.TranslationList[0].Source
	}
	if out == "" {
		out = r.Translation.Text
		detected = r.Translation.DetectedLang
	}
	if out == "" {
		return "", "", fmt.Errorf("腾讯云翻译返回空译文: %s", truncate(string(raw)))
	}
	if c, ok := tencentDetected[strings.ToLower(detected)]; ok {
		detected = c
	} else {
		detected = strings.ToLower(detected)
	}
	return out, detected, nil
}
