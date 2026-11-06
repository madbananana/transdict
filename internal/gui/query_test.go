package gui

import (
	"strings"
	"testing"

	"transdict/internal/dict"
	"transdict/internal/query"
	"transdict/internal/settings"
	"transdict/internal/translate"
)

func TestTargetRoutesByContext(t *testing.T) {
	cases := map[string]string{
		settings.ContextMain:       WindowMain,
		settings.ContextScreenshot: WindowMain,
		settings.ContextClipboard:  WindowMain,
		settings.ContextFloating:   WindowFloat,
		settings.ContextODict:      WindowFloat,
		settings.ContextMini:       WindowMini,
		"":                         WindowMain,
	}
	for context, want := range cases {
		if got := target(context); got != want {
			t.Errorf("target(%q) = %q, 期望 %q", context, got, want)
		}
	}
}

func TestContextOfTreatsMissingResultAsMain(t *testing.T) {
	if got := contextOf(nil); got != settings.ContextMain {
		t.Errorf("contextOf(nil) = %q", got)
	}
	if got := contextOf(&query.Response{Context: settings.ContextMini}); got != settings.ContextMini {
		t.Errorf("contextOf = %q", got)
	}
}

func TestSameTextIgnoresWhitespace(t *testing.T) {
	if !sameText("  hello   world ", "hello world") {
		t.Error("换行与多余空格不应算作差异")
	}
	if sameText("hello world", "hello there") {
		t.Error("不同译文被判为相同")
	}
}

func TestDropUnchangedKeepsFailures(t *testing.T) {
	items := []translate.Result{
		{Label: "echo", Text: "  hello   world "},
		{Label: "real", Text: "Bonjour le monde"},
		{Label: "broken", Text: "hello world", Error: "超时"},
	}
	got := dropUnchanged(items, "hello world")
	if len(got) != 2 {
		t.Fatalf("保留 %d 条, 期望 2: %+v", len(got), got)
	}
	// A service that failed is not evidence that the text is already in the
	// target language, so it stays and still shows its error.
	if got[0].Label != "real" || got[1].Label != "broken" {
		t.Errorf("保留顺序错误: %v, %v", got[0].Label, got[1].Label)
	}
}

func result(labels ...string) []translate.Result {
	out := make([]translate.Result, 0, len(labels))
	for _, l := range labels {
		if strings.HasPrefix(l, "!") {
			out = append(out, translate.Result{Label: l[1:], Error: "失败"})
			continue
		}
		out = append(out, translate.Result{Label: l, Text: l})
	}
	return out
}

func TestAnalyzeNothingRanIsExplained(t *testing.T) {
	cfg := settings.Config{Translate: []settings.Provider{
		{ID: "ai", Category: settings.CatTranslate, Kind: "openai", Label: "AI 模型", Enabled: true},
	}}
	res := &query.Response{SourceText: "hello"}
	(&Service{}).annotate(res, cfg, query.Request{Context: settings.ContextMain})

	if res.Error != "没有可用的服务" {
		t.Fatalf("没有服务运行时必须给出原因，得到 %q", res.Error)
	}
	// The names travel apart from the sentence: Go words the service labels and
	// the window words the sentence around them, in whatever language it draws.
	if len(res.Skipped) != 1 || res.Skipped[0] != "AI 模型" {
		t.Errorf("Skipped = %v, want [AI 模型]", res.Skipped)
	}
}

// A screenshot with no configured engine is the Windows first-run case, and
// 未识别到文字 would blame the picture for work the app was never able to do.
func TestAnalyzeScreenshotNamesTheMissingEngine(t *testing.T) {
	cfg := settings.Config{OCR: []settings.Provider{
		{ID: "ocr-baidu", Category: settings.CatOCR, Kind: "baidu", Label: "百度通用文字识别", Enabled: true},
	}}
	res := &query.Response{}
	(&Service{}).annotate(res, cfg, query.Request{Context: settings.ContextScreenshot, Image: []byte{1}})

	if res.Error != "没有可用的文字识别服务" {
		t.Fatalf("得到 %q", res.Error)
	}
	if len(res.Skipped) != 1 || res.Skipped[0] != "百度通用文字识别" {
		t.Errorf("Skipped = %v, want [百度通用文字识别]", res.Skipped)
	}
}

func TestAnalyzeSilentWhenOnlyEchoesRemain(t *testing.T) {
	cfg := settings.Config{HideSameLang: true}
	res := &query.Response{SourceText: "hello world", Translate: result("hello world", " hello   world ")}
	(&Service{}).annotate(res, cfg, query.Request{Context: settings.ContextMain})

	if res.Error != "原文已是目标语言" {
		t.Errorf("全部回显时应说明原因，得到 %q", res.Error)
	}
	if len(res.Translate) != 0 {
		t.Errorf("回显应被隐藏，剩下 %+v", res.Translate)
	}
}

func TestAnalyzeKeepsDictionaryOnlyQuiet(t *testing.T) {
	cfg := settings.Config{}
	res := &query.Response{SourceText: "word", Dict: []dict.Result{{Label: "苹果词典", Entries: []dict.Entry{{Text: "释义"}}}}}
	(&Service{}).annotate(res, cfg, query.Request{Context: settings.ContextMain, DictionaryOnly: true})
	if res.Error != "" {
		t.Errorf("词典模式有结果时不该报错，得到 %q", res.Error)
	}

	// 词典模式 deliberately runs no translators, so an empty translate list is
	// not a problem to complain about.
	empty := &query.Response{SourceText: "word", Dict: []dict.Result{{Label: "苹果词典"}}}
	(&Service{}).annotate(empty, cfg, query.Request{Context: settings.ContextMain, DictionaryOnly: true})
	if empty.Error != "" {
		t.Errorf("词典模式不该提示没有翻译服务，得到 %q", empty.Error)
	}
}

func TestAnalyzeLeavesErrorsAndEmptyTextAlone(t *testing.T) {
	cfg := settings.Config{}
	withError := &query.Response{SourceText: "x", Error: "已经说过了"}
	(&Service{}).annotate(withError, cfg, query.Request{})
	if withError.Error != "已经说过了" {
		t.Errorf("已有错误不该被覆盖: %q", withError.Error)
	}

	blank := &query.Response{SourceText: ""}
	(&Service{}).annotate(blank, cfg, query.Request{})
	if blank.Error != "" {
		t.Errorf("没有原文时不该补充说明: %q", blank.Error)
	}
}

func TestStageLabelNamesTheWork(t *testing.T) {
	cases := []struct {
		req  query.Request
		want string
	}{
		{query.Request{Text: "hi"}, "查询中…"},
		{query.Request{Image: []byte{1}}, "识别文字中…"},
		{query.Request{Text: "hi", Prompt: "润色"}, "AI 处理中…"},
	}
	for _, c := range cases {
		if got := stageLabel(c.req); got != c.want {
			t.Errorf("stageLabel(%+v) = %q, 期望 %q", c.req, got, c.want)
		}
	}
}

func TestANewQueryReplacesTheOneBeforeIt(t *testing.T) {
	s := &Service{}
	firstCtx, first, _ := s.beginRun(settings.ContextMain)
	if !s.currentRun(settings.ContextMain, first) {
		t.Fatal("刚发起的查询不是当前的")
	}

	_, second, secondCancel := s.beginRun(settings.ContextMain)
	defer secondCancel()
	select {
	case <-firstCtx.Done():
	default:
		t.Error("新查询没有停掉被替代的那一个")
	}
	if s.currentRun(settings.ContextMain, first) {
		t.Error("被替代的查询仍被当作当前查询")
	}
	if !s.currentRun(settings.ContextMain, second) {
		t.Error("新查询不是当前查询")
	}

	// Another window has its own queue: replacing the panel's query must not
	// cancel what the mini window is still waiting for.
	_, mini, miniCancel := s.beginRun(settings.ContextMini)
	defer miniCancel()
	if !s.currentRun(settings.ContextMini, mini) || !s.currentRun(settings.ContextMain, second) {
		t.Error("不同窗口的查询互相影响")
	}
}

// 润色 and 总结 run on the translation stage; marking them as dictionary-only
// removes that stage and every chip answers "没有可用的服务".
func TestPromptRequestIsNotADictionaryQuery(t *testing.T) {
	req := promptRequest("hello", "润色")
	if req.DictionaryOnly {
		t.Error("润色/总结被当成了仅词典查询：翻译阶段会被整段跳过")
	}
	if req.Context != settings.ContextTranslation || req.Mode != "prompt" || req.Prompt != "润色" {
		t.Errorf("promptRequest = %+v", req)
	}
}
