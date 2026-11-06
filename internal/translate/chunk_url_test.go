package translate

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
)

// A paragraph longer than the query string a vendor accepts used to be sent as
// one GET and came back 400/414. The splitter has to keep every rune and keep
// each piece's encoded size inside the budget.
func TestTrSplitURLKeepsEveryRuneAndBoundsTheQuery(t *testing.T) {
	text := "第一行标题\n" + strings.Repeat("敏捷的棕色狐狸跳过了懒惰的狗。", 400) + "\n最后一行"
	chunks := trSplitURL(text, 900)
	if len(chunks) < 2 {
		t.Fatalf("分成 %d 块, 期望多块", len(chunks))
	}
	var merged strings.Builder
	for i, c := range chunks {
		if c.text == "" {
			t.Errorf("第 %d 块为空", i)
		}
		size := 0
		for _, r := range c.text {
			size += trURLBytes(r)
		}
		// A single rune is allowed to exceed the budget; nothing can be split
		// smaller. Anything else has to fit.
		if size > 900 && len([]rune(c.text)) > 1 {
			t.Errorf("第 %d 块编码后 %d 字节, 超出预算 900", i, size)
		}
		merged.WriteString(c.text)
	}
	if merged.String() != strings.ReplaceAll(text, "\n", "") {
		t.Error("分块丢字")
	}
	if !chunks[0].lineEnd {
		t.Error("第一行没有标记为换行结束")
	}
	last := chunks[len(chunks)-1]
	if last.text != "最后一行" || last.lineEnd {
		t.Errorf("最后一块 = %q lineEnd=%v", last.text, last.lineEnd)
	}
}

func TestTrFanoutURLJoinsAlongTheOriginalLines(t *testing.T) {
	echo := func(chunk string) (string, string, error) {
		return strings.ToUpper(chunk), "en", nil
	}

	out, detected, err := trFanoutURL(context.Background(), "aa\nbb\ncc", 2, "-", echo)
	if err != nil {
		t.Fatalf("分段失败: %v", err)
	}
	if out != "AA\nBB\nCC" || detected != "en" {
		t.Errorf("out = %q detected = %q, 期望按行连接", out, detected)
	}

	// A hard split happens inside a line, so the separator is the caller's job.
	out, _, err = trFanoutURL(context.Background(), "abcd", 2, "-", echo)
	if err != nil {
		t.Fatalf("硬切失败: %v", err)
	}
	if out != "AB-CD" {
		t.Errorf("同一行被切开时 out = %q, 期望 AB-CD", out)
	}
}

func TestGoogleSplitsALongParagraphToStayInsideTheURL(t *testing.T) {
	var mu sync.Mutex
	var queries []int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		mu.Lock()
		queries = append(queries, len(r.URL.RawQuery))
		mu.Unlock()
		body, _ := json.Marshal([]any{[]any{q, "en"}})
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	g := &google{httpc: srv.Client(), clients5URL: srv.URL}
	text := strings.Repeat("敏捷的棕色狐狸跳过了懒惰的狗。", 500)
	out, detected, err := g.Translate(context.Background(), text, "auto", "zh-CN")
	if err != nil {
		t.Fatalf("Translate 失败: %v", err)
	}
	if out != text {
		t.Errorf("长文本译文被改动: len=%d want %d", len([]rune(out)), len([]rune(text)))
	}
	if detected != "en" {
		t.Errorf("识别语言 = %q, want en", detected)
	}
	if len(queries) < 2 {
		t.Fatalf("只发了 %d 个请求, 长文本应分块", len(queries))
	}
	for i, n := range queries {
		if n > trURLBudget+512 {
			t.Errorf("第 %d 个请求查询串 %d 字节, 超出分块预算", i, n)
		}
	}
}

func TestYoudaoDictionarySplitsLongText(t *testing.T) {
	var mu sync.Mutex
	var queries []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		mu.Lock()
		queries = append(queries, q)
		mu.Unlock()
		body, _ := json.Marshal(map[string]any{
			"fanyi": map[string]string{"type": "en2zh-CHS", "tran": q},
		})
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	y := &youdao{httpc: srv.Client(), dictURL: srv.URL}
	text := strings.Repeat("敏捷的棕色狐狸跳过了懒惰的狗。", 300)
	out, detected, err := y.Translate(context.Background(), text, "en", "zh-CN")
	if err != nil {
		t.Fatalf("Translate 失败: %v", err)
	}
	if out != text {
		t.Errorf("长文本译文被改动: len=%d want %d", len([]rune(out)), len([]rune(text)))
	}
	if detected != "en" {
		t.Errorf("识别语言 = %q, want en", detected)
	}
	if len(queries) < 2 {
		t.Fatalf("只发了 %d 个请求, 长文本应分块", len(queries))
	}
	for i, q := range queries {
		if n := len(url.QueryEscape(q)); n > youdaoURLBudget {
			t.Errorf("第 %d 块编码后 %d 字节, 超出有道预算", i, n)
		}
	}
}

func TestYoudaoDemoSplitsLongText(t *testing.T) {
	var mu sync.Mutex
	var queries []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		mu.Lock()
		queries = append(queries, q)
		mu.Unlock()
		body, _ := json.Marshal(map[string]any{
			"translation": []string{q},
			"l":           "zh-CHS2en",
			"errorCode":   "0",
		})
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	y := &youdao{httpc: srv.Client(), demoURL: srv.URL}
	text := strings.Repeat("你好世界。", 400)
	out, _, err := y.Translate(context.Background(), text, "zh-CN", "en")
	if err != nil {
		t.Fatalf("Translate 失败: %v", err)
	}
	if len(queries) < 2 {
		t.Fatalf("只发了 %d 个请求, 长文本应分块", len(queries))
	}
	for i, q := range queries {
		if n := len(url.QueryEscape(q)); n > youdaoURLBudget {
			t.Errorf("第 %d 块编码后 %d 字节, 超出有道预算", i, n)
		}
	}
	if got := strings.ReplaceAll(out, " ", ""); got != text {
		t.Errorf("分块拼接后内容不符: len=%d want %d", len([]rune(got)), len([]rune(text)))
	}
}

// The dictionary host's engine stops reading Latin input after roughly 500
// characters without saying so, so an English paragraph has to be split shorter
// than the URL alone would demand.
func TestYoudaoDictionaryKeepsLatinParagraphWhole(t *testing.T) {
	var mu sync.Mutex
	var queries []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		mu.Lock()
		queries = append(queries, q)
		mu.Unlock()
		body, _ := json.Marshal(map[string]any{
			"fanyi": map[string]string{"type": "en2zh-CHS", "tran": q},
		})
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	y := &youdao{httpc: srv.Client(), dictURL: srv.URL}
	var sb strings.Builder
	for i := 1; i <= 20; i++ {
		fmt.Fprintf(&sb, "Sentence number %d talks about item %d. ", i, i)
	}
	text := strings.TrimSpace(sb.String())

	out, _, err := y.Translate(context.Background(), text, "en", "zh-CN")
	if err != nil {
		t.Fatalf("Translate 失败: %v", err)
	}
	if out != text {
		t.Errorf("长英文译文被改动: len=%d want %d", len([]rune(out)), len([]rune(text)))
	}
	if len(queries) < 2 {
		t.Fatalf("只发了 %d 个请求, 英文长段应分段", len(queries))
	}
	for i, q := range queries {
		if n := len([]rune(q)); n > youdaoLatinChunk+5 {
			t.Errorf("第 %d 段 %d 字符, 超出 %d 字符上限", i, n, youdaoLatinChunk)
		}
	}
}
