package dict

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"transdict/internal/netp"
	"transdict/internal/settings"
)

// iciba reads 金山词典. Two public surfaces are used:
//
//   - the word page, whose server-rendered JSON state carries the phonetics,
//     part-of-speech meanings, 词形变化 and 双语例句;
//   - the key-free mobile suggest API, which only answers with meanings but is
//     stable, so it is the fallback when the page changes shape.
//
// Neither needs a key. Both payloads are unofficial, so every accessor is
// optional and the provider reports a miss rather than guessing.
type iciba struct {
	httpc *http.Client
}

var (
	icibaPageEndpoint    = "https://www.iciba.com/word"
	icibaSuggestEndpoint = "https://dict-mobile.iciba.com/interface/index.php"
	icibaUserAgent       = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) " +
		"AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36"
)

const icibaMaxQueryRunes = 60

func init() {
	Register(settings.ServiceType{
		Kind: "iciba", Name: "金山词典", Note: "免密钥，含双语例句", Free: true,
		Site: "https://www.iciba.com/word?w={text}",
	}, func(settings.Provider) (Provider, error) {
		return &iciba{httpc: netp.Client(15 * time.Second)}, nil
	})
}

func (i *iciba) Label() string { return "金山词典" }

func (i *iciba) Explain(ctx context.Context, text, sourceLang, targetLang string) ([]Entry, error) {
	word := cleanLookup(text)
	if word == "" {
		return nil, fmt.Errorf("金山词典：查询内容为空")
	}
	if len([]rune(word)) > icibaMaxQueryRunes {
		return nil, fmt.Errorf("金山词典：查询过长（最多 %d 个字符），词典只收录单词与短语", icibaMaxQueryRunes)
	}

	entries, pageErr := i.fromPage(ctx, word)
	if len(entries) > 0 {
		return entries, nil
	}
	suggest, suggestErr := i.fromSuggest(ctx, word)
	if len(suggest) > 0 {
		return suggest, nil
	}
	if pageErr != nil {
		return nil, pageErr
	}
	if suggestErr != nil {
		return nil, suggestErr
	}
	return nil, fmt.Errorf("金山词典未收录该词：%s", clip(word, 40))
}

func (i *iciba) get(ctx context.Context, endpoint string, q url.Values, accept, referer string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+q.Encode(), nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("User-Agent", icibaUserAgent)
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
	body, status, err := fetch(i.httpc, req)
	if err != nil {
		return nil, status, err
	}
	if status != http.StatusOK {
		return body, status, fmt.Errorf("金山词典 HTTP %d：%s", status, clip(string(body), 120))
	}
	return body, status, nil
}

// fromPage pulls the dictionary out of the __NEXT_DATA__ script the word page
// embeds, which is plain JSON.
func (i *iciba) fromPage(ctx context.Context, word string) ([]Entry, error) {
	q := url.Values{}
	q.Set("w", word)
	body, _, err := i.get(ctx, icibaPageEndpoint, q, "text/html,application/xhtml+xml", "https://www.iciba.com/")
	if err != nil {
		return nil, err
	}
	payload, ok := icibaNextData(body)
	if !ok {
		return nil, nil
	}

	var p icibaPagePayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return nil, nil
	}
	info := p.Props.PageProps.InitialReduxState.Word.WordInfo
	base := info.Base

	var e Entry
	e.Word = strings.TrimSpace(base.WordName)
	if e.Word == "" {
		e.Word = word
	}
	for _, sym := range base.Symbols {
		if s := strings.TrimSpace(sym.PhEn); s != "" {
			e.Phonetics = append(e.Phonetics, Phonetic{Lang: "UK", Symbol: icibaBracket(s), Audio: sym.PhEnMP3})
		}
		if s := strings.TrimSpace(sym.PhAm); s != "" {
			e.Phonetics = append(e.Phonetics, Phonetic{Lang: "US", Symbol: icibaBracket(s), Audio: sym.PhAmMP3})
		}
		for _, part := range icibaPartsOf(sym.Parts) {
			e.Definitions = appendMeaning(e.Definitions, strings.Join(part.means, "；"), part.pos)
		}
	}
	e.Exchange = icibaExchangeLines(base.Exchange)

	for _, group := range info.Sentences {
		for _, s := range group.Sentences {
			source := strings.TrimSpace(s.English)
			if source == "" {
				continue
			}
			translation := strings.TrimSpace(s.Chinese)
			e.Examples = append(e.Examples, Example{Source: source, Translation: translation})
			if len(e.Examples) >= icibaMaxExamples {
				break
			}
		}
		if len(e.Examples) >= icibaMaxExamples {
			break
		}
	}

	if len(e.Definitions) == 0 && len(e.Phonetics) == 0 && len(e.Examples) == 0 {
		return nil, nil
	}
	return []Entry{e}, nil
}

const icibaMaxExamples = 5

// icibaPagePayload is the slice of the word page's server state this provider
// reads. Upstream spells the basic-info key "baesInfo"; that is their key, not
// a typo introduced here.
type icibaPagePayload struct {
	Props struct {
		PageProps struct {
			InitialReduxState struct {
				Word struct {
					WordInfo struct {
						Base struct {
							WordName string        `json:"word_name"`
							Exchange icibaExchange `json:"exchange"`
							Symbols  []struct {
								PhEn    string          `json:"ph_en"`
								PhAm    string          `json:"ph_am"`
								PhEnMP3 string          `json:"ph_en_mp3"`
								PhAmMP3 string          `json:"ph_am_mp3"`
								Parts   json.RawMessage `json:"parts"`
							} `json:"symbols"`
						} `json:"baesInfo"`
						Sentences []struct {
							Tag       string `json:"tag"`
							Sentences []struct {
								English string `json:"en"`
								Chinese string `json:"cn"`
								Origin  string `json:"from"`
							} `json:"sentences"`
						} `json:"new_sentence"`
					} `json:"wordInfo"`
				} `json:"word"`
			} `json:"initialReduxState"`
		} `json:"pageProps"`
	} `json:"props"`
}

// icibaExchange carries the 词形变化 lists the page returns per word.
type icibaExchange struct {
	Plural      []string `json:"word_pl"`
	Third       []string `json:"word_third"`
	Present     []string `json:"word_ing"`
	Past        []string `json:"word_pret"`
	PastPart    []string `json:"word_past"`
	Comparative []string `json:"word_er"`
	Superlative []string `json:"word_est"`
}

type icibaPart struct {
	pos   string
	means []string
}

// icibaPartsOf reads symbols[].parts, which arrives as an array or a single
// object depending on the word.
func icibaPartsOf(raw json.RawMessage) []icibaPart {
	var out []icibaPart
	for _, item := range youdaoAnySlice(raw) {
		var p struct {
			Pos   string   `json:"part"`
			Means []string `json:"means"`
		}
		if err := json.Unmarshal(item, &p); err != nil {
			continue
		}
		var means []string
		for _, m := range p.Means {
			if v := strings.TrimSpace(m); v != "" {
				means = append(means, v)
			}
		}
		if len(means) == 0 {
			continue
		}
		out = append(out, icibaPart{pos: strings.TrimSpace(p.Pos), means: means})
	}
	return out
}

// icibaExchangeLines renders 词形变化 with the labels the site itself shows.
func icibaExchangeLines(ex icibaExchange) []string {
	rows := []struct {
		label string
		forms []string
	}{
		{"复数", ex.Plural},
		{"第三人称单数", ex.Third},
		{"现在分词", ex.Present},
		{"过去式", ex.Past},
		{"过去分词", ex.PastPart},
		{"比较级", ex.Comparative},
		{"最高级", ex.Superlative},
	}
	var out []string
	for _, row := range rows {
		for _, form := range row.forms {
			if v := strings.TrimSpace(form); v != "" {
				out = append(out, row.label+"："+v)
			}
		}
	}
	if len(out) > 12 {
		out = out[:12]
	}
	return out
}

func icibaBracket(ipa string) string {
	if strings.HasPrefix(ipa, "[") || strings.HasPrefix(ipa, "/") {
		return ipa
	}
	return "[" + ipa + "]"
}

// icibaNextData returns the JSON document inside the __NEXT_DATA__ script tag.
func icibaNextData(page []byte) ([]byte, bool) {
	start := bytes.Index(page, []byte(`id="__NEXT_DATA__"`))
	if start < 0 {
		return nil, false
	}
	open := bytes.IndexByte(page[start:], '>')
	if open < 0 {
		return nil, false
	}
	body := page[start+open+1:]
	end := bytes.Index(body, []byte("</script>"))
	if end < 0 {
		return nil, false
	}
	return bytes.TrimSpace(body[:end]), true
}

// fromSuggest uses the mobile endpoint, which answers 释义 only.
func (i *iciba) fromSuggest(ctx context.Context, word string) ([]Entry, error) {
	q := url.Values{}
	q.Set("c", "word")
	q.Set("m", "getsuggest")
	q.Set("nums", "1")
	q.Set("is_need_mean", "1")
	q.Set("word", word)

	body, _, err := i.get(ctx, icibaSuggestEndpoint, q, "application/json", "")
	if err != nil {
		return nil, err
	}
	var payload struct {
		Status int `json:"status"`
		List   []struct {
			Key        string `json:"key"`
			Paraphrase string `json:"paraphrase"`
			Means      []struct {
				Part  string   `json:"part"`
				Means []string `json:"means"`
			} `json:"means"`
		} `json:"message"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, nil
	}

	var out []Entry
	for _, item := range payload.List {
		e := Entry{Word: strings.TrimSpace(item.Key)}
		if e.Word == "" {
			e.Word = word
		}
		for _, m := range item.Means {
			text := strings.Join(m.Means, "；")
			e.Definitions = appendMeaning(e.Definitions, text, strings.TrimSpace(m.Part))
		}
		if len(e.Definitions) == 0 && strings.TrimSpace(item.Paraphrase) != "" {
			e.Definitions = appendMeaning(e.Definitions, item.Paraphrase, "")
		}
		if len(e.Definitions) == 0 {
			continue
		}
		out = append(out, e)
	}
	return out, nil
}
