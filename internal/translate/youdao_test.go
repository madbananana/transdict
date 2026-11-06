package translate

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// jsonapi_s answers a sentence under fanyi.tran and a word under the ec entry.
// Reading only the dictionary block is why a full sentence used to come back
// as "只收录单词与短语"; the machine-translation branch has to win.
func TestYoudaoPayloadPrefersSentenceTranslation(t *testing.T) {
	var p youdaoPayload
	body := `{"fanyi":{"type":"en2zh-CHS","tran":"我昨天把伞落在火车上了。"}}`
	if err := json.Unmarshal([]byte(body), &p); err != nil {
		t.Fatalf("解析响应: %v", err)
	}
	if got := p.translation("I left my umbrella on the train yesterday."); got != "我昨天把伞落在火车上了。" {
		t.Errorf("译文 = %q, want fanyi.tran", got)
	}
	if got := p.detected(); got != "en" {
		t.Errorf("识别语言 = %q, want en", got)
	}
}

// A word fills the ec block; those are its dictionary senses and the fallback
// when the endpoint has no machine translation.
func TestYoudaoPayloadFallsBackToWordSense(t *testing.T) {
	var p youdaoPayload
	body := `{"ec":{"web_trans":["你好世界"]},"web_trans":{"web-translation":[{"key":"hello world","trans":[{"value":"你好世界"}]},{"key":"Hello Kitty","trans":[{"value":"凯蒂猫"}]}]}}`
	if err := json.Unmarshal([]byte(body), &p); err != nil {
		t.Fatalf("解析响应: %v", err)
	}
	if got := p.translation("hello world"); got != "你好世界" {
		t.Errorf("译文 = %q, want ec.web_trans", got)
	}
	if got := p.detected(); got != "" {
		t.Errorf("没有 fanyi.type 时识别语言 = %q, want 空", got)
	}
}

// The web-translation block lists compounds that merely contain the query.
// Only the entry keyed by the query itself is a translation of it.
func TestYoudaoPayloadIgnoresUnrelatedWebEntries(t *testing.T) {
	var p youdaoPayload
	body := `{"web_trans":{"web-translation":[{"key":"hello world","trans":[{"value":"你好世界"}]},{"key":"echo hello world","trans":[{"value":"多个空格输出"}]},{"key":"Hello Kitty Cutie World","trans":[{"value":"凯蒂猫顽皮世界"}]}]}}`
	if err := json.Unmarshal([]byte(body), &p); err != nil {
		t.Fatalf("解析响应: %v", err)
	}
	if got := p.translation("hello world"); got != "你好世界" {
		t.Errorf("译文 = %q, want 与查询相同的词条", got)
	}
	if got := p.translation("hello"); got != "" {
		t.Errorf("没有同名词条时译文 = %q, want 空，不能拿包含它的词条顶替", got)
	}
}

// The fallback knows the entry by a key without the punctuation the user
// typed; 你好，世界 has to find the entry keyed 你好世界.
func TestYoudaoPayloadMatchesKeyWithoutPunctuation(t *testing.T) {
	var p youdaoPayload
	body := `{"web_trans":{"web-translation":[{"key":"你好世界","trans":[{"value":"hello world"}]}]}}`
	if err := json.Unmarshal([]byte(body), &p); err != nil {
		t.Fatalf("解析响应: %v", err)
	}
	if got := p.translation("你好，世界"); got != "hello world" {
		t.Errorf("译文 = %q, want 忽略标点后的同名词条", got)
	}
}

// A Japanese word comes back under newjc, not ec: without reading it the card
// said 没有给出译文 while the site listed the gloss. The part-of-speech banner
// belongs to the dictionary, not to the meaning.
func TestYoudaoReadsAJapaneseWordEntry(t *testing.T) {
	var p youdaoPayload
	body := `{"newjc":{"word":{"head":{"hw":"今日は"},"sense":[{"phrList":[{"jmsy":"【惯用句】 你好，您好，你们好。"}]}]}},"jc":{"$ref":"$.newjc"}}`
	if err := json.Unmarshal([]byte(body), &p); err != nil {
		t.Fatalf("解析响应: %v", err)
	}
	if got := p.translation("こんにちは"); got != "你好，您好，你们好。" {
		t.Errorf("译文 = %q, want 去掉【惯用句】后的释义", got)
	}
	if got := p.detected(); got != "ja" {
		t.Errorf("识别语言 = %q, want ja", got)
	}
}

// 韩汉 and 法汉 entries share a shape; both put a numbering before the gloss.
func TestYoudaoReadsKoreanAndFrenchWordEntries(t *testing.T) {
	var p youdaoPayload
	korean := `{"kc":{"word":[{"trs":[{"tr":[{"l":{"i":["1.【常用句】你好。单词原型为 안녕하다."]}}]}]}]}}`
	if err := json.Unmarshal([]byte(korean), &p); err != nil {
		t.Fatalf("解析韩语响应: %v", err)
	}
	if got := p.translation("안녕하세요"); got != "你好。单词原型为 안녕하다." {
		t.Errorf("韩语译文 = %q", got)
	}
	if got := p.detected(); got != "ko" {
		t.Errorf("识别语言 = %q, want ko", got)
	}

	var f youdaoPayload
	french := `{"fc":{"word":[{"trs":[{"pos":"m.","tr":[{"l":{"i":["早安，日安，白天好，你好"]}}]}]}]}}`
	if err := json.Unmarshal([]byte(french), &f); err != nil {
		t.Fatalf("解析法语响应: %v", err)
	}
	if got := f.translation("bonjour"); got != "早安，日安，白天好，你好" {
		t.Errorf("法语译文 = %q", got)
	}
	if got := f.detected(); got != "fr" {
		t.Errorf("识别语言 = %q, want fr", got)
	}
}

// A word's own entry gives the translation; the web sense can be a brand
// ("苹果公司") rather than the word ("苹果").
func TestYoudaoUsesWordSenseForAWord(t *testing.T) {
	var p youdaoPayload
	body := `{"ec":{"word":{"trs":[{"tran":"苹果；苹果树；苹果公司（Apple Inc.）"}]},"web_trans":["苹果公司"]},"web_trans":{"web-translation":[{"key":"Apple","trans":[{"value":"苹果公司"}]}]}}`
	if err := json.Unmarshal([]byte(body), &p); err != nil {
		t.Fatalf("解析响应: %v", err)
	}
	if got := p.translation("apple"); got != "苹果" {
		t.Errorf("译文 = %q, want 词条首义", got)
	}
}

// A phrase has an entry too, but it explains the expression; the web translation
// is the phrase itself.
func TestYoudaoSkipsWordSenseForAPhrase(t *testing.T) {
	var p youdaoPayload
	body := `{"ec":{"word":{"trs":[{"tran":"你好，世界：一个用于展示基本语法的示例程序。"}]},"web_trans":["你好世界"]}}`
	if err := json.Unmarshal([]byte(body), &p); err != nil {
		t.Fatalf("解析响应: %v", err)
	}
	if got := p.translation("hello world"); got != "你好世界" {
		t.Errorf("译文 = %q, want 短语的网义", got)
	}
}

// 有道 answers anything into Chinese and 中文↔英文; a 日语→韩语 query has no
// route there, so the engine has to know before it spends a request on it.
func TestYoudaoOnlyClaimsPairsItCanAnswer(t *testing.T) {
	y := &youdao{}
	cases := []struct {
		src, dst string
		want     bool
	}{
		{"auto", "zh-CN", true},
		{"ja", "zh-CN", true},
		{"fr", "zh-TW", true},
		{"auto", "en", true},
		{"zh-CN", "en", true},
		{"en", "zh-CN", true},
		{"en", "en", false},
		{"ja", "ko", false},
		{"en", "fr", false},
	}
	for _, c := range cases {
		if got := y.SupportsPair(c.src, c.dst); got != c.want {
			t.Errorf("SupportsPair(%q, %q) = %v, want %v", c.src, c.dst, got, c.want)
		}
	}
}

func TestYoudaoLanguageIds(t *testing.T) {
	cases := map[string]string{
		"":      "auto",
		"auto":  "auto",
		"zh-CN": "zh-CHS",
		"zh-TW": "zh-CHT",
		"en":    "en",
		"ja":    "ja",
		"xx":    "auto",
	}
	for in, want := range cases {
		if got := mapYoudaoLang(in); got != want {
			t.Errorf("mapYoudaoLang(%q) = %q, want %q", in, got, want)
		}
	}
}

// jsonapi_s cannot translate into English, so a 中文→英文 sentence used to end
// up as 没有给出译文 while Google next to it answered normally. The demo
// endpoint is the one that speaks both directions.
func TestYoudaoTranslatesChineseThroughTheDemoEndpoint(t *testing.T) {
	var hits int
	var from, to string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		from, to = r.URL.Query().Get("from"), r.URL.Query().Get("to")
		io.WriteString(w, `{"translation":["The weather is very nice today"],"l":"zh-CHS2en","errorCode":"0"}`)
	}))
	defer srv.Close()

	y := &youdao{httpc: srv.Client(), dictURL: srv.URL, demoURL: srv.URL}
	out, detected, err := y.Translate(context.Background(), "今天天气很好", "zh-CN", "en")
	if err != nil {
		t.Fatalf("Translate 失败: %v", err)
	}
	if out != "The weather is very nice today" {
		t.Errorf("译文 = %q", out)
	}
	if detected != "zh-CN" {
		t.Errorf("识别语言 = %q, want zh-CN", detected)
	}
	if hits != 1 || from != "Auto" || to != "en" {
		t.Errorf("请求 = %d 次 from=%q to=%q, want 一次 Auto→en", hits, from, to)
	}
}

// jsonapi_s takes t=zh-CHT and still answers in 简体. The app promises 繁体
// when that target is chosen, so the answer is restated in 繁体 locally.
func TestYoudaoHonorsATraditionalChineseTarget(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("t"); got != "zh-CHT" {
			t.Errorf("t = %q, want zh-CHT", got)
		}
		io.WriteString(w, `{"fanyi":{"type":"en2zh-CHS","tran":"敏捷的棕色狐狸跳过了懒惰的狗。"}}`)
	}))
	defer srv.Close()

	y := &youdao{httpc: srv.Client(), dictURL: srv.URL, demoURL: srv.URL}
	out, detected, err := y.Translate(context.Background(),
		"The quick brown fox jumps over the lazy dog.", "en", "zh-TW")
	if err != nil {
		t.Fatalf("Translate 失败: %v", err)
	}
	if out != "敏捷的棕色狐狸跳過了懶惰的狗。" {
		t.Errorf("译文 = %q, want 繁体", out)
	}
	if detected != "en" {
		t.Errorf("识别语言 = %q, want en", detected)
	}
}

// Neither endpoint translates 中文→日语, and answering with the Chinese gloss
// would look like a translation. Refuse instead of inventing one.
func TestYoudaoRefusesUnsupportedTargets(t *testing.T) {
	y := &youdao{} // no client: reaching the network would panic
	if out, _, err := y.Translate(context.Background(), "今天天气很好", "zh-CN", "ja"); err == nil || out != "" {
		t.Errorf("中文→日语 = %q, %v, want 错误", out, err)
	}
}

func TestYoudaoDemoReportsAnEmptyAnswer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"errorCode":"102"}`)
	}))
	defer srv.Close()

	y := &youdao{httpc: srv.Client(), demoURL: srv.URL}
	if out, _, err := y.Translate(context.Background(), "apple", "en", "en"); err == nil || out != "" {
		t.Errorf("同语种 = %q, %v, want 错误", out, err)
	}
}

// A burst of queries gets {"msg":"请求频率过快","errorCode":411}. errorCode is a
// number in that answer, so decoding it as a string failed the whole body and
// the card blamed 接口可能已变更 while the only problem was a throttle.
func TestYoudaoDemoReportsItsRateLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"msg":"请求频率过快","errorCode":411}`)
	}))
	defer srv.Close()

	y := &youdao{httpc: srv.Client(), demoURL: srv.URL}
	out, _, err := y.Translate(context.Background(), "今天天气很好", "zh-CN", "en")
	if err == nil {
		t.Fatalf("限流响应 = %q, want 错误", out)
	}
	if msg := err.Error(); !strings.Contains(msg, "请求频率过快") || strings.Contains(msg, "接口可能已变更") {
		t.Errorf("错误 = %q, want 提到限流原因且不甩锅接口", msg)
	}
}
