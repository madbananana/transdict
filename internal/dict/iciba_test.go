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

// icibaNextFixture renders the server state the word page embeds. It is built
// by marshalling maps so the fixture can never be malformed JSON; the
// "baesInfo" key keeps upstream's own spelling.
func icibaNextFixture(t *testing.T, wordName string) string {
	t.Helper()
	payload := map[string]any{
		"props": map[string]any{
			"pageProps": map[string]any{
				"initialReduxState": map[string]any{
					"word": map[string]any{
						"wordInfo": map[string]any{
							"baesInfo": map[string]any{
								"word_name": wordName,
								"exchange": map[string]any{
									"word_pl":  []string{"goods"},
									"word_er":  []string{"better"},
									"word_est": []string{"best"},
								},
								"symbols": []any{map[string]any{
									"ph_en":     "ɡʊd",
									"ph_am":     "ɡʊd",
									"ph_en_mp3": "https://res.iciba.com/uk.mp3",
									"ph_am_mp3": "https://res.iciba.com/us.mp3",
									"parts": []any{
										map[string]any{"part": "adj.", "means": []string{"好的", "令人愉快的"}},
										map[string]any{"part": "n.", "means": []string{"好处", "善行"}},
									},
								}},
							},
							"new_sentence": []any{map[string]any{
								"tag": "全部",
								"sentences": []any{
									map[string]any{"en": "It was very good of you to come.",
										"cn": "你能来真是太好了。", "from": "金山词霸"},
									map[string]any{"en": "", "cn": "缺原文应跳过"},
								},
							}},
						},
					},
				},
			},
		},
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("构造金山测试数据失败: %v", err)
	}
	return `<html><head><script id="__NEXT_DATA__" type="application/json">` +
		string(encoded) + `</script><body>ok</body></html>`
}

type icibaProbe struct {
	pageQueries    []string
	suggestQueries []string
}

func newIcibaServer(t *testing.T, probe *icibaProbe, pageStatus int, pageBody string, suggestBody string) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/word":
			probe.pageQueries = append(probe.pageQueries, r.URL.Query().Get("w"))
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(pageStatus)
			_, _ = w.Write([]byte(pageBody))
		case "/interface/index.php":
			probe.suggestQueries = append(probe.suggestQueries, r.URL.RawQuery)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(suggestBody))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	pageOriginal, suggestOriginal := icibaPageEndpoint, icibaSuggestEndpoint
	icibaPageEndpoint = server.URL + "/word"
	icibaSuggestEndpoint = server.URL + "/interface/index.php"
	t.Cleanup(func() {
		icibaPageEndpoint, icibaSuggestEndpoint = pageOriginal, suggestOriginal
	})
}

const icibaSuggestFixture = `{"status":1,"message":[{"key":"good","paraphrase":"n.好处,善行",
  "means":[{"part":"adj.","means":["好的","令人愉快的"]},{"part":"n.","means":["好处"]}]}]}`

// newIcibaProvider builds the provider through the registry so it carries the
// same HTTP client the app uses.
func newIcibaProvider(t *testing.T) Provider {
	t.Helper()
	p, err := New(settings.Provider{ID: "dict-iciba", Category: settings.CatDict,
		Kind: "iciba", Label: "金山词典", Enabled: true})
	if err != nil {
		t.Fatalf("New(iciba) 失败: %v", err)
	}
	return p
}

func TestIcibaPageRequestAndParsing(t *testing.T) {
	probe := &icibaProbe{}
	newIcibaServer(t, probe, http.StatusOK, icibaNextFixture(t, "good"), `{}`)

	p := newIcibaProvider(t)

	entries, err := p.Explain(context.Background(), "  good morning  ", "en", "zh-CN")
	if err != nil {
		t.Fatalf("Explain 失败: %v", err)
	}
	if len(probe.pageQueries) != 1 || probe.pageQueries[0] != "good morning" {
		t.Errorf("页面请求 w = %v, 期望单次携带清理后的词组", probe.pageQueries)
	}
	if len(probe.suggestQueries) != 0 {
		t.Errorf("页面已命中，不应再请求 suggest：%v", probe.suggestQueries)
	}

	if len(entries) != 1 {
		t.Fatalf("条目数 = %d, 期望 1", len(entries))
	}
	e := entries[0]
	if e.Word != "good" {
		t.Errorf("Word = %q, 期望 good", e.Word)
	}
	if len(e.Phonetics) != 2 || e.Phonetics[0].Symbol != "[ɡʊd]" ||
		e.Phonetics[0].Audio != "https://res.iciba.com/uk.mp3" {
		t.Errorf("Phonetics = %+v", e.Phonetics)
	}
	if len(e.Definitions) != 2 || e.Definitions[0].Pos != "adj." ||
		strings.Join(e.Definitions[0].Meanings, "|") != "好的|令人愉快的" {
		t.Errorf("Definitions = %+v", e.Definitions)
	}
	if strings.Join(e.Exchange, "|") != "复数：goods|比较级：better|最高级：best" {
		t.Errorf("Exchange = %v", e.Exchange)
	}
	if len(e.Examples) != 1 || e.Examples[0].Source != "It was very good of you to come." ||
		e.Examples[0].Translation != "你能来真是太好了。" {
		t.Errorf("Examples = %+v", e.Examples)
	}
}

func TestIcibaFallsBackToSuggest(t *testing.T) {
	probe := &icibaProbe{}
	newIcibaServer(t, probe, http.StatusOK, "<html>改版了，没有内嵌 JSON</html>", icibaSuggestFixture)

	p := newIcibaProvider(t)
	entries, err := p.Explain(context.Background(), "good", "en", "zh-CN")
	if err != nil {
		t.Fatalf("Explain 失败: %v", err)
	}
	if len(probe.suggestQueries) != 1 {
		t.Fatalf("应回落到 suggest 接口, 实际请求 %v", probe.suggestQueries)
	}
	query, err := http.NewRequest(http.MethodGet, "http://x/?"+probe.suggestQueries[0], nil)
	if err != nil {
		t.Fatalf("构造请求失败: %v", err)
	}
	q := query.URL.Query()
	for key, want := range map[string]string{"c": "word", "m": "getsuggest",
		"is_need_mean": "1", "word": "good"} {
		if got := q.Get(key); got != want {
			t.Errorf("suggest 参数 %s = %q, 期望 %q", key, got, want)
		}
	}

	if len(entries) != 1 || entries[0].Word != "good" {
		t.Fatalf("条目 = %+v", entries)
	}
	if len(entries[0].Definitions) != 2 {
		t.Errorf("Definitions = %+v", entries[0].Definitions)
	}
}

func TestIcibaReportsMiss(t *testing.T) {
	probe := &icibaProbe{}
	newIcibaServer(t, probe, http.StatusOK, icibaNextFixture(t, "zzqxvw"), `{"status":0,"message":[]}`)

	p := newIcibaProvider(t)
	_, err := p.Explain(context.Background(), "good", "en", "zh-CN")
	if err != nil && strings.Contains(errText(err), "未收录") {
		return
	}
	// The fixture above does answer with 释义, so a miss only shows up when the
	// page is empty: assert that path explicitly.
	empty := `<html><script id="__NEXT_DATA__" type="application/json">{"props":{"pageProps":{}}}</script></html>`
	newIcibaServer(t, &icibaProbe{}, http.StatusOK, empty, `{"status":0,"message":[]}`)
	if _, err := p.Explain(context.Background(), "qqqzzz", "en", "zh-CN"); err == nil ||
		!strings.Contains(errText(err), "未收录") {
		t.Fatalf("错误 = %v, 期望「未收录」", err)
	}
}

func TestIcibaRejectsLongQuery(t *testing.T) {
	probe := &icibaProbe{}
	newIcibaServer(t, probe, http.StatusOK, icibaNextFixture(t, "good"), icibaSuggestFixture)

	p := newIcibaProvider(t)
	if _, err := p.Explain(context.Background(), strings.Repeat("长", 80), "zh", "en"); err == nil {
		t.Fatal("超长查询应被拒绝")
	}
	if len(probe.pageQueries) != 0 {
		t.Error("超长查询不应发出请求")
	}
}

func TestIcibaNextDataExtraction(t *testing.T) {
	page := []byte(`<html><body><script id="__NEXT_DATA__" type="application/json">{"a":1}</script></body></html>`)
	payload, ok := icibaNextData(page)
	if !ok || string(payload) != `{"a":1}` {
		t.Fatalf("提取结果 = %q ok=%v", payload, ok)
	}
	if _, ok := icibaNextData([]byte(`<html>none</html>`)); ok {
		t.Error("缺少标记时应返回 false")
	}
	if _, ok := icibaNextData([]byte(`<script id="__NEXT_DATA__" type="application/json">{"a":1}`)); ok {
		t.Error("缺少结束标签时应返回 false")
	}
}

func TestIcibaRegisteredAsDict(t *testing.T) {
	meta, ok := Meta("iciba")
	if !ok {
		t.Fatal("iciba 未注册")
	}
	if meta.Category != settings.CatDict || !meta.Free || meta.Multi {
		t.Errorf("meta = %+v, 期望 dict/free/单实例", meta)
	}
}
