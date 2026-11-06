package translate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"

	"transdict/internal/netp"
	"transdict/internal/settings"
	"transdict/internal/zhconv"
)

// Youdao queries the dictionary site's jsonapi_s, which answers without a key
// from mainland networks where the Google endpoint is typically blocked.
// A sentence comes back under fanyi.tran (真实机翻); short entries also carry
// web_trans, which is what the same endpoint calls its dictionary block.
// Both shapes are unofficial and can change without notice.
type youdao struct {
	httpc   *http.Client
	dictURL string
	demoURL string
}

const (
	youdaoDictURL = "https://dict.youdao.com/jsonapi_s"
	youdaoDemoURL = "https://aidemo.youdao.com/trans"
)

func init() {
	Register(settings.ServiceType{
		Kind: "youdao", Name: "有道", Note: "免密钥，整句翻译与词典释义", Free: true,
		Default: settings.Provider{Enabled: true},
	}, func(settings.Provider) (Provider, error) {
		return &youdao{
			httpc:   netp.Client(15 * time.Second),
			dictURL: youdaoDictURL,
			demoURL: youdaoDemoURL,
		}, nil
	})
}

func (y *youdao) Label() string { return "有道" }

type youdaoPayload struct {
	Le   string `json:"le"`
	Lang string `json:"lang"`
	// Fanyi is the machine-translation block. Type looks like "en2zh-CHS".
	Fanyi struct {
		Type string `json:"type"`
		Tran string `json:"tran"`
	} `json:"fanyi"`
	// Ec is the word entry. Word.trs carries the real glosses ("苹果；苹果树"),
	// and web_trans the site's web sense, which for a brand word can be the
	// company ("苹果公司") rather than the word.
	Ec struct {
		Word struct {
			Trs []struct {
				Tran string `json:"tran"`
			} `json:"trs"`
		} `json:"word"`
		WebTrans []string `json:"web_trans"`
	} `json:"ec"`
	WebTrans struct {
		WebTranslation []struct {
			Key   string `json:"key"`
			Trans []struct {
				Value string `json:"value"`
			} `json:"trans"`
		} `json:"web-translation"`
	} `json:"web_trans"`
	// JC holds the Japanese word entry. The endpoint names it newjc and leaves
	// jc as a JSON reference to it, so only the data block is read.
	JC youdaoJapanese `json:"newjc"`
	// KC (韩汉) and FC (法汉) share one shape: a list of headwords whose trs
	// carry the glosses.
	KC youdaoForeign `json:"kc"`
	FC youdaoForeign `json:"fc"`
}

// youdaoJapanese is the 日汉 word entry: each sense is a phrList whose jmsy is
// the Chinese explanation of that usage ("【惯用句】 你好，您好，你们好。").
type youdaoJapanese struct {
	Word struct {
		Sense []struct {
			PhrList []struct {
				Jmsy string `json:"jmsy"`
			} `json:"phrList"`
		} `json:"sense"`
	} `json:"word"`
}

// youdaoForeign is the 韩汉/法汉 entry: word[].trs[].tr[].l.i holds the gloss
// strings, sometimes prefixed with a numbering ("1.【常用句】你好。").
type youdaoForeign struct {
	Word []struct {
		Trs []struct {
			Tr []struct {
				L struct {
					I []string `json:"i"`
				} `json:"l"`
			} `json:"tr"`
		} `json:"trs"`
	} `json:"word"`
}

// translation reads the answer from the block that carries it: the machine
// translation for a sentence, the word entry's glosses for a word, and the
// web-translation entry only when it is the queried phrase itself. The other
// web entries are compounds containing the query ("Hello Kitty …"), and
// joining them put a list of unrelated titles under the translation.
func (p youdaoPayload) translation(query string) string {
	if v := strings.TrimSpace(p.Fanyi.Tran); v != "" {
		return v
	}
	// A single word has a real entry; its first sense is the translation. For a
	// phrase the entry explains the expression instead of naming it, so the
	// word-level sense list is skipped there.
	if !strings.ContainsAny(strings.TrimSpace(query), " \t，。,:：;；") {
		if v := p.firstSense(); v != "" {
			return v
		}
		if v := p.foreignSense(); v != "" {
			return v
		}
	}
	if glosses := joinYoudao(p.Ec.WebTrans); glosses != "" {
		return glosses
	}
	q := youdaoKey(query)
	for _, entry := range p.WebTrans.WebTranslation {
		if q == "" || youdaoKey(entry.Key) != q {
			continue
		}
		values := make([]string, 0, len(entry.Trans))
		for _, t := range entry.Trans {
			values = append(values, t.Value)
		}
		if out := joinYoudao(values); out != "" {
			return out
		}
	}
	return ""
}

// firstSense returns the leading gloss of the word entry ("苹果" out of
// "苹果；苹果树；苹果公司（Apple Inc.）").
func (p youdaoPayload) firstSense() string {
	for _, tr := range p.Ec.Word.Trs {
		v := strings.TrimSpace(tr.Tran)
		if v == "" {
			continue
		}
		if i := strings.IndexAny(v, "；;"); i > 0 {
			v = v[:i]
		}
		return strings.TrimSpace(v)
	}
	return ""
}

// foreignSense reads the 日汉/韩汉/法汉 entries. Without this a Japanese or
// French word had no fanyi block and no ec entry, so the card said 没有给出译文
// while the site itself listed a dozen glosses.
func (p youdaoPayload) foreignSense() string {
	senses := make([]string, 0, 3)
	for _, sense := range p.JC.Word.Sense {
		for _, phr := range sense.PhrList {
			if v := cleanGloss(phr.Jmsy); v != "" {
				senses = append(senses, v)
			}
		}
	}
	for _, block := range []youdaoForeign{p.KC, p.FC} {
		for _, word := range block.Word {
			for _, trs := range word.Trs {
				for _, tr := range trs.Tr {
					for _, v := range tr.L.I {
						if v = cleanGloss(v); v != "" {
							senses = append(senses, v)
						}
					}
				}
			}
		}
	}
	return joinYoudao(senses)
}

// cleanGloss drops the dictionary's part-of-speech banner from a foreign gloss:
// "1.【常用句】你好。" is the gloss 你好, and keeping the banner would put
// 常用句 on the card as if it were part of the meaning.
func cleanGloss(s string) string {
	s = strings.TrimSpace(s)
	// "1.【常用句】…" — the numbering is only a marker when it ends the prefix.
	digits := 0
	for digits < len(s) && s[digits] >= '0' && s[digits] <= '9' {
		digits++
	}
	if digits > 0 && digits < len(s) {
		if rest := s[digits:]; strings.HasPrefix(rest, ".") {
			s = strings.TrimSpace(rest[1:])
		} else if strings.HasPrefix(rest, "、") {
			s = strings.TrimSpace(rest[len("、"):])
		}
	}
	if strings.HasPrefix(s, "【") {
		if i := strings.Index(s, "】"); i >= 0 {
			s = strings.TrimSpace(s[i+len("】"):])
		}
	}
	return s
}

// youdaoKey compares a query with a web entry's key the way the site does:
// case, spaces and punctuation are not part of the identity, so 你好，世界 finds
// the entry keyed 你好世界.
func youdaoKey(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// joinYoudao trims and joins senses with the separator a Chinese dictionary
// uses between them.
func joinYoudao(values []string) string {
	parts := make([]string, 0, len(values))
	for _, v := range values {
		if v = strings.TrimSpace(v); v != "" {
			parts = append(parts, v)
		}
	}
	return strings.Join(parts, "；")
}

// detected reports the language the service recognized. The fanyi type is the
// only reliable source ("en2zh-CHS"); le/lang are constants across calls. A
// foreign word entry carries no fanyi block, so the block that answered names
// the language instead.
func (p youdaoPayload) detected() string {
	if left, _, ok := strings.Cut(p.Fanyi.Type, "2"); ok {
		if code := youdaoSourceCode(left); code != "" {
			return code
		}
	}
	switch {
	case len(p.JC.Word.Sense) > 0:
		return "ja"
	case len(p.KC.Word) > 0:
		return "ko"
	case len(p.FC.Word) > 0:
		return "fr"
	}
	return ""
}

func youdaoSourceCode(id string) string {
	switch id {
	case "zh-CHS", "zh-CHT":
		if id == "zh-CHT" {
			return "zh-TW"
		}
		return "zh-CN"
	case "en", "ja", "ko", "fr", "de", "ru":
		return id
	}
	return ""
}

// SupportsPair reports the pairs the two endpoints can answer. jsonapi_s
// translates many sources into Chinese; the demo endpoint is the only 中文→英文
// route, and nothing here speaks 日语→韩语. Leaving the service out beats an
// error card that only repeats the limit beside services that answered.
func (y *youdao) SupportsPair(sourceLang, targetLang string) bool {
	src := mapYoudaoLang(sourceLang)
	switch mapYoudaoLang(targetLang) {
	case "auto", "zh-CHS", "zh-CHT":
		return true
	case "en":
		return src == "auto" || src == "zh-CHS" || src == "zh-CHT"
	}
	return false
}

func (y *youdao) Translate(ctx context.Context, text, sourceLang, targetLang string) (string, string, error) {
	// jsonapi_s only machine-translates into Chinese; its word entries are
	// Chinese glosses too. English targets therefore go through the demo
	// endpoint, and every other target is a pair neither endpoint serves.
	switch target := mapYoudaoLang(targetLang); target {
	case "auto", "zh-CHS", "zh-CHT":
		return y.dictTranslate(ctx, text, target)
	case "en":
		return y.demoTranslate(ctx, text)
	default:
		return "", "", errors.New("有道只支持中文与英语互译")
	}
}

// youdaoURLBudget is smaller than trURLBudget: the dictionary host puts an
// 8 KB ceiling on the whole request head, not just the query string, so the
// user agent and referer have to fit inside the same budget.
const youdaoURLBudget = 6500

// youdaoLatinChunk is the source-character ceiling for Latin prose. The
// dictionary host's machine-translation engine stops reading a Latin query after
// roughly 500 characters — a 16-sentence paragraph came back with 15 sentences
// translated and no error — so English has to be cut shorter than the URL limit
// alone would require. Chinese text is processed whole and keeps the URL budget.
const youdaoLatinChunk = 450

// youdaoBudget picks the chunk size for text: encoded bytes for Chinese (the URL
// is what binds), characters for Latin prose (the engine is what binds).
func youdaoBudget(text string) int {
	var letters, latin int
	for _, r := range text {
		if !unicode.IsLetter(r) {
			continue
		}
		letters++
		if r < 0x250 {
			latin++
		}
	}
	if letters > 0 && latin*5 >= letters*3 {
		return youdaoLatinChunk
	}
	return youdaoURLBudget
}

// dictTranslate asks the dictionary endpoint, which answers in Chinese: the
// machine translation when it has one, otherwise the word's own senses. The
// text travels in the query string, so a long paragraph is cut into chunks that
// fit; 有道 answered 400 before the pieces were bounded.
func (y *youdao) dictTranslate(ctx context.Context, text, target string) (string, string, error) {
	return trFanoutURL(ctx, text, youdaoBudget(text), "",
		func(chunk string) (string, string, error) {
			return y.dictOnce(ctx, chunk, target)
		})
}

func (y *youdao) dictOnce(ctx context.Context, text, target string) (string, string, error) {
	q := url.Values{}
	q.Set("doctype", "json")
	q.Set("jsonversion", "4")
	q.Set("q", text)
	// jsonapi_s only fills fanyi.tran when le is auto: an explicit source (and
	// only then, 中文→英文) drops the machine-translation block entirely, so the
	// card read "有道没有给出译文". The endpoint picks the pair itself from the
	// text; fanyi.type is what reports the direction it chose.
	q.Set("le", "auto")
	q.Set("t", target)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		y.dictURL+"?"+q.Encode(), nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Referer", "https://dict.youdao.com/")

	resp, err := y.httpc.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return "", "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("有道 HTTP %d: %s", resp.StatusCode, truncate(string(body)))
	}

	var payload youdaoPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", "", fmt.Errorf("有道响应解析失败（接口可能已变更）: %s", truncate(string(body)))
	}

	out := payload.translation(text)
	if out == "" {
		return "", payload.detected(), errors.New("有道没有给出译文，请稍后重试或改用其他服务")
	}
	// jsonapi_s accepts t=zh-CHT and then answers in zh-CHS regardless, so a
	// 繁体 target has to be honored here. The converter only restates the
	// answer's script; it never changes the wording.
	if target == "zh-CHT" {
		out = zhconv.Traditional(out)
	}
	return out, payload.detected(), nil
}

// youdaoDemoPayload is the 有道智云 demo endpoint's answer. Unlike jsonapi_s it
// translates 中文→英文, but only between those two languages.
type youdaoDemoPayload struct {
	L           string      `json:"l"` // "zh-CHS2en"
	Translation []string    `json:"translation"`
	ErrorCode   json.Number `json:"errorCode"`
	Msg         string      `json:"msg"`
}

// failure reports the endpoint's own complaint. errorCode is a number in the
// rate-limit answer ("请求频率过快") but a string elsewhere, so decoding it as
// a plain string used to fail the whole body and surface as 接口可能已变更 —
// blaming a broken parser for a throttle the user can wait out.
func (p youdaoDemoPayload) failure() error {
	code := p.ErrorCode.String()
	if code == "" || code == "0" {
		return nil
	}
	if msg := strings.TrimSpace(p.Msg); msg != "" {
		return fmt.Errorf("有道：%s（错误码 %s）", msg, code)
	}
	return fmt.Errorf("有道返回错误码 %s", code)
}

// demoTranslate splits long text so the demo endpoint's GET query stays within
// its URL limit; the answer is English, so two pieces cut out of one line need
// a space between them.
func (y *youdao) demoTranslate(ctx context.Context, text string) (string, string, error) {
	return trFanoutURL(ctx, text, youdaoURLBudget, " ",
		func(chunk string) (string, string, error) {
			return y.demoOnce(ctx, chunk)
		})
}

// demoOnce is one request to the 有道智云 demo endpoint. Its GET query has the
// same URL ceiling as the dictionary endpoint.
func (y *youdao) demoOnce(ctx context.Context, text string) (string, string, error) {
	q := url.Values{}
	q.Set("q", text)
	q.Set("from", "Auto")
	q.Set("to", "en")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		y.demoURL+"?"+q.Encode(), nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Referer", "https://ai.youdao.com/")

	resp, err := y.httpc.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return "", "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("有道 HTTP %d: %s", resp.StatusCode, truncate(string(body)))
	}

	var payload youdaoDemoPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", "", fmt.Errorf("有道响应解析失败（接口可能已变更）: %s", truncate(string(body)))
	}
	if err := payload.failure(); err != nil {
		return "", "", err
	}

	parts := make([]string, 0, len(payload.Translation))
	for _, v := range payload.Translation {
		if v = strings.TrimSpace(v); v != "" {
			parts = append(parts, v)
		}
	}
	if len(parts) == 0 {
		return "", youdaoDemoSource(payload.L), errors.New("有道没有给出译文，请稍后重试或改用其他服务")
	}
	return strings.Join(parts, " "), youdaoDemoSource(payload.L), nil
}

// youdaoDemoSource reads the detected language out of the "zh-CHS2en" direction.
func youdaoDemoSource(direction string) string {
	if left, _, ok := strings.Cut(direction, "2"); ok {
		return youdaoSourceCode(left)
	}
	return ""
}

// mapYoudaoLang turns an app language code into the id jsonapi_s expects.
func mapYoudaoLang(code string) string {
	switch code {
	case "", "auto":
		return "auto"
	case "zh-CN", "zh":
		return "zh-CHS"
	case "zh-TW":
		return "zh-CHT"
	case "en", "ja", "ko", "fr", "de", "ru":
		return code
	default:
		return "auto"
	}
}
