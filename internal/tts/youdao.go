package tts

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"transdict/internal/netp"
	"transdict/internal/settings"
)

// Youdao's public dictionary-sound endpoint answers for one dictionary entry,
// not for a sentence: text it does not know comes back as HTTP 500 with
// "returned null audio". Two other quirks matter here — a request without the
// le= hint returns a 1.9 kB silent placeholder even for 你好, and the audio has
// to be downloaded for the failure to be visible, since an <audio> element
// cannot report it. The clip is therefore fetched, and a failure lets the
// engine move on to the next configured voice.
var youdaoDictvoice = "https://dict.youdao.com/dictvoice"

// youdaoMaxAudioRunes bounds the audio= payload: the endpoint silently
// truncates very long text, so we clamp rather than hand back a clipped clip.
const youdaoMaxAudioRunes = 1000

type youdao struct {
	endpoint string
	httpc    *http.Client
}

func init() {
	Register(settings.ServiceType{
		Kind: "youdao", Name: "有道朗读",
		Note: "免密钥，词典词条发音；长句会换用下一个朗读服务",
		Free: true,
		Fields: []settings.Field{
			{Key: settings.FieldBaseURL, Label: "接口地址（可选）", Placeholder: youdaoDictvoice},
		},
		Default: settings.Provider{Enabled: true},
	}, func(p settings.Provider) (Provider, error) {
		base := strings.TrimRight(p.BaseURL, "/")
		if base == "" {
			base = youdaoDictvoice
		}
		return &youdao{endpoint: base, httpc: netp.Client(20 * time.Second)}, nil
	})
}

func (y *youdao) Label() string { return "有道" }

// Speak downloads the clip. slow cannot be lowered here — dictvoice documents
// only audio=, type= and le= — so the player applies the rate change.
func (y *youdao) Speak(ctx context.Context, text, lang string, slow bool) (*Clip, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, fmt.Errorf("有道朗读：待朗读文本为空")
	}
	q := url.Values{}
	q.Set("audio", clampRunes(text, youdaoMaxAudioRunes))
	q.Set("type", youdaoVoiceType(lang))
	if le := youdaoLanguage(lang); le != "" {
		q.Set("le", le)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, y.endpoint+"?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgentChrome)
	req.Header.Set("Referer", "https://dict.youdao.com/")

	resp, err := y.httpc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("有道朗读请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("有道朗读读取响应失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("有道朗读只念词条，这段文本没有音频（HTTP %d）", resp.StatusCode)
	}
	if !isMP3(body) {
		return nil, fmt.Errorf("有道朗读：接口未返回音频数据")
	}
	_ = slow
	return &Clip{MIME: "audio/mpeg", Data: body}, nil
}

// youdaoVoiceType maps a canonical language code to the dictvoice type value:
// 1 = 英音(UK), 2 = 美音(US). English defaults to US; Chinese — and every other
// language, which the endpoint serves with its own native voice — uses type 1.
func youdaoVoiceType(lang string) string {
	if strings.HasPrefix(lang, "en") {
		return "2"
	}
	return "1"
}

// youdaoLanguage is the le= hint that picks the native voice. Without it the
// endpoint answers 200 with a short silent clip for 中文, 日语 and 韩语.
func youdaoLanguage(lang string) string {
	switch lang {
	case "", "auto":
		return ""
	}
	if i := strings.IndexByte(lang, '-'); i > 0 {
		return lang[:i]
	}
	return lang
}

// isMP3 reports whether b starts like an MPEG audio stream: an ID3 tag or an
// MPEG frame header (eleven set sync bits).
func isMP3(b []byte) bool {
	if len(b) >= 3 && string(b[:3]) == "ID3" {
		return true
	}
	return len(b) >= 2 && b[0] == 0xFF && b[1]&0xE0 == 0xE0
}
