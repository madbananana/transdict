package tts

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"transdict/internal/netp"
	"transdict/internal/settings"
)

// Baidu's speech synthesis open API: an OAuth token from the AI cloud, then one
// form-encoded POST per utterance. The older web route this provider used to
// sign itself now rejects every request with "cuid is empty" / "apikey is
// empty", so the only route that still answers is the documented one — and it
// takes the user's own API Key and Secret Key.
var (
	baiduTokenURL = "https://aip.baidubce.com/oauth/2.0/token"
	baiduTTSURL   = "https://tsn.baidu.com/text2audio"

	// baiduCUID identifies this installation to the service. The API wants a
	// stable unique string per client; a hash of the host name is stable and
	// carries nothing personal beyond what the machine already calls itself.
	baiduCUID = func() string {
		host, err := os.Hostname()
		if err != nil || strings.TrimSpace(host) == "" {
			host = "transdict"
		}
		return "transdict-" + md5Hex(host)[:16]
	}()

	// Tokens live for 30 days, and asking for a new one on every 朗读 would add
	// a round trip to a button press. The cache is package-level because each
	// Speak builds a fresh provider from the stored settings.
	baiduTokenMu sync.Mutex
	baiduTokens  = map[string]baiduToken{}
	baiduNow     = time.Now
)

type baiduToken struct {
	token   string
	expires time.Time
}

type baidu struct {
	tokenURL string
	ttsURL   string
	apiKey   string
	secret   string
	httpc    *http.Client
}

func init() {
	Register(settings.ServiceType{
		Kind: "baidu", Name: "百度朗读",
		Note: "语音合成开放接口；需要百度智能云的 API Key 与 Secret Key",
		Fields: []settings.Field{
			{Key: settings.FieldAPIKey, Label: "API Key", Required: true},
			{Key: settings.FieldSecret, Label: "Secret Key", Secret: true, Required: true},
			{Key: settings.FieldBaseURL, Label: "接口地址（可选）", Placeholder: baiduTTSURL},
		},
	}, func(p settings.Provider) (Provider, error) {
		base := strings.TrimRight(p.BaseURL, "/")
		if base == "" {
			base = baiduTTSURL
		}
		return &baidu{
			tokenURL: baiduTokenURL,
			ttsURL:   base,
			apiKey:   strings.TrimSpace(p.APIKey),
			secret:   strings.TrimSpace(p.Secret),
			httpc:    netp.Client(20 * time.Second),
		}, nil
	})
}

func (b *baidu) Label() string { return "百度" }

func (b *baidu) Speak(ctx context.Context, text, lang string, slow bool) (*Clip, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, fmt.Errorf("百度朗读：待朗读文本为空")
	}
	if b.apiKey == "" || b.secret == "" {
		return nil, fmt.Errorf("百度朗读：需要 API Key 与 Secret Key，请在服务设置里填写")
	}
	lan, ok := baiduLang(lang)
	if !ok {
		return nil, fmt.Errorf("百度朗读：不支持 %s 的发音", lang)
	}
	token, err := b.accessToken(ctx)
	if err != nil {
		return nil, err
	}

	spd := "5"
	if slow {
		spd = "3"
	}
	form := url.Values{}
	// tex arrives encoded and the API decodes it once before speaking, so the
	// value has to be escaped inside the form that Encode escapes again.
	form.Set("tex", url.QueryEscape(text))
	form.Set("tok", token)
	form.Set("cuid", baiduCUID)
	form.Set("ctp", "1")
	form.Set("lan", lan)
	form.Set("spd", spd)
	form.Set("pit", "5")
	form.Set("vol", "5")
	form.Set("per", "0") // 度小美; the mixed-mode voice reads both languages.
	form.Set("aue", "3") // 3 = MP3.

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.ttsURL,
		strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", userAgentChrome)

	resp, err := b.httpc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("百度朗读请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, fmt.Errorf("百度朗读读取响应失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("百度朗读 HTTP %d: %s", resp.StatusCode, baiduErrorText(body))
	}
	// The route answers failures with a JSON object even under a 200.
	if ct := resp.Header.Get("Content-Type"); strings.Contains(ct, "json") || looksJSON(body) {
		if msg := baiduJSONError(body); msg != "" {
			return nil, fmt.Errorf("百度朗读错误 %s", msg)
		}
	}
	if len(body) == 0 {
		return nil, fmt.Errorf("百度朗读：接口未返回音频数据")
	}
	return &Clip{MIME: "audio/mp3", Data: body}, nil
}

// accessToken asks for (and then remembers) the OAuth token the synth endpoint
// requires. A failure is reported as the service reported it, because "unknown
// client id" is the answer a mistyped key actually needs.
func (b *baidu) accessToken(ctx context.Context) (string, error) {
	key := b.apiKey + "\x00" + b.tokenURL
	baiduTokenMu.Lock()
	if t, ok := baiduTokens[key]; ok && baiduNow().Before(t.expires) {
		baiduTokenMu.Unlock()
		return t.token, nil
	}
	baiduTokenMu.Unlock()

	q := url.Values{"grant_type": {"client_credentials"}, "client_id": {b.apiKey}, "client_secret": {b.secret}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.tokenURL+"?"+q.Encode(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgentChrome)

	resp, err := b.httpc.Do(req)
	if err != nil {
		return "", fmt.Errorf("百度朗读鉴权失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if err != nil {
		return "", fmt.Errorf("百度朗读鉴权读取失败: %w", err)
	}
	var out struct {
		Token     string `json:"access_token"`
		ExpiresIn int64  `json:"expires_in"`
		Error     string `json:"error"`
		Desc      string `json:"error_description"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("百度朗读鉴权 HTTP %d: %s", resp.StatusCode, errBody(body))
		}
		return "", fmt.Errorf("百度朗读鉴权响应无法解析: %w", err)
	}
	if out.Token == "" {
		msg := out.Desc
		if msg == "" {
			msg = out.Error
		}
		if msg == "" {
			msg = errBody(body)
		}
		return "", fmt.Errorf("百度朗读鉴权失败: %s", msg)
	}

	ttl := time.Duration(out.ExpiresIn)*time.Second - 5*time.Minute
	if ttl < time.Minute {
		ttl = time.Minute
	}
	baiduTokenMu.Lock()
	baiduTokens[key] = baiduToken{token: out.Token, expires: baiduNow().Add(ttl)}
	baiduTokenMu.Unlock()
	return out.Token, nil
}

// baiduJSONError renders the JSON failure the service returns for a rejected
// utterance ("err_no"/"err_msg") and is empty for anything else.
func baiduJSONError(body []byte) string {
	var e struct {
		No  json.Number `json:"err_no"`
		Msg string      `json:"err_msg"`
	}
	if json.Unmarshal(body, &e) != nil || e.Msg == "" {
		return ""
	}
	return strings.TrimSpace(e.No.String() + ": " + e.Msg)
}

func baiduErrorText(body []byte) string {
	if msg := baiduJSONError(body); msg != "" {
		return msg
	}
	return errBody(body)
}

func looksJSON(b []byte) bool {
	t := strings.TrimLeft(string(b), " \n\t\r")
	return strings.HasPrefix(t, "{")
}

// baiduLang maps a canonical code onto the open API's lan values. It speaks
// Chinese and English; anything else is refused rather than read with the wrong
// phonetics, and the next provider gets its turn.
func baiduLang(code string) (string, bool) {
	switch code {
	case "", "auto", "zh", "zh-CN", "zh-TW":
		return "zh", true
	case "en", "en-US", "en-GB":
		return "en", true
	default:
		return "", false
	}
}
