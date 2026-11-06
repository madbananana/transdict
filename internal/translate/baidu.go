package translate

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"transdict/internal/netp"
	"transdict/internal/settings"
)

// Baidu 通用翻译 API. Credentials are appid + 密钥; the request signature is
// MD5(appid + q + salt + 密钥) hex-encoded, so the random salt is exposed as an
// injectable package var to keep signatures reproducible under test.
var baiduEndpoint = "https://fanyi-api.baidu.com/api/trans/vip/translate"

var baiduNonce = func() string { return strconv.FormatInt(time.Now().UnixNano(), 10) }

type baidu struct {
	appID   string
	signKey string
	baseURL string
	httpc   *http.Client
}

func init() {
	Register(settings.ServiceType{
		Kind: "baidu", Name: "百度翻译",
		Note: "应用ID(appid)+密钥；签名 MD5(appid+q+salt+密钥)，单段≤2000字",
		Fields: []settings.Field{
			{Key: settings.FieldAppID, Label: "应用ID (appid)", Required: true},
			{Key: settings.FieldAPIKey, Label: "密钥 (key)", Secret: true, Required: true},
			{Key: settings.FieldBaseURL, Label: "接口地址（可留空）", Placeholder: baiduEndpoint},
		},
		Default: settings.Provider{APIKey: "", AppID: ""},
	}, func(p settings.Provider) (Provider, error) {
		return &baidu{
			appID:   strings.TrimSpace(p.AppID),
			signKey: strings.TrimSpace(p.APIKey),
			baseURL: strings.TrimSpace(p.BaseURL),
			httpc:   netp.Client(20 * time.Second),
		}, nil
	})
}

func (b *baidu) Label() string { return "百度翻译" }

func (b *baidu) endpoint() string {
	if b.baseURL != "" {
		return strings.TrimRight(b.baseURL, "/")
	}
	return baiduEndpoint
}

// baiduLang maps a canonical code to Baidu's vocabulary; anything it does not
// list explicitly is passed through as the bare language code.
func baiduLang(code string) string {
	switch code {
	case "", "auto":
		return "auto"
	case "zh-CN", "zh":
		return "zh"
	case "zh-TW":
		return "cht"
	case "ja":
		return "jp"
	case "ko":
		return "kor"
	case "fr":
		return "fra"
	case "es":
		return "spa"
	default:
		return trBase(code)
	}
}

// baiduToCanonical reverses Baidu's detected-language codes back to the app's
// canonical codes so the UI can label them.
var baiduToCanonical = map[string]string{
	"zh": "zh-CN", "cht": "zh-TW", "en": "en", "jp": "ja", "kor": "ko",
	"fra": "fr", "de": "de", "spa": "es", "ru": "ru", "it": "it", "pt": "pt",
	"th": "th", "vi": "vi", "ar": "ar",
}

func (b *baidu) Translate(ctx context.Context, text, sourceLang, targetLang string) (string, string, error) {
	from := baiduLang(sourceLang)
	to := baiduLang(targetLang)
	return trFanout(ctx, text, 2000, func(chunk string) (string, string, error) {
		return b.once(ctx, chunk, from, to)
	})
}

func (b *baidu) once(ctx context.Context, q, from, to string) (string, string, error) {
	salt := baiduNonce()
	sign := fmt.Sprintf("%x", md5.Sum([]byte(b.appID+q+salt+b.signKey)))

	form := url.Values{}
	form.Set("q", q)
	form.Set("from", from)
	form.Set("to", to)
	form.Set("appid", b.appID)
	form.Set("salt", salt)
	form.Set("sign", sign)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.endpoint(),
		strings.NewReader(form.Encode()))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

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
		return "", "", fmt.Errorf("百度翻译 HTTP %d: %s", resp.StatusCode, truncate(string(raw)))
	}

	var payload struct {
		From        string `json:"from"`
		To          string `json:"to"`
		TransResult []struct {
			Src string `json:"src"`
			Dst string `json:"dst"`
		} `json:"trans_result"`
		ErrorCode string `json:"error_code"`
		ErrorMsg  string `json:"error_msg"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", "", fmt.Errorf("百度翻译响应解析失败: %s", truncate(string(raw)))
	}
	if payload.ErrorCode != "" {
		return "", "", fmt.Errorf("百度翻译错误 %s: %s", payload.ErrorCode, payload.ErrorMsg)
	}
	if len(payload.TransResult) == 0 {
		return "", "", fmt.Errorf("百度翻译返回空译文: %s", truncate(string(raw)))
	}

	// Join rows with newlines rather than trimming so chunk-boundary newlines
	// survive when long text is split and reassembled by trFanout.
	dst := make([]string, 0, len(payload.TransResult))
	for _, t := range payload.TransResult {
		dst = append(dst, t.Dst)
	}
	detected := payload.From
	if c, ok := baiduToCanonical[detected]; ok {
		detected = c
	}
	return strings.Join(dst, "\n"), detected, nil
}

// ---------------------------------------------------------------------------
// Shared chunking helpers for every keyed provider in this batch. Names are
// prefixed with tr* (transdict) so they cannot collide with other providers'
// package-level symbols.
// ---------------------------------------------------------------------------

// trFanoutLimit bounds concurrent per-chunk requests.
const trFanoutLimit = 4

// trBase strips a region subtag and lowercases, giving the bare language code
// most vendors key on.
func trBase(code string) string {
	if i := strings.IndexAny(code, "-_"); i >= 0 {
		return strings.ToLower(code[:i])
	}
	return strings.ToLower(code)
}

// trSplit breaks text into at most maxRunes-rune pieces while keeping every
// newline attached to the text that precedes it, so concatenating the results
// reproduces the original paragraph structure. A single line longer than the
// limit is hard-split on rune boundaries.
func trSplit(text string, maxRunes int) []string {
	if maxRunes <= 0 {
		maxRunes = 1
	}
	runes := []rune(text)
	if len(runes) == 0 {
		return nil
	}
	if len(runes) <= maxRunes {
		return []string{text}
	}

	var segments [][]rune
	start := 0
	for i, r := range runes {
		if r == '\n' {
			segments = append(segments, runes[start:i+1])
			start = i + 1
		}
	}
	if start < len(runes) {
		segments = append(segments, runes[start:])
	}

	var chunks []string
	var cur []rune
	for _, seg := range segments {
		if len(seg) > maxRunes {
			if len(cur) > 0 {
				chunks = append(chunks, string(cur))
				cur = nil
			}
			for i := 0; i < len(seg); i += maxRunes {
				end := i + maxRunes
				if end > len(seg) {
					end = len(seg)
				}
				chunks = append(chunks, string(seg[i:end]))
			}
			continue
		}
		if len(cur)+len(seg) > maxRunes {
			chunks = append(chunks, string(cur))
			cur = nil
		}
		cur = append(cur, seg...)
	}
	if len(cur) > 0 {
		chunks = append(chunks, string(cur))
	}
	return chunks
}

// trFanout splits text and translates each chunk concurrently (bounded by
// trFanoutLimit), then joins the results in order. The first non-empty detected
// language is returned. On any chunk failure the first error (by index) wins so
// the message is deterministic.
func trFanout(ctx context.Context, text string, maxRunes int,
	do func(chunk string) (string, string, error)) (string, string, error) {
	chunks := trSplit(text, maxRunes)
	if len(chunks) <= 1 {
		return do(text)
	}
	results, detected, errs := trMap(ctx, chunks, do)

	var sb strings.Builder
	for i := range chunks {
		if errs[i] != nil {
			return "", "", errs[i]
		}
		sb.WriteString(results[i])
	}
	for _, d := range detected {
		if d != "" {
			return sb.String(), d, nil
		}
	}
	return sb.String(), "", nil
}

// trMap runs do over every chunk with a bounded number of requests in flight.
func trMap(ctx context.Context, chunks []string,
	do func(chunk string) (string, string, error)) ([]string, []string, []error) {
	results := make([]string, len(chunks))
	detected := make([]string, len(chunks))
	errs := make([]error, len(chunks))

	sem := make(chan struct{}, trFanoutLimit)
	var wg sync.WaitGroup
	for i := range chunks {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[i], detected[i], errs[i] = do(chunks[i])
		}(i)
	}
	wg.Wait()
	return results, detected, errs
}

// trURLBudget is how many percent-encoded bytes one GET chunk may carry. The
// key-free endpoints answer 400/413/414 when the whole text is put in one URL
// (Google at roughly 18 kB, 有道 below 14 kB), so a long paragraph has to be
// split before it is sent. 8000 leaves room for the other query parameters.
const trURLBudget = 8000

// trURLBytes estimates what one rune costs in a url.Values query: unreserved
// characters stay a byte, everything else ASCII takes three, and a non-ASCII
// rune is three UTF-8 bytes each escaped to %XX.
func trURLBytes(r rune) int {
	switch {
	case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		return 1
	case r == '-', r == '_', r == '.', r == '~', r == ' ':
		return 1
	case r < 0x80:
		return 3
	default:
		return 9
	}
}

// trChunk is one URL-sized piece and whether it ended where the source did: a
// chunk that stops at a newline owes its answer a line break, while one cut out
// of a longer line does not.
type trChunk struct {
	text    string
	lineEnd bool
}

// trSplitURL breaks text into pieces whose encoded size fits budget, preferring
// newline boundaries so the paragraph structure survives the round trip.
func trSplitURL(text string, budget int) []trChunk {
	if budget <= 0 {
		budget = 1
	}
	var lines []string
	start := 0
	for i, r := range text {
		if r == '\n' {
			lines = append(lines, text[start:i+1])
			start = i + 1
		}
	}
	if start < len(text) {
		lines = append(lines, text[start:])
	}

	var out []trChunk
	var cur strings.Builder
	curBytes := 0
	flush := func(lineEnd bool) {
		if cur.Len() == 0 {
			return
		}
		out = append(out, trChunk{text: cur.String(), lineEnd: lineEnd})
		cur.Reset()
		curBytes = 0
	}

	for _, line := range lines {
		lineEnd := strings.HasSuffix(line, "\n")
		body := strings.TrimSuffix(line, "\n")
		cost := 0
		for _, r := range body {
			cost += trURLBytes(r)
		}
		if cost > budget {
			// One line cannot fit: whatever was pending ends mid-line, and the
			// line itself is cut on rune boundaries.
			flush(false)
			var piece strings.Builder
			pieceBytes := 0
			for _, r := range body {
				c := trURLBytes(r)
				if pieceBytes+c > budget && piece.Len() > 0 {
					out = append(out, trChunk{text: piece.String()})
					piece.Reset()
					pieceBytes = 0
				}
				piece.WriteRune(r)
				pieceBytes += c
			}
			if piece.Len() > 0 {
				out = append(out, trChunk{text: piece.String(), lineEnd: lineEnd})
			}
			continue
		}
		if curBytes+cost > budget {
			flush(false)
		}
		cur.WriteString(body)
		curBytes += cost
		if lineEnd {
			flush(true)
		}
	}
	flush(false)
	return out
}

// trFanoutURL is trFanout for providers that take their text in a URL. join is
// what goes between two answers cut out of the same line (" " keeps English
// words apart, "" is right for Chinese); a chunk that ended at a newline joins
// with one.
func trFanoutURL(ctx context.Context, text string, budget int, join string,
	do func(chunk string) (string, string, error)) (string, string, error) {
	chunks := trSplitURL(text, budget)
	if len(chunks) <= 1 {
		return do(text)
	}
	pieces := make([]string, len(chunks))
	for i, c := range chunks {
		pieces[i] = c.text
	}
	results, detected, errs := trMap(ctx, pieces, do)

	var sb strings.Builder
	for i := range chunks {
		if errs[i] != nil {
			return "", "", errs[i]
		}
		if i > 0 {
			if chunks[i-1].lineEnd {
				sb.WriteByte('\n')
			} else {
				sb.WriteString(join)
			}
		}
		sb.WriteString(results[i])
	}
	for _, d := range detected {
		if d != "" {
			return sb.String(), d, nil
		}
	}
	return sb.String(), "", nil
}

// trJoinFor is the separator two pieces of one line get in the answer: Chinese
// and the other unspaced scripts read wrong with an inserted space.
func trJoinFor(target string) string {
	switch trBase(target) {
	case "zh", "ja", "ko", "th":
		return ""
	}
	return " "
}
