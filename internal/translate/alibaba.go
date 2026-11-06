package translate

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"transdict/internal/netp"
	"transdict/internal/settings"
)

// Alibaba 机器翻译 (alimt) uses the RPC style: every parameter is percent-encoded,
// sorted, joined into a canonical query string, and the whole thing is signed
// with HMAC-SHA1 keyed by AccessKeySecret + "&". The nonce and timestamp feed
// the signature, so both are injectable package vars for deterministic tests.
var alibabaEndpoint = "https://mt.cn-hangzhou.aliyuncs.com"

var (
	aliNonce     = func() string { return strconv.FormatInt(time.Now().UnixNano(), 10) }
	aliTimestamp = func() string { return time.Now().UTC().Format("2006-01-02T15:04:05Z") }
)

const alibabaRegionDefault = "cn-hangzhou"

type alibaba struct {
	accessKeyID string
	secret      string
	region      string
	bucket      string
	baseURL     string
	httpc       *http.Client
}

func init() {
	Register(settings.ServiceType{
		Kind: "alibaba", Name: "阿里翻译",
		Note:  "RPC 签名 HMAC-SHA1；AccessKeyId 填应用ID、AccessKeySecret 填密钥；region/bucket 在扩展项",
		Multi: true,
		Fields: []settings.Field{
			{Key: settings.FieldAppID, Label: "AccessKeyId", Required: true},
			{Key: settings.FieldSecret, Label: "AccessKeySecret", Secret: true, Required: true},
			{Key: "region", Label: "地域 (region)", Placeholder: alibabaRegionDefault},
			{Key: "bucket", Label: "Bucket（可选）", Placeholder: ""},
			{Key: settings.FieldBaseURL, Label: "接口地址（可留空）", Placeholder: alibabaEndpoint},
		},
		Default: settings.Provider{Extra: map[string]string{"region": alibabaRegionDefault}},
	}, func(p settings.Provider) (Provider, error) {
		region := strings.TrimSpace(p.Get("region"))
		if region == "" {
			region = alibabaRegionDefault
		}
		return &alibaba{
			accessKeyID: strings.TrimSpace(p.AppID),
			secret:      strings.TrimSpace(p.Secret),
			region:      region,
			bucket:      strings.TrimSpace(p.Get("bucket")),
			baseURL:     strings.TrimSpace(p.BaseURL),
			httpc:       netp.Client(20 * time.Second),
		}, nil
	})
}

func (a *alibaba) Label() string { return "阿里翻译" }

func (a *alibaba) endpoint() string {
	if a.baseURL != "" {
		return strings.TrimRight(a.baseURL, "/")
	}
	if a.region != alibabaRegionDefault {
		return fmt.Sprintf("https://mt.%s.aliyuncs.com", a.region)
	}
	return alibabaEndpoint
}

func aliLang(code string) string {
	switch code {
	case "", "auto":
		return "auto"
	case "zh-CN", "zh":
		return "zh"
	case "zh-TW":
		return "zh-tw"
	default:
		return trBase(code)
	}
}

// aliyunPercentEncode follows Alibaba's RFC-3986 variant: query-escape then map
// '+' -> %20, '*' -> %2A, and '~' back to itself.
func aliyunPercentEncode(s string) string {
	e := url.QueryEscape(s)
	e = strings.ReplaceAll(e, "+", "%20")
	e = strings.ReplaceAll(e, "*", "%2A")
	e = strings.ReplaceAll(e, "%7E", "~")
	return e
}

// alibabaCanonical builds the sorted, percent-encoded query string (without the
// Signature parameter) used both to sign and to verify.
func alibabaCanonical(params map[string]string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	pairs := make([]string, 0, len(keys))
	for _, k := range keys {
		pairs = append(pairs, aliyunPercentEncode(k)+"="+aliyunPercentEncode(params[k]))
	}
	return strings.Join(pairs, "&")
}

// alibabaSignature returns the base64 HMAC-SHA1 signature for the RPC request.
func alibabaSignature(secret string, params map[string]string) string {
	canonical := alibabaCanonical(params)
	stringToSign := "POST&" + aliyunPercentEncode("/") + "&" + aliyunPercentEncode(canonical)
	mac := hmac.New(sha1.New, []byte(secret+"&"))
	mac.Write([]byte(stringToSign))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func (a *alibaba) Translate(ctx context.Context, text, sourceLang, targetLang string) (string, string, error) {
	src := aliLang(sourceLang)
	dst := aliLang(targetLang)
	return trFanout(ctx, text, 1000, func(chunk string) (string, string, error) {
		return a.once(ctx, chunk, src, dst)
	})
}

func (a *alibaba) once(ctx context.Context, q, src, dst string) (string, string, error) {
	params := map[string]string{
		"Action":           "TranslateGeneral",
		"Version":          "2018-10-12",
		"Format":           "JSON",
		"AccessKeyId":      a.accessKeyID,
		"SignatureMethod":  "HMAC-SHA1",
		"SignatureVersion": "1.0",
		"SignatureNonce":   aliNonce(),
		"Timestamp":        aliTimestamp(),
		"RegionId":         a.region,
		"SourceLanguage":   src,
		"TargetLanguage":   dst,
		"SourceText":       q,
	}
	params["Signature"] = alibabaSignature(a.secret, withoutSignature(params))

	form := url.Values{}
	for k, v := range params {
		form.Set(k, v)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.endpoint(),
		strings.NewReader(form.Encode()))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := a.httpc.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return "", "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("阿里翻译 HTTP %d: %s", resp.StatusCode, truncate(string(raw)))
	}

	var payload struct {
		Data struct {
			Translated string `json:"Translated"`
			Lang       string `json:"Lang"`
		} `json:"Data"`
		Code    string `json:"Code"`
		Message string `json:"Message"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", "", fmt.Errorf("阿里翻译响应解析失败: %s", truncate(string(raw)))
	}
	if payload.Data.Translated == "" {
		if payload.Code != "" {
			return "", "", fmt.Errorf("阿里翻译错误 %s: %s", payload.Code, payload.Message)
		}
		return "", "", fmt.Errorf("阿里翻译返回空译文: %s", truncate(string(raw)))
	}
	return payload.Data.Translated, payload.Data.Lang, nil
}

func withoutSignature(params map[string]string) map[string]string {
	out := make(map[string]string, len(params))
	for k, v := range params {
		if k != "Signature" {
			out[k] = v
		}
	}
	return out
}
