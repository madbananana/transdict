package translate

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"transdict/internal/settings"
)

// openAIFor builds a provider against a test server; extra is the service row's
// saved settings beyond the endpoint and key.
func openAIFor(url string, extra map[string]string) *openAI {
	return newOpenAI(settings.Provider{
		BaseURL: url, APIKey: "key", Model: "gpt-4o-mini", Extra: extra,
	})
}

// The 思考程度 picker must reach the wire as reasoning_effort, and stay absent
// when it is off, because plain chat models reject an unknown field.
func TestOpenAIForwardsReasoningEffort(t *testing.T) {
	tests := []struct {
		name   string
		effort string
		want   string
	}{
		{name: "关闭时不带字段", effort: "", want: ""},
		{name: "高", effort: "high", want: "high"},
		{name: "最大", effort: "max", want: "max"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got map[string]any
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				if err := json.Unmarshal(body, &got); err != nil {
					t.Errorf("请求体不是 JSON: %v", err)
				}
				w.Header().Set("Content-Type", "application/json")
				io.WriteString(w, `{"choices":[{"message":{"content":"你好"}}]}`)
			}))
			defer srv.Close()

			p := openAIFor(srv.URL, map[string]string{settings.FieldReasoningEffort: tc.effort})
			out, _, err := p.Translate(context.Background(), "hello", "en", "zh-CN")
			if err != nil {
				t.Fatalf("Translate: %v", err)
			}
			if out != "你好" {
				t.Errorf("译文 = %q", out)
			}
			effort, ok := got["reasoning_effort"]
			if tc.want == "" {
				if ok {
					t.Errorf("关闭时不应发送 reasoning_effort, 实际 %v", effort)
				}
				return
			}
			if effort != tc.want {
				t.Errorf("reasoning_effort = %v, 期望 %q", effort, tc.want)
			}
		})
	}
}

// 隐藏 <think> 标签内容 must drop the inline scratchpad, closed or truncated,
// and must leave a normal answer untouched.
func TestOpenAIHidesThinkBlock(t *testing.T) {
	tests := []struct {
		name    string
		hide    bool
		content string
		want    string
	}{
		{name: "关闭时保留", hide: false, content: "<think>想一下</think>你好", want: "<think>想一下</think>你好"},
		{name: "开启时去掉", hide: true, content: "<think>想一下</think>你好", want: "你好"},
		{name: "多行与属性", hide: true, content: "<think type=\"x\">\n第一行\n第二行\n</think>\n你好", want: "你好"},
		{name: "截断未闭合", hide: true, content: "<think>想一半", want: ""},
		{name: "普通答案不受影响", hide: true, content: "你好", want: "你好"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				io.WriteString(w, `{"choices":[{"message":{"content":`+quoteJSON(tc.content)+`}}]}`)
			}))
			defer srv.Close()

			extra := map[string]string{}
			if tc.hide {
				extra[settings.FieldHideThink] = "1"
			}
			p := openAIFor(srv.URL, extra)
			out, _, err := p.Translate(context.Background(), "hello", "en", "zh-CN")
			if err != nil {
				t.Fatalf("Translate: %v", err)
			}
			if out != tc.want {
				t.Errorf("译文 = %q, 期望 %q", out, tc.want)
			}
		})
	}
}

// A streamed reply reaches the card delta by delta; the [DONE] line ends the
// read without being mistaken for content.
func TestOpenAIStreamsDeltas(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &got)
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"你\"}}]}\n\n")
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"好\"}}]}\n\n")
		io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	p := openAIFor(srv.URL, nil)
	var partials []string
	out, _, err := p.TranslateStream(context.Background(), "hello", "en", "zh-CN", func(s string) {
		partials = append(partials, s)
	})
	if err != nil {
		t.Fatalf("TranslateStream: %v", err)
	}
	if out != "你好" {
		t.Errorf("译文 = %q, want 你好", out)
	}
	if len(partials) < 2 || partials[0] != "你" || partials[len(partials)-1] != "你好" {
		t.Errorf("增量 = %v, want 先 你 再 你好", partials)
	}
	if got["stream"] != true {
		t.Errorf("stream = %v, want true", got["stream"])
	}
}

// A gateway that ignores stream:true answers whole JSON; the card still gets the
// finished text instead of waiting for events that never come.
func TestOpenAIStreamFallsBackToWholeJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"choices":[{"message":{"content":"你好"}}]}`)
	}))
	defer srv.Close()

	p := openAIFor(srv.URL, nil)
	var partials []string
	out, _, err := p.TranslateStream(context.Background(), "hello", "en", "zh-CN", func(s string) {
		partials = append(partials, s)
	})
	if err != nil {
		t.Fatalf("TranslateStream: %v", err)
	}
	if out != "你好" || len(partials) != 1 || partials[0] != "你好" {
		t.Errorf("译文 = %q, 增量 = %v, want 一次完整的 你好", out, partials)
	}
}

// A wrong address usually answers with an HTML page. Naming that beats echoing
// the markup into the card, and it is the explanation EasyDict gives.
func TestOpenAIExplainsAnHTMLResponse(t *testing.T) {
	for _, tc := range []struct{ name, ctype string }{
		{"content-type", "text/html; charset=utf-8"},
		{"body-only", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.ctype != "" {
					w.Header().Set("Content-Type", tc.ctype)
				}
				io.WriteString(w, "<!doctype html><title>Not Found</title>")
			}))
			defer srv.Close()

			p := openAIFor(srv.URL, nil)
			_, _, err := p.TranslateStream(context.Background(), "hello", "en", "zh-CN", nil)
			if err == nil {
				t.Fatal("HTML 页面应报错")
			}
			msg := err.Error()
			if !strings.Contains(msg, "端点返回了 HTML 页面") || !strings.Contains(msg, "API 路径") {
				t.Errorf("错误 = %q, want HTML 页面说明", msg)
			}
			if strings.Contains(msg, "<!doctype") {
				t.Errorf("错误 %q 泄漏了页面原文", msg)
			}
		})
	}
}

// A plain-text body is not the JSON the endpoint promised either.
func TestOpenAIExplainsANonJSONResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		io.WriteString(w, "gateway says no")
	}))
	defer srv.Close()

	p := openAIFor(srv.URL, nil)
	_, _, err := p.Translate(context.Background(), "hello", "en", "zh-CN")
	if err == nil {
		t.Fatal("非 JSON 响应应报错")
	}
	if msg := err.Error(); !strings.Contains(msg, "响应格式不符合预期") || !strings.Contains(msg, "API 文档") {
		t.Errorf("错误 = %q, want 响应格式说明", msg)
	}
}

// 隐藏 <think> 标签内容 applies while streaming too, so the tag never flashes in
// the card before the block closes.
func TestOpenAIStreamHidesThinkDeltas(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"<think>想</think>\"}}]}\n\n")
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"你好\"}}]}\n\n")
		io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	p := openAIFor(srv.URL, map[string]string{settings.FieldHideThink: "1"})
	var partials []string
	out, _, err := p.TranslateStream(context.Background(), "hello", "en", "zh-CN", func(s string) {
		partials = append(partials, s)
	})
	if err != nil {
		t.Fatalf("TranslateStream: %v", err)
	}
	if out != "你好" {
		t.Errorf("译文 = %q, want 你好", out)
	}
	for _, s := range partials {
		if strings.Contains(s, "<think") || strings.Contains(s, "想") {
			t.Errorf("增量 %q 泄漏了 think 内容", s)
		}
	}
}

// 使用自定义 Prompt sends the saved system/user pair instead of a built-in
// instruction, and expands EasyDict's four variables.
func TestOpenAICustomPromptSubstitutesVariables(t *testing.T) {
	var messages []map[string]string
	srv := captureMessages(t, &messages)
	defer srv.Close()

	p := openAIFor(srv.URL, map[string]string{
		settings.FieldEnableCustomPrompt: "1",
		settings.FieldSystemPrompt:       "translate into ${{queryTargetLanguage}}",
		settings.FieldUserPrompt:         "text=${{queryText}};from=${{queryFromLanguage}};first=${{firstLanguage}}",
	})
	p.FirstLang = "zh-CN"
	if _, _, err := p.Translate(context.Background(), "hello", "en", "ja"); err != nil {
		t.Fatalf("Translate: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("messages = %v, want system + user", messages)
	}
	if got := messages[0]["content"]; got != "translate into 日语" {
		t.Errorf("system = %q", got)
	}
	if got := messages[1]["content"]; got != "text=hello;from=英语;first=简体中文" {
		t.Errorf("user = %q", got)
	}
}

// An empty user prompt falls back to the query text and an empty system prompt
// sends no system message, matching EasyDict's custom-prompt path.
func TestOpenAICustomPromptFallsBackAndOmits(t *testing.T) {
	cases := []struct {
		name     string
		system   string
		user     string
		wantRole []string
		wantUser string
	}{
		{name: "只有系统", system: "只按系统提示", wantRole: []string{"system", "user"}, wantUser: "hello"},
		{name: "只有用户", user: "${{queryText}}!", wantRole: []string{"user"}, wantUser: "hello!"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var messages []map[string]string
			srv := captureMessages(t, &messages)
			defer srv.Close()

			p := openAIFor(srv.URL, map[string]string{
				settings.FieldEnableCustomPrompt: "1",
				settings.FieldSystemPrompt:       tc.system,
				settings.FieldUserPrompt:         tc.user,
			})
			if _, _, err := p.Translate(context.Background(), "hello", "en", "zh-CN"); err != nil {
				t.Fatalf("Translate: %v", err)
			}
			if len(messages) != len(tc.wantRole) {
				t.Fatalf("messages = %v, want %d", messages, len(tc.wantRole))
			}
			for i, role := range tc.wantRole {
				if messages[i]["role"] != role {
					t.Errorf("messages[%d].role = %q, want %q", i, messages[i]["role"], role)
				}
			}
			if got := messages[len(messages)-1]["content"]; got != tc.wantUser {
				t.Errorf("user = %q, want %q", got, tc.wantUser)
			}
		})
	}
}

// The task switches choose the built-in instruction, as EasyDict does for
// 查单词/句子分析/翻译.
func TestOpenAIPicksTheTaskPrompt(t *testing.T) {
	cases := []struct {
		name string
		text string
		src  string
		want func(*openAI) string
	}{
		{name: "英文单词", text: "hello", src: "en", want: func(p *openAI) string { return p.dictionaryInstruction("zh-CN") }},
		{name: "中文词语", text: "你好", src: "zh-CN", want: func(p *openAI) string { return p.dictionaryInstruction("zh-CN") }},
		{name: "英文句子", text: "The quick brown fox jumps.", src: "en", want: func(p *openAI) string { return p.sentenceInstruction("zh-CN") }},
		{name: "中文段落", text: "这个软件的界面比原来的好看多了。", src: "zh-CN", want: func(p *openAI) string { return p.instruction("zh-CN") }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var messages []map[string]string
			srv := captureMessages(t, &messages)
			defer srv.Close()

			p := openAIFor(srv.URL, nil)
			if _, _, err := p.Translate(context.Background(), tc.text, tc.src, "zh-CN"); err != nil {
				t.Fatalf("Translate: %v", err)
			}
			if got := messages[0]["content"]; got != tc.want(p) {
				t.Errorf("system = %q, want %q", got, tc.want(p))
			}
		})
	}
}

// A task turned off must not answer that kind of query; with every task off the
// service stays out of the query entirely.
func TestOpenAITaskSwitchesBoundTheService(t *testing.T) {
	all := openAIFor("http://example.invalid", nil)
	if !all.SupportsQuery("hello", "en", "zh-CN") {
		t.Error("默认应回答单词查询")
	}

	wordOnly := openAIFor("http://example.invalid", map[string]string{
		settings.FieldTranslation: "0", settings.FieldSentence: "0",
	})
	if !wordOnly.SupportsQuery("hello", "en", "zh-CN") {
		t.Error("查单词开启时应回答单词")
	}
	if wordOnly.SupportsQuery("hello world", "en", "zh-CN") {
		t.Error("翻译与句子分析关闭时不应回答句子")
	}

	none := openAIFor("http://example.invalid", map[string]string{
		settings.FieldTranslation: "0", settings.FieldSentence: "0", settings.FieldDictionary: "0",
	})
	if none.SupportsQuery("hello", "en", "zh-CN") {
		t.Error("三项都关闭时不应回答任何查询")
	}
}

// An ad-hoc instruction (润色) wins over the saved custom prompt for that call,
// just as it wins over 附加指令.
func TestOpenAIAdHocPromptBeatsCustomPrompt(t *testing.T) {
	var messages []map[string]string
	srv := captureMessages(t, &messages)
	defer srv.Close()

	p := openAIFor(srv.URL, map[string]string{
		settings.FieldEnableCustomPrompt: "1",
		settings.FieldSystemPrompt:       "自定义系统提示",
	})
	instructed := p.WithPrompt("润色成更地道的英文").(*openAI)
	if _, _, err := instructed.Translate(context.Background(), "hello world", "en", "zh-CN"); err != nil {
		t.Fatalf("Translate: %v", err)
	}
	system := messages[0]["content"]
	if !strings.Contains(system, "润色成更地道的英文") || strings.Contains(system, "自定义系统提示") {
		t.Errorf("system = %q, want 润色指令且不含自定义提示", system)
	}
}

// captureMessages decodes each request's chat messages so a test can assert what
// reached the wire.
func captureMessages(t *testing.T, into *[]map[string]string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []map[string]string `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("请求体不是 JSON: %v", err)
		}
		*into = body.Messages
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"choices":[{"message":{"content":"ok"}}]}`)
	}))
}

func quoteJSON(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// 启用流式传输 defaults on so an existing row keeps its streamed card, and an
// explicit 关闭 is what turns the pipeline onto the one-shot path.
func TestStreamSwitchDefaultsOn(t *testing.T) {
	if !newOpenAI(settings.Provider{}).StreamingEnabled() {
		t.Error("默认应为开启流式传输")
	}
	off := newOpenAI(settings.Provider{Extra: map[string]string{settings.FieldStream: "0"}})
	if off.StreamingEnabled() {
		t.Error("关闭后仍按流式发送")
	}
}

// 从云端获取模型 reads the saved endpoint's own catalog, with the saved key.
func TestListModelsReadsTheCatalog(t *testing.T) {
	var gotAuth, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotPath = r.Header.Get("Authorization"), r.URL.Path
		io.WriteString(w, `{"data":[{"id":"gpt-4o-mini"},{"id":"gpt-4o"}]}`)
	}))
	defer srv.Close()

	models, err := openAIFor(srv.URL, nil).ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if gotPath != "/models" || gotAuth != "Bearer key" {
		t.Errorf("请求 path=%q auth=%q", gotPath, gotAuth)
	}
	if len(models) != 2 || models[0] != "gpt-4o-mini" || models[1] != "gpt-4o" {
		t.Errorf("models = %v", models)
	}
}

func TestListModelsReportsHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusUnauthorized)
	}))
	defer srv.Close()
	if _, err := openAIFor(srv.URL, nil).ListModels(context.Background()); err == nil {
		t.Error("401 没有报错")
	}
}

// EasyDict's OpenAI field asks for a complete .../chat/completions URL; a base
// like .../v1 is what this app used to save. Both must reach the endpoint once,
// and the catalog must sit beside it either way.
func TestOpenAIAcceptsAFullChatAddress(t *testing.T) {
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			io.WriteString(w, `{"data":[{"id":"gpt-4o-mini"}]}`)
			return
		}
		io.WriteString(w, `{"choices":[{"message":{"content":"你好"}}]}`)
	}))
	defer srv.Close()

	p, err := New(settings.Provider{
		ID: "tr-openai", Kind: "openai", BaseURL: srv.URL + "/v1/chat/completions",
		Model: "gpt-4o-mini", APIKey: "k",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	out, _, err := p.Translate(context.Background(), "hello", "en", "zh-CN")
	if err != nil || out != "你好" {
		t.Fatalf("译文 = %q, err = %v", out, err)
	}
	models := p.(ModelLister)
	if _, err := models.ListModels(context.Background()); err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	want := []string{"POST /v1/chat/completions", "GET /v1/models"}
	if strings.Join(paths, ",") != strings.Join(want, ",") {
		t.Errorf("请求 = %v, want %v", paths, want)
	}
}
