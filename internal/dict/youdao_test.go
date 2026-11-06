package dict

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"transdict/internal/settings"
)

// youdaoFixture is shaped like a real jsonapi_s answer, including the two
// spellings of the ec block: "word" as an array here, and meanings delivered
// both as {"pos","tran"} and as {"tr":[{"l":{"i":[...]}}]}.
const youdaoFixture = `{
  "ec": {
    "word": [
      {
        "ukphone": "ˈæp(ə)l",
        "usphone": "ˈæp(ə)l",
        "ukspeech": "apple&type=1",
        "usspeech": "apple&type=2",
        "return-phrase": "apple",
        "trs": [
          {"pos": "n.", "tran": "苹果；苹果树；苹果公司（Apple Inc.）"},
          {"tr": [{"l": {"i": ["adj. 苹果的；苹果状的"]}}]}
        ],
        "wfs": [
          {"wf": {"name": "复数", "value": "apples"}},
          {"wf": {"name": "", "value": ""}}
        ]
      }
    ]
  },
  "simple": {"query": "apple", "word": [{"ukphone": "ˈæp(ə)l"}]},
  "web_trans": {
    "web-translation": [
      {"key": "apple", "trans": [{"value": "苹果"}, {"value": "苹果公司"}, {"value": "苹果"}]},
      {"key": "Apple Inc.", "trans": [{"value": "苹果公司"}]},
      {"key": "no-trans-entry", "trans": []}
    ]
  },
  "blng_sents_part": {
    "sentence-pair": [
      {"sentence": "He ate the <b>apple</b>, stalk and all.", "sentence-translation": "他把那个苹果吃了个干净。"},
      {"sentence": "", "sentence-translation": "空句子应被跳过"}
    ]
  }
}`

func newYoudaoServer(t *testing.T, status int, body string, seen *http.Request) Provider {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if seen != nil {
			*seen = *r.Clone(r.Context())
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)

	original := youdaoDictEndpoint
	youdaoDictEndpoint = server.URL + "/jsonapi_s"
	t.Cleanup(func() { youdaoDictEndpoint = original })

	p, err := New(settings.Provider{ID: "dict-youdao", Category: settings.CatDict,
		Kind: "youdao", Label: "有道词典", Enabled: true})
	if err != nil {
		t.Fatalf("New(youdao) 失败: %v", err)
	}
	return p
}

func TestYoudaoRequestCarriesQuery(t *testing.T) {
	var seen http.Request
	p := newYoudaoServer(t, http.StatusOK, `{}`, &seen)

	_, explainErr := p.Explain(context.Background(), "  apple  ", "en", "zh-CN")
	if explainErr == nil {
		t.Fatal("空响应应返回错误，而不是空条目")
	}
	if !strings.Contains(errText(explainErr), "未收录") {
		t.Fatalf("空响应错误文本 = %q, 期望包含「未收录」", errText(explainErr))
	}

	q := seen.URL.Query()
	if got := q.Get("q"); got != "apple" {
		t.Errorf("q = %q, 期望 apple", got)
	}
	if got := q.Get("le"); got != "eng" {
		t.Errorf("le = %q, 期望 eng", got)
	}
	if got := q.Get("doctype"); got != "json" {
		t.Errorf("doctype = %q, 期望 json", got)
	}
	if got := q.Get("jsonversion"); got != "4" {
		t.Errorf("jsonversion = %q, 期望 4", got)
	}
	if got := seen.Header.Get("User-Agent"); got == "" {
		t.Error("应带上 User-Agent，否则站点会拒绝")
	}
	if got := seen.Header.Get("Referer"); !strings.Contains(got, "dict.youdao.com") {
		t.Errorf("Referer = %q, 期望指向有道词典", got)
	}
}

func TestYoudaoLocale(t *testing.T) {
	cases := []struct{ src, dst, want string }{
		{"", "zh-CN", "eng"},
		{"auto", "zh-CN", "eng"},
		{"en", "zh-CN", "eng"},
		// 中文词条只有 eng 字典带回汉英释义与拼音；le=zh 只给网络释义。
		{"zh-CN", "en", "eng"},
		{"ja", "zh-CN", "jp"},
		{"ko", "zh-CN", "kor"},
		{"de", "zh-CN", "eng"},
	}
	for _, c := range cases {
		if got := youdaoLocale(c.src, c.dst); got != c.want {
			t.Errorf("youdaoLocale(%q,%q) = %q, 期望 %q", c.src, c.dst, got, c.want)
		}
	}
}

func TestYoudaoExplainBuildsEntry(t *testing.T) {
	p := newYoudaoServer(t, http.StatusOK, youdaoFixture, nil)

	entries, err := p.Explain(context.Background(), "apple", "en", "zh-CN")
	if err != nil {
		t.Fatalf("Explain 失败: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("条目数 = %d, 期望 1", len(entries))
	}
	e := entries[0]

	if e.Word != "apple" {
		t.Errorf("Word = %q, 期望 apple", e.Word)
	}
	if len(e.Phonetics) != 2 {
		t.Fatalf("Phonetics = %+v, 期望英美各一条", e.Phonetics)
	}
	if e.Phonetics[0].Lang != "UK" || e.Phonetics[0].Symbol != "[ˈæp(ə)l]" {
		t.Errorf("英式音标 = %+v, 期望 UK [ˈæp(ə)l]", e.Phonetics[0])
	}
	if e.Phonetics[1].Lang != "US" {
		t.Errorf("美式音标 = %+v, 期望 US", e.Phonetics[1])
	}
	if !strings.HasPrefix(e.Phonetics[0].Audio, youdaoVoiceEndpoint) ||
		!strings.Contains(e.Phonetics[0].Audio, "type=1") {
		t.Errorf("英式朗读 URL = %q", e.Phonetics[0].Audio)
	}
	if !strings.Contains(e.Phonetics[1].Audio, "type=2") {
		t.Errorf("美式朗读 URL = %q", e.Phonetics[1].Audio)
	}

	if len(e.Definitions) != 2 {
		t.Fatalf("Definitions = %+v, 期望两条词性", e.Definitions)
	}
	if e.Definitions[0].Pos != "n." || len(e.Definitions[0].Meanings) != 3 {
		t.Errorf("首个释义 = %+v", e.Definitions[0])
	}
	if e.Definitions[1].Pos != "adj." || strings.Join(e.Definitions[1].Meanings, "|") != "苹果的|苹果状的" {
		t.Errorf("第二个释义 = %+v, 期望从文本中拆出 adj. 并按分号分条", e.Definitions[1])
	}

	wantExchange := []string{"复数：apples"}
	if len(e.Exchange) != 1 || e.Exchange[0] != wantExchange[0] {
		t.Errorf("Exchange = %v, 期望 %v", e.Exchange, wantExchange)
	}

	// The payload's web entry repeats 苹果 and 苹果公司, both already listed as
	// senses above; printing them again under 网义 made the brand look like a
	// separate meaning of the word.
	if len(e.Web) != 0 {
		t.Errorf("Web = %+v, 期望重复义不再单列", e.Web)
	}

	if len(e.Examples) != 1 {
		t.Fatalf("Examples = %+v, 期望一条双语例句", e.Examples)
	}
	if e.Examples[0].Source != "He ate the apple, stalk and all." {
		t.Errorf("例句原文 = %q, 期望去掉 <b> 标签", e.Examples[0].Source)
	}
	if e.Examples[0].Translation != "他把那个苹果吃了个干净。" {
		t.Errorf("例句译文 = %q", e.Examples[0].Translation)
	}
}

// The duplicate filter must not throw away a web sense the entry does not have:
// only glosses already printed under definitions are dropped.
func TestYoudaoKeepsWebSenseThatAddsMeaning(t *testing.T) {
	body := `{"ec":{"word":{"trs":[{"tran":"苹果"}]}},"web_trans":{"web-translation":[{"key":"apple","trans":[{"value":"苹果"},{"value":"苹果公司"}]}]}}`
	p := newYoudaoServer(t, http.StatusOK, body, nil)

	entries, err := p.Explain(context.Background(), "apple", "en", "zh-CN")
	if err != nil {
		t.Fatalf("Explain 失败: %v", err)
	}
	if len(entries) != 1 || len(entries[0].Web) != 1 {
		t.Fatalf("条目 = %+v, 期望保留一条新增网义", entries)
	}
	if got := strings.Join(entries[0].Web[0].Conts, "|"); got != "苹果公司" {
		t.Errorf("网络释义 = %q, 期望只去掉与释义重复的义项", got)
	}
}

// Some headwords have no ec block at all, only web translations: the provider
// must still answer rather than report a miss.
func TestYoudaoFallsBackToWebWhenNoEc(t *testing.T) {
	body := `{"web_trans":{"web-translation":[{"key":"Temple Run","trans":[{"value":"神庙逃亡"}]}]}}`
	p := newYoudaoServer(t, http.StatusOK, body, nil)

	entries, err := p.Explain(context.Background(), "Temple Run", "en", "zh-CN")
	if err != nil {
		t.Fatalf("Explain 失败: %v", err)
	}
	if len(entries) != 1 || entries[0].Word != "Temple Run" {
		t.Fatalf("条目 = %+v", entries)
	}
	if len(entries[0].Web) != 1 || entries[0].Web[0].Conts[0] != "神庙逃亡" {
		t.Errorf("网络释义 = %+v", entries[0].Web)
	}
}

// A Chinese headword has no ec block: the 汉英 side lives in ce/ce_new/
// wuguanghua and the pinyin in simple. Before these were read, 你好 came back
// with three example sentences and no meaning at all.
const youdaoChineseFixture = `{
  "ce": {"word": {"phone": "nǐ hǎo", "return-phrase": "你好", "trs": [
    {"voice": "hello&type=2", "#text": "hello", "#tran": "喂，你好"},
    {"voice": "hi&type=2", "#text": "hi", "#tran": "嗨"}
  ]}},
  "ce_new": {"word": [{"phone": "nǐ hǎo", "trs": [
    {"tr": [{"l": {"i": ["how do you do; how are you; hello"]}}]}
  ]}]},
  "wuguanghua": {"dataList": [{"word": "你好", "trs": [
    {"tr": {"en": "how do you do; hello"}}
  ]}]},
  "simple": {"query": "你好", "word": [{"phone": "nǐ hǎo", "return-phrase": "你好"}]},
  "blng_sents_part": {"sentence-pair": [
    {"sentence": "你好，很高兴见到你。", "sentence-translation": "Hello, nice to meet you."}
  ]}
}`

func TestYoudaoReadsChineseEntries(t *testing.T) {
	p := newYoudaoServer(t, http.StatusOK, youdaoChineseFixture, nil)

	entries, err := p.Explain(context.Background(), "你好", "zh-CN", "en")
	if err != nil {
		t.Fatalf("Explain 失败: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("条目 = %+v, want 1", entries)
	}
	e := entries[0]
	if len(e.Phonetics) != 1 || e.Phonetics[0].Lang != "拼音" || e.Phonetics[0].Symbol != "[nǐ hǎo]" {
		t.Errorf("拼音 = %+v, want 拼音 [nǐ hǎo]", e.Phonetics)
	}
	if len(e.Definitions) != 1 || e.Definitions[0].Pos != "英" {
		t.Fatalf("释义 = %+v, want 一组汉英词", e.Definitions)
	}
	if got := strings.Join(e.Definitions[0].Meanings, "|"); got != "hello|hi|how do you do|how are you" {
		t.Errorf("汉英 = %q, want 去重保序", got)
	}
	if len(e.Examples) != 1 {
		t.Errorf("例句 = %d 条, want 1", len(e.Examples))
	}
}

func TestYoudaoToleratesMissingKeysAndWrongShape(t *testing.T) {
	bodies := []string{
		`{}`,
		`{"ec":null}`,
		`{"ec":{"word":[]}}`,
		`{"ec":{"word":{"trs":"not-an-object"}}}`,
		`{"web_trans":{"web-translation":{"key":"x","trans":{"value":"y"}}}}`,
		`{"ec":[1,2,3]}`,
	}
	for _, body := range bodies {
		p := newYoudaoServer(t, http.StatusOK, body, nil)
		if _, err := p.Explain(context.Background(), "apple", "en", "zh-CN"); err == nil && !strings.Contains(body, "web-translation") {
			t.Errorf("响应 %s 应报错而不是静默返回", body)
		}
	}
}

func TestYoudaoHTTPError(t *testing.T) {
	p := newYoudaoServer(t, http.StatusForbidden, `<html>blocked</html>`, nil)
	_, err := p.Explain(context.Background(), "apple", "en", "zh-CN")
	if err == nil || !strings.Contains(errText(err), "403") {
		t.Fatalf("错误 = %v, 期望包含状态码 403", err)
	}
}

func newYoudaoProvider(t *testing.T) Provider {
	t.Helper()
	p, err := New(settings.Provider{ID: "dict-youdao", Category: settings.CatDict,
		Kind: "youdao", Label: "有道词典", Enabled: true})
	if err != nil {
		t.Fatalf("New(youdao) 失败: %v", err)
	}
	return p
}

func TestYoudaoRejectsLongQuery(t *testing.T) {
	var called bool
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	}))
	defer server.Close()

	original := youdaoDictEndpoint
	youdaoDictEndpoint = server.URL
	defer func() { youdaoDictEndpoint = original }()

	p := newYoudaoProvider(t)
	long := strings.Repeat("word ", 40)
	if _, err := p.Explain(context.Background(), long, "en", "zh"); err == nil {
		t.Fatal("超长查询应被拒绝")
	}
	if called {
		t.Error("超长查询不应发出请求")
	}
}

func TestYoudaoRegisteredAsDict(t *testing.T) {
	meta, ok := Meta("youdao")
	if !ok {
		t.Fatal("youdao 未注册")
	}
	if meta.Category != settings.CatDict {
		t.Errorf("Category = %q, 期望 %q", meta.Category, settings.CatDict)
	}
	if !meta.Free {
		t.Error("youdao 应标记为免费服务")
	}
	if meta.Multi {
		t.Error("youdao 不允许多实例")
	}
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// keep the json import honest: the fixture must stay parseable.
func TestYoudaoFixtureIsJSON(t *testing.T) {
	var check map[string]json.RawMessage
	if err := json.Unmarshal([]byte(youdaoFixture), &check); err != nil {
		t.Fatalf("测试数据本身不是合法 JSON: %v", err)
	}
}
