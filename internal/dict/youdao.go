package dict

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"

	"transdict/internal/netp"
	"transdict/internal/settings"
)

// youdaoDict reads the JSON API that dict.youdao.com serves to its own web
// client. No key is required, so it is the default dictionary for word lookups.
// The payload is unofficial and its shape varies per word: a field is an object
// for one headword and a one-element array for the next, so every accessor here
// tolerates both and never assumes a key exists.
type youdaoDict struct {
	httpc *http.Client
}

// Package-level so the tests can point them at an httptest server.
var (
	youdaoDictEndpoint  = "https://dict.youdao.com/jsonapi_s"
	youdaoVoiceEndpoint = "https://dict.youdao.com/dictvoice"
)

// The web client only answers for short text, and a long paragraph is a
// translation job rather than a dictionary lookup.
const youdaoMaxQueryRunes = 80

// youdaoMaxExamples bounds the bilingual corpus so one card stays readable.
const youdaoMaxExamples = 5

func init() {
	Register(settings.ServiceType{
		Kind: "youdao", Name: "有道词典", Note: "免密钥，单词查询最完整", Free: true,
		Site:    "https://www.youdao.com/result?word={text}&lang=en",
		Default: settings.Provider{Enabled: true},
	}, func(settings.Provider) (Provider, error) {
		return &youdaoDict{httpc: netp.Client(15 * time.Second)}, nil
	})
}

func (y *youdaoDict) Label() string { return "有道词典" }

func (y *youdaoDict) Explain(ctx context.Context, text, sourceLang, targetLang string) ([]Entry, error) {
	word := cleanLookup(text)
	if word == "" {
		return nil, fmt.Errorf("有道词典：查询内容为空")
	}
	if len([]rune(word)) > youdaoMaxQueryRunes {
		return nil, fmt.Errorf("有道词典：查询过长（最多 %d 个字符），词典只收录单词与短语", youdaoMaxQueryRunes)
	}

	q := url.Values{}
	q.Set("doctype", "json")
	q.Set("jsonversion", "4")
	q.Set("q", word)
	q.Set("le", youdaoLocale(sourceLang, targetLang))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		youdaoDictEndpoint+"?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Referer", "https://dict.youdao.com/")
	req.Header.Set("Accept", "application/json")

	body, status, err := fetch(y.httpc, req)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("有道词典 HTTP %d：%s", status, clip(string(body), 120))
	}

	var payload youdaoPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("有道词典响应解析失败（接口可能已变更）：%s", clip(string(body), 120))
	}
	return payload.entries(word)
}

// youdaoPayload keeps only the sections this provider renders. Everything is
// RawMessage or optional because the API omits whatever it has no data for.
type youdaoPayload struct {
	Ec     json.RawMessage `json:"ec"`
	Ce     json.RawMessage `json:"ce"`
	CeNew  json.RawMessage `json:"ce_new"`
	WGH    json.RawMessage `json:"wuguanghua"`
	Simple json.RawMessage `json:"simple"`
	Sents  struct {
		Pairs json.RawMessage `json:"sentence-pair"`
	} `json:"blng_sents_part"`
	WebTrans struct {
		Items json.RawMessage `json:"web-translation"`
	} `json:"web_trans"`
}

type youdaoWord struct {
	UKPhone      string          `json:"ukphone"`
	USPhone      string          `json:"usphone"`
	Phone        string          `json:"phone"`
	UKSpeech     string          `json:"ukspeech"`
	USSpeech     string          `json:"usspeech"`
	Trs          json.RawMessage `json:"trs"`
	Wfs          json.RawMessage `json:"wfs"`
	ReturnPhrase json.RawMessage `json:"return-phrase"`
}

type youdaoTr struct {
	Pos  string          `json:"pos"`
	Tran string          `json:"tran"`
	Tr   json.RawMessage `json:"tr"`
}

type youdaoWf struct {
	WF struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	} `json:"wf"`
}

type youdaoWebItem struct {
	Key   string `json:"key"`
	Trans []struct {
		Value string `json:"value"`
	} `json:"trans"`
}

type youdaoSent struct {
	Sentence    string `json:"sentence"`
	Translation string `json:"sentence-translation"`
}

// entries turns the payload into headword cards. The ec (英汉双解) block drives
// one entry per word object; the other sections decorate them.
func (p *youdaoPayload) entries(word string) ([]Entry, error) {
	words := youdaoObjects(p.Ec, "word")
	var out []Entry

	for _, raw := range words {
		var w youdaoWord
		if err := json.Unmarshal(raw, &w); err != nil {
			continue
		}
		e := Entry{Word: youdaoHeadword(w.ReturnPhrase, word)}
		e.Phonetics = youdaoPhonetics(w, word)
		e.Definitions = youdaoDefinitions(w.Trs)
		e.Exchange = youdaoExchange(w.Wfs)
		e.Examples = youdaoExamples(p.Sents.Pairs)
		e.Web = youdaoWeb(p.WebTrans.Items, word)
		// A word's web sense often restates a brand it already lists as a real
		// sense ("Apple：苹果公司" beside 苹果公司（Apple Inc.）); repeating it as
		// 网义 made the card look like a company page. Keep only web senses that
		// add something the definitions do not already say.
		e.Web = onlyNewWeb(e.Web, e.Definitions)
		if len(e.Definitions) == 0 && len(e.Web) == 0 && len(e.Phonetics) == 0 {
			continue
		}
		out = append(out, e)
	}

	if len(out) == 0 {
		// Phrases and names often have no ec block at all but do have web
		// translations, so fall back to those before calling it a miss.
		var e Entry
		e.Word = word
		e.Web = youdaoWeb(p.WebTrans.Items, word)
		e.Examples = youdaoExamples(p.Sents.Pairs)
		e.Definitions = youdaoChineseDefinitions(p)
		if phones := youdaoSimplePhonetics(p.Simple, word); len(phones) > 0 {
			e.Phonetics = phones
		}
		if len(e.Web) == 0 && len(e.Examples) == 0 && len(e.Phonetics) == 0 && len(e.Definitions) == 0 {
			return nil, fmt.Errorf("有道词典未收录该词：%s", clip(word, 40))
		}
		out = append(out, e)
	}
	return out, nil
}

// youdaoObjects reads name inside the ec block, accepting both the object and
// the array spelling the API uses.
func youdaoObjects(raw json.RawMessage, name string) []json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	var outer map[string]json.RawMessage
	if err := json.Unmarshal(raw, &outer); err != nil {
		return nil
	}
	inner, ok := outer[name]
	if !ok {
		return nil
	}
	return youdaoAnySlice(inner)
}

// youdaoAnySlice decodes a value that may be an array or a bare object.
func youdaoAnySlice(raw json.RawMessage) []json.RawMessage {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return nil
	}
	if strings.HasPrefix(trimmed, "[") {
		var items []json.RawMessage
		if err := json.Unmarshal([]byte(trimmed), &items); err != nil {
			return nil
		}
		return items
	}
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "\"") {
		return []json.RawMessage{json.RawMessage(trimmed)}
	}
	return nil
}

func youdaoHeadword(raw json.RawMessage, fallback string) string {
	for _, item := range youdaoAnySlice(raw) {
		// "return-phrase" is either a bare string or {"l":{"i":...}}.
		var s string
		if err := json.Unmarshal(item, &s); err == nil && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
		if text := youdaoNestedString(item, "l", "i"); text != "" {
			return text
		}
	}
	return fallback
}

// youdaoNestedString walks two levels of objects and returns the first string
// it finds, which may also be delivered as an array of strings.
func youdaoNestedString(raw json.RawMessage, path ...string) string {
	current := raw
	for _, key := range path {
		var m map[string]json.RawMessage
		if err := json.Unmarshal(current, &m); err != nil {
			return ""
		}
		next, ok := m[key]
		if !ok {
			return ""
		}
		current = next
	}
	var s string
	if err := json.Unmarshal(current, &s); err == nil {
		return strings.TrimSpace(s)
	}
	var list []json.RawMessage
	if err := json.Unmarshal(current, &list); err == nil {
		var parts []string
		for _, item := range list {
			if v := youdaoNestedString(item); v != "" {
				parts = append(parts, v)
			}
		}
		return strings.TrimSpace(strings.Join(parts, " "))
	}
	return ""
}

// youdaoChineseDefinitions renders the 汉英 side of a Chinese headword. The ec
// block is English→Chinese only, so a Chinese lookup used to come back with
// examples and no meaning at all; ce lists the English terms with the note
// explaining where each fits, and ce_new / wuguanghua repeat shorter lists.
func youdaoChineseDefinitions(p *youdaoPayload) []Definition {
	var terms []string
	add := func(s string) {
		for _, part := range strings.FieldsFunc(s, func(r rune) bool {
			return r == '；' || r == ';' || r == '\n'
		}) {
			part = strings.TrimSpace(strings.TrimRight(strings.TrimSpace(part), ":："))
			if part != "" {
				terms = appendUniqueFold(terms, part)
			}
		}
	}

	for _, raw := range youdaoObjects(p.Ce, "word") {
		var w struct {
			Trs []struct {
				Text string `json:"#text"`
			} `json:"trs"`
		}
		if json.Unmarshal(raw, &w) != nil {
			continue
		}
		for _, tr := range w.Trs {
			add(tr.Text)
		}
	}

	var concise struct {
		Word []struct {
			Trs []struct {
				Tr json.RawMessage `json:"tr"`
			} `json:"trs"`
		} `json:"word"`
	}
	if json.Unmarshal(p.CeNew, &concise) == nil {
		for _, w := range concise.Word {
			for _, tr := range w.Trs {
				for _, item := range youdaoAnySlice(tr.Tr) {
					add(youdaoNestedString(item, "l", "i"))
				}
			}
		}
	}

	var wgh struct {
		DataList []struct {
			Trs []struct {
				Tr struct {
					En string `json:"en"`
				} `json:"tr"`
			} `json:"trs"`
		} `json:"dataList"`
	}
	if json.Unmarshal(p.WGH, &wgh) == nil {
		for _, w := range wgh.DataList {
			for _, tr := range w.Trs {
				add(tr.Tr.En)
			}
		}
	}

	if len(terms) == 0 {
		return nil
	}
	if len(terms) > 8 {
		terms = terms[:8]
	}
	return []Definition{{Pos: "英", Meanings: terms}}
}

func appendUniqueFold(list []string, want string) []string {
	for _, have := range list {
		if strings.EqualFold(have, want) {
			return list
		}
	}
	return append(list, want)
}

func youdaoSimplePhonetics(raw json.RawMessage, word string) []Phonetic {
	words := youdaoAnySlice(raw)
	if len(words) == 0 {
		return nil
	}
	var outer map[string]json.RawMessage
	if err := json.Unmarshal(words[0], &outer); err != nil {
		return nil
	}
	list, ok := outer["word"]
	if !ok {
		return nil
	}
	var items []youdaoWord
	if err := json.Unmarshal(list, &items); err != nil {
		var single youdaoWord
		if err := json.Unmarshal(list, &single); err != nil {
			return nil
		}
		items = []youdaoWord{single}
	}
	if len(items) == 0 {
		return nil
	}
	if phones := youdaoPhonetics(items[0], ""); len(phones) > 0 {
		return phones
	}
	// The simple block's phone is the pinyin of a Chinese headword and the
	// transcription of anything else.
	if p := strings.TrimSpace(items[0].Phone); p != "" {
		label := "音标"
		if hasHan(word) {
			label = "拼音"
		}
		return []Phonetic{{Lang: label, Symbol: youdaoBracket(p)}}
	}
	return nil
}

func hasHan(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

func youdaoPhonetics(w youdaoWord, word string) []Phonetic {
	var out []Phonetic
	if s := strings.TrimSpace(w.UKPhone); s != "" {
		out = append(out, Phonetic{Lang: "UK", Symbol: youdaoBracket(s),
			Audio: youdaoAudio(word, w.UKSpeech, 1)})
	}
	if s := strings.TrimSpace(w.USPhone); s != "" {
		out = append(out, Phonetic{Lang: "US", Symbol: youdaoBracket(s),
			Audio: youdaoAudio(word, w.USSpeech, 2)})
	}
	return out
}

func youdaoBracket(ipa string) string {
	if strings.HasPrefix(ipa, "[") || strings.HasPrefix(ipa, "/") {
		return ipa
	}
	return "[" + ipa + "]"
}

// youdaoAudio builds the 朗读 URL. The API's own speech key is preferred
// because it already carries the query string the endpoint expects.
func youdaoAudio(word, speech string, kind int) string {
	q := url.Values{}
	if trimmed := strings.TrimSpace(speech); trimmed != "" {
		parts := strings.SplitN(trimmed, "&", 2)
		q.Set("audio", parts[0])
	} else {
		q.Set("audio", word)
	}
	q.Set("type", strconv.Itoa(kind))
	return youdaoVoiceEndpoint + "?" + q.Encode()
}

// youdaoDefinitions renders the trs block. Two spellings occur: {"pos","tran"}
// and {"pos","tr":[{"l":{"i":[...]}}]}.
func youdaoDefinitions(raw json.RawMessage) []Definition {
	var out []Definition
	for _, item := range youdaoAnySlice(raw) {
		var t youdaoTr
		if err := json.Unmarshal(item, &t); err != nil {
			continue
		}
		texts := []string{}
		if v := strings.TrimSpace(t.Tran); v != "" {
			texts = append(texts, v)
		}
		for _, inner := range youdaoAnySlice(t.Tr) {
			if v := youdaoNestedString(inner, "l", "i"); v != "" {
				texts = append(texts, v)
			}
		}
		for _, text := range texts {
			out = appendMeaning(out, text, t.Pos)
		}
	}
	if len(out) > 8 {
		out = out[:8]
	}
	return out
}

// appendMeaning splits "优良的；能干的" into one definition per POS, so the UI
// can show 词性 headings.
func appendMeaning(out []Definition, text, pos string) []Definition {
	text = strings.TrimSpace(text)
	if text == "" {
		return out
	}
	if pos == "" {
		pos, text = youdaoSplitPos(text)
	}
	meanings := youdaoSplitMeanings(text)
	if len(meanings) == 0 {
		return out
	}
	for i := range out {
		if out[i].Pos == pos {
			out[i].Meanings = append(out[i].Meanings, meanings...)
			return out
		}
	}
	return append(out, Definition{Pos: pos, Meanings: meanings})
}

var youdaoParts = []string{
	"n.", "v.", "vt.", "vi.", "adj.", "adv.", "prep.", "conj.", "pron.",
	"num.", "art.", "aux.", "int.", "abbr.", "modal", "na.", "un.",
}

// youdaoSplitPos peels a leading part-of-speech token off a meaning string.
func youdaoSplitPos(text string) (pos, rest string) {
	fields := strings.Fields(text)
	if len(fields) < 2 {
		return "", text
	}
	head := strings.ToLower(fields[0])
	for _, p := range youdaoParts {
		if head == p {
			return fields[0], strings.TrimSpace(strings.Join(fields[1:], " "))
		}
	}
	return "", text
}

func youdaoSplitMeanings(text string) []string {
	var out []string
	for _, part := range strings.FieldsFunc(text, func(r rune) bool {
		return r == '；' || r == ';' || r == '\n'
	}) {
		if v := strings.TrimSpace(part); v != "" {
			out = append(out, v)
		}
	}
	if len(out) > 12 {
		out = out[:12]
	}
	return out
}

func youdaoExchange(raw json.RawMessage) []string {
	var out []string
	for _, item := range youdaoAnySlice(raw) {
		var wf youdaoWf
		if err := json.Unmarshal(item, &wf); err != nil {
			continue
		}
		name := strings.TrimSpace(wf.WF.Name)
		value := strings.TrimSpace(wf.WF.Value)
		switch {
		case name != "" && value != "":
			out = append(out, name+"："+value)
		case value != "":
			out = append(out, value)
		}
	}
	if len(out) > 12 {
		out = out[:12]
	}
	return out
}

func youdaoWeb(raw json.RawMessage, word string) []WebDefinition {
	var items []youdaoWebItem
	if err := json.Unmarshal(raw, &items); err != nil {
		var single youdaoWebItem
		if err := json.Unmarshal(raw, &single); err != nil {
			return nil
		}
		items = []youdaoWebItem{single}
	}
	// The block is a list of compounds containing the query as well as the word
	// itself ("Apple Pay", "BIG APPLE" beside "apple"). Only the entry keyed by
	// the query belongs in this word's card; the rest pulled the card toward
	// whichever phrase the site had seen most often.
	want := youdaoWordKey(word)
	var out []WebDefinition
	for _, item := range items {
		key := strings.TrimSpace(item.Key)
		if key == "" || (want != "" && youdaoWordKey(key) != want) {
			continue
		}
		w := WebDefinition{Value: key}
		seen := map[string]bool{}
		for _, t := range item.Trans {
			v := strings.TrimSpace(t.Value)
			if v == "" || seen[v] {
				continue
			}
			seen[v] = true
			w.Conts = append(w.Conts, v)
		}
		if len(w.Conts) == 0 {
			continue
		}
		if len(w.Conts) > 6 {
			w.Conts = w.Conts[:6]
		}
		out = append(out, w)
		if len(out) >= 10 {
			break
		}
	}
	return out
}

// onlyNewWeb drops web senses whose glosses already appear among the word's
// definitions. A brand entry such as "苹果公司" repeats the third sense of apple
// and should not be printed a second time under 网义.
func onlyNewWeb(web []WebDefinition, defs []Definition) []WebDefinition {
	if len(web) == 0 || len(defs) == 0 {
		return web
	}
	known := make([]string, 0, len(defs))
	for _, d := range defs {
		for _, m := range d.Meanings {
			if m = strings.TrimSpace(m); m != "" {
				known = append(known, m)
			}
		}
	}
	var out []WebDefinition
	for _, w := range web {
		var conts []string
		for _, c := range w.Conts {
			c = strings.TrimSpace(c)
			if c == "" {
				continue
			}
			seen := false
			for _, m := range known {
				if strings.Contains(m, c) {
					seen = true
					break
				}
			}
			if !seen {
				conts = append(conts, c)
			}
		}
		if len(conts) > 0 {
			w.Conts = conts
			out = append(out, w)
		}
	}
	return out
}

// youdaoWordKey compares headwords the way the site does, ignoring case,
// spacing and punctuation.
func youdaoWordKey(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func youdaoExamples(raw json.RawMessage) []Example {
	var out []Example
	for _, item := range youdaoAnySlice(raw) {
		var s youdaoSent
		if err := json.Unmarshal(item, &s); err != nil {
			continue
		}
		source := stripHTMLTags(strings.TrimSpace(s.Sentence))
		if source == "" {
			continue
		}
		out = append(out, Example{Source: source,
			Translation: strings.TrimSpace(stripHTMLTags(s.Translation))})
		if len(out) >= youdaoMaxExamples {
			break
		}
	}
	return out
}

// youdaoLocale picks the language the dictionary is queried in. The web API
// only separates Chinese from everything else for the ec block.
func youdaoLocale(sourceLang, targetLang string) string {
	switch {
	case strings.HasPrefix(sourceLang, "zh"):
		// le=zh answers with the Chinese-only block, which carries no
		// definitions at all; the bidirectional eng block is where 汉英 terms
		// and the pinyin live.
		return "eng"
	case strings.HasPrefix(sourceLang, "ja"):
		return "jp"
	case strings.HasPrefix(sourceLang, "ko"):
		return "kor"
	case sourceLang == "" || sourceLang == "auto" || sourceLang == "en":
		if strings.HasPrefix(targetLang, "zh") || targetLang == "en" || targetLang == "" {
			return "eng"
		}
		return "eng"
	default:
		return "eng"
	}
}

func fetch(httpc *http.Client, req *http.Request) ([]byte, int, error) {
	resp, err := httpc.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return body, resp.StatusCode, nil
}

// cleanLookup normalises a selection before it reaches a dictionary: one
// whitespace run, no trailing punctuation the headword never carries.
func cleanLookup(text string) string {
	fields := strings.Fields(text)
	joined := strings.Join(fields, " ")
	if joined == "" {
		return ""
	}
	return strings.Trim(joined, "　,.!?;:。，！？；：\"'“”‘’()（）[]【】")
}

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
