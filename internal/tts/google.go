package tts

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"time"

	"transdict/internal/netp"
	"transdict/internal/settings"
)

// Google Translate's speak button calls translate_tts. The endpoint documents
// client=t plus a per-segment token ("tok"), and rejects anything longer than
// about 200 characters, so long text is split on sentence boundaries and the
// returned MP3 frames are concatenated (raw MPEG audio frames play back
// seamlessly one after another).
var googleTTSEndpoint = "https://translate.google.com/translate_tts"

// googleMaxChars is the payload ceiling we chunk against; a hair below the
// documented ~200-char rejection point.
const googleMaxChars = 190

// client is netp.Client plus the cookie jar this endpoint expects.
func client(def time.Duration, jar http.CookieJar) *http.Client {
	c := netp.Client(def)
	c.Jar = jar
	return c
}

type google struct {
	endpoint string
	httpc    *http.Client
	maxChars int
}

func init() {
	Register(settings.ServiceType{
		Kind: "google", Name: "Google 朗读",
		Note: "免密钥，长文本自动分段拼接",
		Free: true,
		Fields: []settings.Field{
			{Key: settings.FieldBaseURL, Label: "接口地址（可选）", Placeholder: googleTTSEndpoint},
		},
		Default: settings.Provider{Enabled: true},
	}, func(p settings.Provider) (Provider, error) {
		base := strings.TrimRight(p.BaseURL, "/")
		if base == "" {
			base = googleTTSEndpoint
		}
		jar, _ := cookiejar.New(nil)
		return &google{
			endpoint: base,
			maxChars: googleMaxChars,
			httpc:    client(30*time.Second, jar),
		}, nil
	})
}

func (g *google) Label() string { return "Google" }

func (g *google) Speak(ctx context.Context, text, lang string, slow bool) (*Clip, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, fmt.Errorf("Google 朗读：待朗读文本为空")
	}
	tl := googleLang(lang)
	// speed= is the documented slow-down knob; support has varied across
	// Google's rotations of this endpoint, so treat the exact value as
	// best-effort until re-verified live.
	speed := "1"
	if slow {
		speed = "0.5"
	}

	var buf bytes.Buffer
	for _, chunk := range googleChunk(text, g.maxChars) {
		body, err := g.fetch(ctx, chunk, tl, speed)
		if err != nil {
			return nil, err
		}
		buf.Write(body) // MP3 frames concatenate cleanly.
	}
	if buf.Len() == 0 {
		return nil, fmt.Errorf("Google 朗读：接口未返回音频数据")
	}
	return &Clip{MIME: "audio/mpeg", Data: buf.Bytes()}, nil
}

func (g *google) fetch(ctx context.Context, text, tl, speed string) ([]byte, error) {
	q := url.Values{}
	q.Set("client", "t")
	q.Set("dt", "t") // the "t" segment type the endpoint documents.
	q.Set("ie", "UTF-8")
	q.Set("sl", tl)
	q.Set("tl", tl)
	q.Set("text", text)
	q.Set("tok", googleToken(text)) // token computed for this exact segment.
	q.Set("speed", speed)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, g.endpoint+"?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgentChrome)
	req.Header.Set("Referer", "https://translate.google.com/")
	req.Header.Set("Accept", "*/*")

	resp, err := g.httpc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Google 朗读请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("Google 朗读读取响应失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Google 朗读 HTTP %d: %s", resp.StatusCode, errBody(body))
	}
	return body, nil
}

// googleLang turns a canonical language code into the tl value the endpoint
// expects. Anything undetected falls back to English rather than erroring.
func googleLang(code string) string {
	switch code {
	case "", "auto":
		return "en"
	case "zh", "zh-CN":
		return "zh-CN"
	case "zh-TW":
		return "zh-TW"
	default:
		return code
	}
}

// googleChunk splits text into spoken segments no longer than max runes,
// preferring to break on sentence boundaries and falling back to spaces and
// finally hard rune cuts so it never returns an oversized chunk.
func googleChunk(text string, max int) []string {
	if max <= 0 {
		max = googleMaxChars
	}
	runes := []rune(text)

	// Cut on sentence terminators first, keeping the terminator with the
	// segment it ends.
	var parts []string
	start := 0
	for i, r := range runes {
		if googleIsTerm(r) {
			parts = append(parts, string(runes[start:i+1]))
			start = i + 1
		}
	}
	if start < len(runes) {
		parts = append(parts, string(runes[start:]))
	}

	var out []string
	cur := ""
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if len([]rune(p)) > max {
			if cur != "" {
				out = append(out, cur)
				cur = ""
			}
			out = append(out, googleHardSplit(p, max)...)
			continue
		}
		switch {
		case cur == "":
			cur = p
		case len([]rune(cur))+len([]rune(p)) <= max:
			cur += p
		default:
			out = append(out, cur)
			cur = p
		}
	}
	if cur != "" {
		out = append(out, cur)
	}
	if len(out) == 0 {
		return []string{text}
	}
	return out
}

// googleHardSplit packs an over-long segment on spaces, cutting any single
// oversized token by runes so every returned chunk respects max.
func googleHardSplit(s string, max int) []string {
	var out []string
	cur := ""
	for _, w := range strings.Fields(s) {
		if len([]rune(w)) > max {
			if cur != "" {
				out = append(out, cur)
				cur = ""
			}
			for _, part := range splitRunes(w, max) {
				out = append(out, part)
			}
			continue
		}
		if cur == "" {
			cur = w
		} else if len([]rune(cur))+len([]rune(w))+1 <= max {
			cur += " " + w
		} else {
			out = append(out, cur)
			cur = w
		}
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

func splitRunes(s string, max int) []string {
	var out []string
	r := []rune(s)
	for len(r) > 0 {
		n := max
		if n > len(r) {
			n = len(r)
		}
		out = append(out, string(r[:n]))
		r = r[n:]
	}
	return out
}

func googleIsTerm(r rune) bool {
	switch r {
	case '。', '！', '？', '；', '，', '\n', '!', '?', ';', ',':
		return true
	case '.':
		return true
	default:
		return false
	}
}

// googleRL reproduces the rotate/xor inner loop of the documented token
// function over a fixed operator string, using uint32 wrap-around arithmetic
// to mirror JavaScript's 32-bit bitwise semantics.
func googleRL(a uint32, op string) uint32 {
	for c := 0; c < len(op)-2; c += 3 {
		dc := op[c+2]
		var d uint32
		if dc >= 'a' {
			d = uint32(dc) - 87
		} else {
			d = uint32(dc - '0')
		}
		var x uint32
		if op[c+1] == '+' {
			x = a >> d
		} else {
			x = a << d
		}
		if op[c] == '+' {
			a += x
		} else {
			a ^= x
		}
	}
	return a
}

// googleToken computes the translate_tts "tok" for a segment using the
// publicly documented tkk-seeded hash (seed 406644, xor constant 3293161072,
// operator strings "+-a^+6" and "+-3^+b+-f"). The expansion of the text into
// byte codes equals its UTF-8 encoding, so []byte(text) is faithful. Google
// rotates the tkk it ships, so the constants may need refreshing — hence this
// is a package var and the seed is documented in the README notes.
var googleToken = func(text string) string {
	const (
		seed uint32 = 406644
		xor  uint32 = 3293161072
	)
	a := seed
	for _, by := range []byte(text) {
		a += uint32(by)
		a = googleRL(a, "+-a^+6")
	}
	a = googleRL(a, "+-3^+b+-f")
	a ^= xor
	a %= 1000000
	return strconv.FormatUint(uint64(a), 10) + "." + strconv.FormatUint(uint64(a^seed), 10)
}
