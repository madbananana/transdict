package tts

import (
	"context"
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

// Bing's web translator serves speech through the same abuse-prevention token
// as its translate endpoint: the /translator page embeds params_AbusePrevention
// Helper = [issued-ms,"token",ttl-ms], and tfettts wants that token plus an
// SSML body. The host is read from the page's final URL because cn.bing.com
// redirects to www.bing.com outside mainland networks.
var bingTranslator = "https://cn.bing.com/translator"

const bingMaxRunes = 7000

var (
	bingIGPattern  = regexp.MustCompile(`IG:\s*"([^"]+)"`)
	bingIIDPattern = regexp.MustCompile(`data-iid\s*=\s*"([^"]+)"`)
	bingAbusePatt  = regexp.MustCompile(`params_AbusePreventionHelper\s*=\s*\[([^\]]+)\]`)
)

type bingVoice struct{ lang, name string }

// bingVoices follows Microsoft's public neural-voice list. The language keys
// are our canonical codes; an unknown language falls back to the English
// voice rather than failing, because a silent button is worse than an
// English-accented reading.
var bingVoices = map[string]bingVoice{
	"zh-CN": {"zh-CN", "zh-CN-XiaoxiaoNeural"},
	"zh-TW": {"zh-TW", "zh-TW-HsiaoChenNeural"},
	"en":    {"en-US", "en-US-JennyNeural"},
	"ja":    {"ja-JP", "ja-JP-NanamiNeural"},
	"ko":    {"ko-KR", "ko-KR-SunHiNeural"},
	"fr":    {"fr-FR", "fr-FR-DeniseNeural"},
	"es":    {"es-ES", "es-ES-ElviraNeural"},
	"pt":    {"pt-PT", "pt-PT-RaquelNeural"},
	"it":    {"it-IT", "it-IT-ElsaNeural"},
	"de":    {"de-DE", "de-DE-KatjaNeural"},
	"ru":    {"ru-RU", "ru-RU-SvetlanaNeural"},
	"ar":    {"ar-EG", "ar-EG-SalmaNeural"},
	"sv":    {"sv-SE", "sv-SE-HedvigNeural"},
	"ro":    {"ro-RO", "ro-RO-AlinaNeural"},
	"th":    {"th-TH", "th-TH-PremwadeeNeural"},
	"sk":    {"sk-SK", "sk-SK-ViktoriaNeural"},
	"nl":    {"nl-NL", "nl-NL-ColetteNeural"},
	"cs":    {"cs-CZ", "cs-CZ-AntoninNeural"},
	"tr":    {"tr-TR", "tr-TR-EmelNeural"},
	"el":    {"el-GR", "el-GR-AthinaNeural"},
	"da":    {"da-DK", "da-DK-ChristelNeural"},
	"fi":    {"fi-FI", "fi-FI-NooraNeural"},
	"pl":    {"pl-PL", "pl-PL-AgnieszkaNeural"},
	"lt":    {"lt-LT", "lt-LT-OnaNeural"},
	"lv":    {"lv-LV", "lv-LV-EveritaNeural"},
	"uk":    {"uk-UA", "uk-UA-OstapNeural"},
	"bg":    {"bg-BG", "bg-BG-KalinaNeural"},
	"id":    {"id-ID", "id-ID-DamayantiNeural"},
	"ms":    {"ms-MY", "ms-MY-OsmanNeural"},
	"sl":    {"sl-SI", "sl-SI-PetraNeural"},
	"et":    {"et-EE", "et-EE-AnuNeural"},
	"vi":    {"vi-VN", "vi-VN-HoaiMyNeural"},
	"fa":    {"fa-IR", "fa-IR-SimaNeural"},
	"hi":    {"hi-IN", "hi-IN-MadhurNeural"},
	"ur":    {"ur-PK", "ur-PK-AsadNeural"},
	"tl":    {"fil-PH", "fil-PH-AlingNeural"},
	"lo":    {"lo-LA", "lo-LA-AcharaNeural"},
	"my":    {"my-MM", "my-MM-ShanNeural"},
	"no":    {"nb-NO", "nb-NO-PernilleNeural"},
	"sr":    {"sr-SP", "sr-SP-LjubicaNeural"},
	"hr":    {"hr-HR", "hr-HR-SreckoNeural"},
	"mn":    {"mn-MN", "mn-MN-NarangerelNeural"},
	"he":    {"he-IL", "he-IL-HilaNeural"},
	"ka":    {"ka-GE", "ka-GE-EkaNeural"},
}

type bing struct {
	translator string
	httpc      *http.Client

	mu      sync.Mutex
	origin  string
	ig, iid string
	key     string
	token   string
	expires time.Time
}

func init() {
	Register(settings.ServiceType{
		Kind: "bing", Name: "必应朗读",
		Note: "免密钥，微软神经网络语音，英式英语用 en-GB",
		Free: true,
		Fields: []settings.Field{
			{Key: settings.FieldBaseURL, Label: "接口地址（可选）", Placeholder: bingTranslator},
		},
		Default: settings.Provider{Enabled: false},
	}, func(p settings.Provider) (Provider, error) {
		page := strings.TrimSpace(p.BaseURL)
		if page == "" {
			page = bingTranslator
		}
		return &bing{translator: page, httpc: netp.Client(25 * time.Second)}, nil
	})
}

func (b *bing) Label() string { return "Bing" }

func (b *bing) Speak(ctx context.Context, text, lang string, slow bool) (*Clip, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, fmt.Errorf("Bing 朗读：待朗读文本为空")
	}
	limit := bingMaxRunes
	if isCJK(lang) {
		limit = 2000
	}
	ssml := bingSSML(clampRunes(text, limit), lang, slow)

	for attempt := 0; attempt < 2; attempt++ {
		if err := b.ensureToken(ctx); err != nil {
			return nil, err
		}
		clip, retry, err := b.speakWithToken(ctx, ssml)
		if err != nil {
			return nil, err
		}
		if !retry {
			return clip, nil
		}
		// The token aged out between the expiry check and the request; a fresh
		// page load is the only fix the endpoint offers.
		b.invalidate()
	}
	return nil, fmt.Errorf("Bing 朗读：语音令牌已失效，请重试")
}

func (b *bing) ensureToken(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.token != "" && time.Now().Before(b.expires) {
		return nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, b.translator, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgentChrome)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")

	resp, err := b.httpc.Do(req)
	if err != nil {
		return fmt.Errorf("Bing 朗读获取令牌失败: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return fmt.Errorf("Bing 朗读读取页面失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Bing 朗读获取令牌失败（HTTP %d）", resp.StatusCode)
	}

	html := string(body)
	abuse := bingAbusePatt.FindStringSubmatch(html)
	if len(abuse) != 2 {
		return fmt.Errorf("Bing 朗读：页面里没有语音令牌")
	}
	parts := strings.Split(strings.ReplaceAll(abuse[1], `"`, ""), ",")
	if len(parts) < 3 {
		return fmt.Errorf("Bing 朗读：语音令牌格式不对")
	}
	issued, err := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
	if err != nil {
		return fmt.Errorf("Bing 朗读：语音令牌格式不对")
	}
	ttl, err := strconv.ParseInt(strings.TrimSpace(parts[2]), 10, 64)
	if err != nil || ttl <= 0 {
		ttl = 3600000
	}

	if m := bingIGPattern.FindStringSubmatch(html); len(m) == 2 {
		b.ig = m[1]
	}
	if m := bingIIDPattern.FindStringSubmatch(html); len(m) == 2 {
		b.iid = m[1]
	}
	b.key = strings.TrimSpace(parts[0])
	b.token = strings.TrimSpace(parts[1])
	// Refresh at half-life: the token stays usable while the next page load is
	// cheap, instead of failing exactly when it matters.
	if ttl > 1000 {
		ttl /= 2
	}
	b.expires = time.UnixMilli(issued + ttl)

	if resp.Request != nil && resp.Request.URL != nil {
		u := resp.Request.URL
		b.origin = u.Scheme + "://" + u.Host
	} else {
		b.origin = "https://www.bing.com"
	}
	return nil
}

func (b *bing) invalidate() {
	b.mu.Lock()
	b.token = ""
	b.mu.Unlock()
}

func (b *bing) speakWithToken(ctx context.Context, ssml string) (*Clip, bool, error) {
	b.mu.Lock()
	origin, ig, iid, key, token := b.origin, b.ig, b.iid, b.key, b.token
	b.mu.Unlock()

	u, err := url.Parse(origin + "/tfettts")
	if err != nil {
		return nil, false, err
	}
	q := u.Query()
	q.Set("isVertical", "1")
	q.Set("IG", ig)
	q.Set("IID", iid)
	u.RawQuery = q.Encode()

	form := url.Values{}
	form.Set("ssml", ssml)
	form.Set("token", token)
	form.Set("key", key)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), strings.NewReader(form.Encode()))
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", userAgentChrome)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", origin+"/translator")

	resp, err := b.httpc.Do(req)
	if err != nil {
		return nil, false, fmt.Errorf("Bing 朗读请求失败: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, false, fmt.Errorf("Bing 朗读读取响应失败: %w", err)
	}
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
		return nil, true, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("Bing 朗读 HTTP %d：%s", resp.StatusCode, errBody(body))
	}
	if !isMP3(body) {
		return nil, false, fmt.Errorf("Bing 朗读：接口未返回音频数据")
	}
	return &Clip{MIME: "audio/mpeg", Data: body}, false, nil
}

// bingSSML builds the markup tfettts expects. Text must be XML-escaped; the
// prosody rate is where 慢速 lives, since the endpoint has no speed parameter.
func bingSSML(text, lang string, slow bool) string {
	voice, ok := bingVoices[lang]
	if !ok {
		voice = bingVoices["en"]
	}
	rate := "-10%"
	if slow {
		rate = "-40%"
	}
	return `<speak version="1.0" xml:lang='` + voice.lang + `'>` +
		`<voice name='` + voice.name + `'>` +
		`<prosody rate='` + rate + `'>` + xmlEscape(text) + `</prosody>` +
		`</voice></speak>`
}

func xmlEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;",
		`"`, "&quot;", "'", "&apos;")
	return r.Replace(s)
}

func isCJK(lang string) bool {
	return strings.HasPrefix(lang, "zh") || strings.HasPrefix(lang, "ja") || strings.HasPrefix(lang, "ko")
}
