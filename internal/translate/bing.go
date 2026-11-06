package translate

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"transdict/internal/netp"
	"transdict/internal/settings"
)

// Bing uses the Microsoft Edge translator: an unauthenticated GET to the auth
// endpoint mints a short-lived bearer token, then the token is replayed against
// the cognitive-translator API. No user key is required, so the service is free.
// On networks where edge.microsoft.com is unreachable the translator page mints
// an equivalent token for its own host, which is the fallback below.
var (
	bingAuthURL = "https://edge.microsoft.com/translate/auth"
	bingAPIURL  = "https://api-edge.cognitive.microsofttranslator.com/translate"
	bingPageURL = "https://www.bing.com/translator"
)

// bingUA is the browser identity both bing.com endpoints accept; the auth host
// ignores it, but the page rejects requests that look scripted.
const bingUA = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0 Safari/537.36"

// bingCredential is either a bearer token for the cognitive API or a page-minted
// abuse-prevention tuple ("key" is the timestamp, "token" the signed key).
type bingCredential struct {
	bearer    string
	base      string // scheme://host of the page that minted the web token
	ig        string
	timestamp string
	key       string
	expires   time.Time
}

type bing struct {
	baseURL string
	httpc   *http.Client
}

// Tokens live at package level because a provider instance is built per query:
// keeping them on the instance would fetch the page on every query, and that GET
// is the slowest part of the first answer. The key is the pair of endpoints, so
// a test server never reuses a real host's credential.
var (
	bingTokensMu sync.Mutex
	bingTokens   = map[string]*bingCredential{}
)

func init() {
	Register(settings.ServiceType{
		Kind: "bing", Name: "必应翻译",
		Note: "免密钥，复用 Edge 翻译的临时令牌", Free: true,
		Fields: []settings.Field{
			{Key: settings.FieldBaseURL, Label: "接口地址（可留空）", Placeholder: bingAPIURL},
		},
		Default: settings.Provider{Enabled: true},
	}, func(p settings.Provider) (Provider, error) {
		return &bing{
			baseURL: strings.TrimSpace(p.BaseURL),
			httpc:   netp.Client(20 * time.Second),
		}, nil
	})
}

func (b *bing) Label() string { return "必应翻译" }

func (b *bing) apiEndpoint() string {
	if b.baseURL != "" {
		return strings.TrimRight(b.baseURL, "/")
	}
	return bingAPIURL
}

func bingLang(code string) string {
	switch code {
	case "", "auto":
		return ""
	case "zh-CN", "zh":
		return "zh-Hans"
	case "zh-TW":
		return "zh-Hant"
	case "tl":
		return "fil"
	default:
		return trBase(code)
	}
}

// credential returns a cached token, refreshing when missing or expired. The
// lock is held across the fetch so a fan-out burst spends one request, not one
// per chunk.
func (b *bing) credential(ctx context.Context) (*bingCredential, error) {
	key := bingAuthURL + "|" + bingPageURL
	bingTokensMu.Lock()
	defer bingTokensMu.Unlock()
	if cached := bingTokens[key]; cached != nil && time.Now().Before(cached.expires) {
		return cached, nil
	}

	bearer, err := b.edgeToken(ctx)
	if err == nil {
		cred := &bingCredential{bearer: bearer, expires: time.Now().Add(8 * time.Minute)}
		bingTokens[key] = cred
		return cred, nil
	}
	// edge.microsoft.com answers 404 on some networks while bing.com itself is
	// reachable; the page mints a token its own host accepts.
	cred, pageErr := b.pageCredential(ctx)
	if pageErr != nil {
		return nil, fmt.Errorf("%w（网页令牌也失败: %v）", err, pageErr)
	}
	bingTokens[key] = cred
	return cred, nil
}

// edgeToken asks the Edge auth host for a bearer token.
func (b *bing) edgeToken(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, bingAuthURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", bingUA)
	resp, err := b.httpc.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	token := strings.TrimSpace(string(raw))
	if resp.StatusCode != http.StatusOK || token == "" {
		return "", fmt.Errorf("必应翻译获取令牌失败 HTTP %d: %s", resp.StatusCode, truncate(token))
	}
	return token, nil
}

var (
	// IG:"93C5…" and params_AbusePreventionHelper = [ms,"key",ttl] on the page.
	bingPageIG     = regexp.MustCompile(`IG\s*:\s*"([0-9A-Fa-f]+)"`)
	bingPageHelper = regexp.MustCompile(`params_AbusePreventionHelper\s*=\s*\[(\d+),\s*"([^"]+)",\s*(\d+)\]`)
)

// pageCredential reads the translator page and returns the token it embeds.
func (b *bing) pageCredential(ctx context.Context) (*bingCredential, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, bingPageURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", bingUA)
	resp, err := b.httpc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("必应翻译页面 HTTP %d", resp.StatusCode)
	}
	ig := bingPageIG.FindSubmatch(raw)
	helper := bingPageHelper.FindSubmatch(raw)
	if ig == nil || helper == nil {
		return nil, errors.New("必应翻译页面没有提供令牌")
	}
	issued, err := strconv.ParseInt(string(helper[1]), 10, 64)
	if err != nil {
		return nil, err
	}
	ttl, err := strconv.ParseInt(string(helper[3]), 10, 64)
	if err != nil {
		return nil, err
	}
	expires := time.UnixMilli(issued).Add(time.Duration(ttl) * time.Millisecond)
	// Stop using it a little early rather than race the server's own cutoff.
	if expires = expires.Add(-30 * time.Second); !expires.After(time.Now()) {
		expires = time.Now().Add(30 * time.Second)
	}
	at := resp.Request.URL
	return &bingCredential{
		base:      at.Scheme + "://" + at.Host,
		ig:        string(ig[1]),
		timestamp: string(helper[1]),
		key:       string(helper[2]),
		expires:   expires,
	}, nil
}

func (b *bing) Translate(ctx context.Context, text, sourceLang, targetLang string) (string, string, error) {
	src := bingLang(sourceLang)
	dst := bingLang(targetLang)
	return trFanout(ctx, text, 5000, func(chunk string) (string, string, error) {
		return b.once(ctx, chunk, src, dst)
	})
}

func (b *bing) once(ctx context.Context, q, src, dst string) (string, string, error) {
	cred, err := b.credential(ctx)
	if err != nil {
		return "", "", err
	}
	if cred.bearer != "" {
		return b.onceAPI(ctx, cred.bearer, q, src, dst)
	}
	return b.onceWeb(ctx, cred, q, src, dst)
}

// onceAPI is the cognitive-translator route the Edge token unlocks.
func (b *bing) onceAPI(ctx context.Context, token, q, src, dst string) (string, string, error) {
	qv := url.Values{}
	qv.Set("api-version", "3.0")
	if src != "" {
		qv.Set("from", src)
	}
	qv.Set("to", dst)

	body, err := json.Marshal([]map[string]string{{"Text": q}})
	if err != nil {
		return "", "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.apiEndpoint()+"?"+qv.Encode(),
		bytes.NewReader(body))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := b.httpc.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return "", "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("必应翻译 HTTP %d: %s", resp.StatusCode, truncate(string(raw)))
	}
	return parseBingAnswer(raw)
}

// onceWeb is the web translator's own route: a form post the page-minted token
// authorizes, answering in the same shape as the cognitive API.
func (b *bing) onceWeb(ctx context.Context, cred *bingCredential, q, src, dst string) (string, string, error) {
	from := src
	if from == "" {
		from = "auto-detect"
	}
	form := url.Values{}
	form.Set("fromLang", from)
	form.Set("to", dst)
	form.Set("text", q)
	form.Set("key", cred.timestamp)
	form.Set("token", cred.key)

	endpoint := cred.base + "/ttranslatev3?isVertical=1&IG=" + url.QueryEscape(cred.ig) + "&IID=translator.5023.1"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", cred.base+"/translator")
	req.Header.Set("User-Agent", bingUA)

	resp, err := b.httpc.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return "", "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("必应翻译 HTTP %d: %s", resp.StatusCode, truncate(string(raw)))
	}
	return parseBingAnswer(raw)
}

// parseBingAnswer reads the translation and the language it was written in.
func parseBingAnswer(raw []byte) (string, string, error) {
	var out []struct {
		Translations []struct {
			Text string `json:"text"`
			To   string `json:"to"`
		} `json:"translations"`
		Detected struct {
			Language string `json:"language"`
		} `json:"detectedLanguage"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", "", fmt.Errorf("必应翻译响应解析失败: %s", truncate(string(raw)))
	}
	if len(out) == 0 || len(out[0].Translations) == 0 {
		return "", "", fmt.Errorf("必应翻译返回空译文: %s", truncate(string(raw)))
	}
	return out[0].Translations[0].Text, bingSourceCode(out[0].Detected.Language), nil
}

// bingSourceCode turns the response's language id into the app's code.
func bingSourceCode(id string) string {
	switch id {
	case "zh-Hans":
		return "zh-CN"
	case "zh-Hant":
		return "zh-TW"
	case "fil":
		return "tl"
	}
	return id
}
