package query

import (
	"context"
	"errors"
	"strings"
	"testing"

	"transdict/internal/dict"
	"transdict/internal/ocr"
	"transdict/internal/settings"
	"transdict/internal/translate"
)

// google needs no key and openai needs one, which is what makes the pair a
// stable way to say "ready" and "enabled but unusable" without inventing a
// fake provider registry inside the test.
func ready(id string) settings.Provider {
	return settings.Provider{ID: id, Category: settings.CatTranslate, Kind: "google", Label: id, Enabled: true}
}

func cfgWith(translate []settings.Provider) settings.Config {
	return settings.Config{Translate: translate}
}

func ids(providers []settings.Provider) []string {
	out := make([]string, len(providers))
	for i, p := range providers {
		out[i] = p.ID
	}
	return out
}

// No platform registers an "apple" translation, so the row stands for a service
// a config file remembers but this build cannot run.
// A kind this build does not register stands for a service the user's file
// still carries after a downgrade or a hand edit.
func unshipped(id string) settings.Provider {
	return settings.Provider{ID: id, Category: settings.CatTranslate, Kind: "ghost",
		Label: "苹果翻译", Enabled: true}
}

func needsKey(id string) settings.Provider {
	return settings.Provider{ID: id, Category: settings.CatTranslate, Kind: "openai",
		Label: "AI", Enabled: true}
}

// The names beside 没有可用的服务 tell the user what to configure, so a service no
// build implements must not appear — that is a fault nobody can fix.
func TestSkippedIgnoresServicesWeNeverShip(t *testing.T) {
	cfg := cfgWith([]settings.Provider{unshipped("tr-apple"), needsKey("ai")})
	got := Skipped(cfg, settings.ContextMain, settings.CatTranslate)
	if len(got) != 1 || got[0] != "AI" {
		t.Errorf("Skipped = %v, want [AI]", got)
	}
}

// A window that did not tick a service is not waiting for it either.
func TestSkippedNamesOnlyWhatTheContextChecked(t *testing.T) {
	cfg := cfgWith([]settings.Provider{ready("a"), needsKey("ai")})
	cfg.Selections = map[string][]string{
		settings.ContextMain:    {"a"},
		settings.ContextReplace: {"ai"},
	}
	if got := Skipped(cfg, settings.ContextMain, settings.CatTranslate); len(got) != 0 {
		t.Errorf("主窗口没有勾选 AI，Skipped = %v, want 空", got)
	}
	if got := Skipped(cfg, settings.ContextReplace, settings.CatTranslate); len(got) != 1 || got[0] != "AI" {
		t.Errorf("翻译并替换勾选了 AI，Skipped = %v, want [AI]", got)
	}
}

func TestPruneDropsWhatThisBuildCannotRun(t *testing.T) {
	cfg := cfgWith([]settings.Provider{ready("a"), unshipped("tr-apple")})
	cfg.Selections = map[string][]string{settings.ContextMain: {"tr-apple", "a"}}

	if n := Prune(&cfg); n != 1 {
		t.Fatalf("Prune = %d, want 1", n)
	}
	if got := strings.Join(ids(cfg.Translate), ","); got != "a" {
		t.Errorf("清理后服务 = %q, want a", got)
	}
	if got := strings.Join(cfg.Selections[settings.ContextMain], ","); got != "a" {
		t.Errorf("勾选列表 = %q, want a（死行不应留下勾选项）", got)
	}
	if n := Prune(&cfg); n != 0 {
		t.Errorf("再次 Prune = %d, want 0：已清理的配置不该再改写文件", n)
	}
}

// 默认服务是每台机器第一天的样子，所以它只能包含本机真的能跑的服务。
func TestDefaultShipsOnlyRegisteredKinds(t *testing.T) {
	cfg := settings.Default()
	for _, category := range []string{settings.CatOCR, settings.CatTranslate, settings.CatDict, settings.CatTTS} {
		for _, p := range cfg.List(category) {
			if _, ok := MetaOf(category, p.Kind); !ok {
				t.Errorf("默认的 %s 用了 %q：本平台没有这个实现", p.ID, p.Kind)
			}
		}
	}
}

func TestPlanRunsEnabledAndReady(t *testing.T) {
	cfg := cfgWith([]settings.Provider{
		ready("a"),
		{ID: "off", Category: settings.CatTranslate, Kind: "google", Label: "off"},
		ready("d"),
	})
	got := strings.Join(ids(plan(cfg, settings.ContextMain, settings.CatTranslate)), ",")
	if got != "a,d" {
		t.Errorf("plan = %q, 期望 a,d", got)
	}
}

func TestPlanUsesSelectionOrder(t *testing.T) {
	cfg := cfgWith([]settings.Provider{ready("a"), ready("b"), ready("c")})
	cfg.Selections = map[string][]string{settings.ContextMain: {"c", "a"}}
	got := strings.Join(ids(plan(cfg, settings.ContextMain, settings.CatTranslate)), ",")
	if got != "c,a" {
		t.Errorf("勾选顺序应决定展示顺序，得到 %q", got)
	}
}

// A checked list is a list of *shown* services, so one that names a provider
// the config does not have — renamed, deleted, or from an older build — must
// not silently shrink the run.
func TestPlanFallsBackWhenNothingIsRecognised(t *testing.T) {
	cfg := cfgWith([]settings.Provider{ready("a"), ready("b")})
	cfg.Selections = map[string][]string{settings.ContextMain: {"tr-google"}}
	got := strings.Join(ids(plan(cfg, settings.ContextMain, settings.CatTranslate)), ",")
	if got != "a,b" {
		t.Errorf("勾选全部失效时 plan = %q, 期望 a,b", got)
	}
}

func TestPlanDropsUnreadyFromSelection(t *testing.T) {
	unconfigured := settings.Provider{ID: "ai", Category: settings.CatTranslate, Kind: "openai",
		Label: "AI", Enabled: true}
	cfg := cfgWith([]settings.Provider{ready("a"), unconfigured})
	cfg.Selections = map[string][]string{settings.ContextMain: {"ai", "a"}}
	got := strings.Join(ids(plan(cfg, settings.ContextMain, settings.CatTranslate)), ",")
	if got != "a" {
		t.Errorf("缺密钥的服务不应进入 plan，得到 %q", got)
	}
	if skipped := Skipped(cfg, settings.ContextMain, settings.CatTranslate); len(skipped) != 1 || skipped[0] != "AI" {
		t.Errorf("Skipped = %v, 期望 [AI]", skipped)
	}
}

func TestWithLangsFallsBackToConfigThenDefaults(t *testing.T) {
	cfg := settings.Config{SourceLang: "en", TargetLang: "ja"}
	got := withLangs(Request{Text: "hi"}, cfg)
	if got.SourceLang != "en" || got.TargetLang != "ja" {
		t.Errorf("应继承配置语言，得到 %q→%q", got.SourceLang, got.TargetLang)
	}

	empty := withLangs(Request{}, settings.Config{})
	if empty.SourceLang != "auto" || empty.TargetLang != "zh-CN" {
		t.Errorf("配置为空时应回落到 auto→zh-CN，得到 %q→%q", empty.SourceLang, empty.TargetLang)
	}

	kept := withLangs(Request{SourceLang: "fr", TargetLang: "de"}, cfg)
	if kept.SourceLang != "fr" || kept.TargetLang != "de" {
		t.Errorf("请求自带的语言不应被覆盖，得到 %q→%q", kept.SourceLang, kept.TargetLang)
	}
}

func TestSamePairTargetFlipsChineseToEnglish(t *testing.T) {
	// Easydict translates 中文 typed into a 中文 target into 英文 instead of
	// echoing it, which is also what 有道 does while Google repeats the input.
	if got := samePairTarget(settings.Config{}, "这个软件的界面比原来的好看多了。", "auto", "zh-CN"); got != "en" {
		t.Errorf("中文→中文目标 = %q, 期望翻成 en", got)
	}
	if got := samePairTarget(settings.Config{}, "这是一句中文。", "auto", "zh-TW"); got != "en" {
		t.Errorf("中文→繁体目标 = %q, 期望翻成 en", got)
	}
	if got := samePairTarget(settings.Config{}, "The quick brown fox.", "auto", "zh-CN"); got != "" {
		t.Errorf("英文输入不应改动目标，得到 %q", got)
	}
	if got := samePairTarget(settings.Config{}, "このソフトの画面は見やすい。", "auto", "zh-CN"); got != "" {
		t.Errorf("含假名的日文不应翻成英文，得到 %q", got)
	}
	if got := samePairTarget(settings.Config{}, "这个软件很好用。", "zh-CN", "zh-CN"); got != "" {
		t.Errorf("手动指定源语言时不应改动目标，得到 %q", got)
	}
}

// 第一语言/第二语言 replace the built-in Chinese↔English pair, the way
// Easydict's 语种识别 section does: the reader's own two languages decide where
// 自动选择 lands.
func TestAutoTargetFollowsTheReadersPair(t *testing.T) {
	cfg := settings.Config{FirstLang: "ja", SecondLang: "zh-CN"}
	if got := autoTarget(cfg, "今天天气很好，我们去公园吧。"); got != "ja" {
		t.Errorf("中文 → 自动选择（首选日语）= %q, want ja", got)
	}
	if got := autoTarget(cfg, "このソフトの画面は見やすい。"); got != "zh-CN" {
		t.Errorf("日文 → 自动选择（首选日语）= %q, want zh-CN", got)
	}
	if got := samePairTarget(cfg, "今天天气很好，我们去公园吧。", "auto", "zh-CN"); got != "ja" {
		t.Errorf("中文→中文目标（首选日语）= %q, want ja", got)
	}
}

// 语种识别 optimization feeds a service's answer into the same decision: a
// language Google or Baidu named replaces what the system recogniser guessed.
func TestDetectionOptimizationOverridesTheSystemAnswer(t *testing.T) {
	cfg := settings.Config{FirstLang: "en", SecondLang: "zh-CN"}
	if got := autoTargetDetected(cfg, "en"); got != "zh-CN" {
		t.Errorf("在线识别为英文 → 自动选择 = %q, want zh-CN", got)
	}
	if got := autoTargetDetected(cfg, "de"); got != "en" {
		t.Errorf("在线识别为德文 → 自动选择 = %q, want en", got)
	}
	if got := samePairTargetDetected(cfg, "zh-CN", "auto", "zh-CN"); got != "en" {
		t.Errorf("在线识别为中文 → 中文目标应翻成英文，得到 %q", got)
	}
	if got := samePairTargetDetected(cfg, "en", "auto", "zh-CN"); got != "" {
		t.Errorf("在线识别为英文 → 中文目标不应改动，得到 %q", got)
	}
}

// 仅使用系统语种识别 must not make a request, and a chosen service that is not
// configured must not hold the query for its timeout either.
func TestDetectionOptimizationIsOptInAndSilentWithoutAService(t *testing.T) {
	e := New(nil)
	if _, ok := e.optimizeDetection(context.Background(), settings.Config{}, "hello"); ok {
		t.Error("默认模式不应发起在线识别")
	}
	google := settings.Config{DetectOptimize: settings.DetectOptimizeGoogle}
	if _, ok := e.optimizeDetection(context.Background(), google, "hello"); ok {
		t.Error("没有配置 Google 时不应发起在线识别")
	}
	baidu := settings.Config{DetectOptimize: settings.DetectOptimizeBaidu,
		Translate: []settings.Provider{{Category: settings.CatTranslate, Kind: "baidu", Enabled: true}}}
	if _, ok := e.optimizeDetection(context.Background(), baidu, "hello"); ok {
		t.Error("百度没有密钥时不应发起在线识别")
	}
}

func TestSamePairTargetIgnoresAChineseNameInEnglishText(t *testing.T) {
	// Apple OCR reads the Chinese name in this English message; that does not
	// make the message Chinese, and flipping its target to English turned the
	// query into an English→English echo.
	text := "Congratulations!\nReview of your submission has been completed. It is now " +
		"eligible for distribution.\nSubmitted: Oct 30, 2026 at 11:43 PM Pacific " +
		"Daylight Time\nSubmitted by: 全勇陈\nNumber of items submitted: 1"
	if got := samePairTarget(settings.Config{}, text, "auto", "zh-CN"); got != "" {
		t.Errorf("英文正文里的中文人名不应触发翻转，得到 %q", got)
	}
	// Chinese that quotes a couple of English words is still Chinese.
	if got := samePairTarget(settings.Config{}, "今天用 Go 写了两个小函数，感觉很好。", "auto", "zh-CN"); got != "en" {
		t.Errorf("中文里夹少量英文仍应翻成英文，得到 %q", got)
	}
}

func TestPickTextPrefersTheLongestAnswer(t *testing.T) {
	outcomes := []ocr.Outcome{
		{Provider: "a", Label: "A", Error: "额度用尽"},
		{Provider: "b", Label: "B", Blocks: []ocr.Block{{Text: "短"}}},
		{Provider: "c", Label: "C", Blocks: []ocr.Block{{Text: "这是一段更长的识别结果"}}},
	}
	if got := PickText(outcomes); got != "这是一段更长的识别结果" {
		t.Errorf("PickText = %q, 期望最长的那段", got)
	}
	if got := PickText([]ocr.Outcome{{Provider: "a", Error: "失败"}}); got != "" {
		t.Errorf("全部失败时 PickText = %q, 期望空", got)
	}
}

// 启用 OCR 文本规范化 is what turns a screenshot's printed line breaks back into
// a paragraph; off, the recogniser's lines are handed over untouched.
func TestOCRTextNormalizesOnlyWhenTheSettingIsOn(t *testing.T) {
	outcomes := []ocr.Outcome{{Provider: "apple",
		Blocks: []ocr.Block{{Text: "one"}, {Text: "two"}}}}
	if got := OCRText(settings.Config{}, outcomes); got != "one\ntwo" {
		t.Errorf("关闭时 OCRText = %q, 期望保留换行", got)
	}
	if got := OCRText(settings.Config{OCRTextNormalize: true}, outcomes); got != "one two" {
		t.Errorf("开启时 OCRText = %q, 期望合并成一行", got)
	}
}

func TestFirstTranslationSkipsFailures(t *testing.T) {
	res := &Response{Translate: []translate.Result{
		{Label: "A", Error: "超时"},
		{Label: "B", Text: "   "},
		{Label: "C", Text: " 译文 "},
		{Label: "D", Text: "不该出现"},
	}}
	if got := FirstTranslation(res); got != " 译文 " {
		t.Errorf("FirstTranslation = %q, 期望第一个真正回答的服务", got)
	}
	if got := FirstTranslation(nil); got != "" {
		t.Errorf("空结果应返回空串，得到 %q", got)
	}
}

func TestTypesComeFromRegistration(t *testing.T) {
	// The settings list is built from the packages' own Register calls, so a
	// kind that is not registered cannot appear here — and a registered one
	// must resolve back to the same category.
	for _, category := range []string{settings.CatOCR, settings.CatTranslate, settings.CatDict, settings.CatTTS} {
		types := Types(category)
		if len(types) == 0 {
			t.Fatalf("类别 %s 没有注册任何服务", category)
		}
		for _, st := range types {
			if got, ok := MetaOf(category, st.Kind); !ok || got.Kind != st.Kind {
				t.Errorf("%s/%s 无法通过 MetaOf 找回", category, st.Kind)
			}
		}
	}
	if _, ok := MetaOf(settings.CatTranslate, "不存在的服务"); ok {
		t.Error("未注册的 kind 不应被 MetaOf 接受")
	}
}

// 截图 OCR 是一张两阶段的窗口：勾选列表里只有识别服务，译文阶段仍必须跑起来，
// 否则面板只会把外语原样还给你。
func TestPlanRunsAStageTheWindowLeftUnticked(t *testing.T) {
	cfg := settings.Config{
		OCR: []settings.Provider{{ID: "ocr-apple", Category: settings.CatOCR,
			Kind: "apple", Label: "苹果 OCR", Enabled: true}},
		Translate: []settings.Provider{ready("tr-google"), needsKey("ai")},
		Selections: map[string][]string{
			settings.ContextScreenshot: {"ocr-apple"},
		},
	}
	if got := strings.Join(ids(plan(cfg, settings.ContextScreenshot, settings.CatTranslate)), ","); got != "tr-google" {
		t.Errorf("截图翻译的译文服务 = %q, want tr-google", got)
	}
	// The hint starts from the same list, so a service that would have run without
	// a key is named — that is the difference between 未识别到文字 and a fix.
	if skipped := Skipped(cfg, settings.ContextScreenshot, settings.CatTranslate); len(skipped) != 1 || skipped[0] != "AI" {
		t.Errorf("Skipped = %v, want [AI]", skipped)
	}
}

// 使用状态 overrides the window's checked list: 始终关闭 keeps a configured
// service out of automatic queries, 始终开启 adds it even when this window never
// ticked it.
func TestUsageStatusOverridesTheWindowSelection(t *testing.T) {
	mk := func(id, status string) settings.Provider {
		p := ready(id)
		if status != "" {
			p.Extra = map[string]string{settings.FieldUsageStatus: status}
		}
		return p
	}
	cfg := settings.Config{
		Translate: []settings.Provider{
			mk("a", ""), mk("b", settings.UsageAlwaysOff), mk("c", settings.UsageAlwaysOn),
		},
		Selections: map[string][]string{settings.ContextMain: {"a"}},
	}
	if got := strings.Join(ids(plan(cfg, settings.ContextMain, settings.CatTranslate)), ","); got != "a,c" {
		t.Errorf("plan = %q, want a,c", got)
	}
	cfg.Selections = nil
	if got := strings.Join(ids(plan(cfg, settings.ContextMain, settings.CatTranslate)), ","); got != "a,c" {
		t.Errorf("无勾选时 plan = %q, want a,c", got)
	}
}

// 朗读 is a button on a window whose tick list holds text services, so the voice
// list must not be read as "this window chose no voice".
func TestPlanGivesSpeechEveryReadyVoice(t *testing.T) {
	cfg := settings.Config{
		TTS: []settings.Provider{
			{ID: "tts-a", Category: settings.CatTTS, Kind: "youdao", Label: "有道朗读", Enabled: true},
			{ID: "off", Category: settings.CatTTS, Kind: "youdao", Label: "off"},
		},
		Translate: []settings.Provider{ready("tr-google")},
		Selections: map[string][]string{
			settings.ContextMain: {"tr-google"},
		},
	}
	if got := strings.Join(ids(plan(cfg, settings.ContextMain, settings.CatTTS)), ","); got != "tts-a" {
		t.Errorf("朗读服务 = %q, want tts-a", got)
	}
}

// A window with nowhere to show a stage must not spend requests filling it: the
// replace prompt has one line, and a paragraph of OCR text is not a headword.
func TestContextRunsOnlyItsDeclaredStages(t *testing.T) {
	cases := []struct {
		context string
		want    map[string]bool
	}{
		{settings.ContextReplace, map[string]bool{
			settings.CatTranslate: true, settings.CatDict: false, settings.CatOCR: false}},
		{settings.ContextODict, map[string]bool{
			settings.CatDict: true, settings.CatTranslate: false}},
		{settings.ContextScreenshot, map[string]bool{
			settings.CatOCR: true, settings.CatTranslate: true, settings.CatDict: false}},
		{settings.ContextSilentOCR, map[string]bool{
			settings.CatOCR: true, settings.CatTranslate: false}},
		{settings.ContextMain, map[string]bool{
			settings.CatDict: true, settings.CatTranslate: true, settings.CatOCR: false}},
	}
	for _, tc := range cases {
		for category, want := range tc.want {
			if got := declares(tc.context, category); got != want {
				t.Errorf("%s 跑 %s = %v, want %v", tc.context, category, got, want)
			}
		}
	}
}

// A service page is offered as a way to check a card, so the template has to
// arrive at the site with the queried text in it — a leftover placeholder opens
// the front page, and an unescaped word opens the wrong word.
func TestEveryDeclaredServicePageCarriesTheQuery(t *testing.T) {
	for _, types := range [][]settings.ServiceType{translate.Types(), dict.Types()} {
		for _, meta := range types {
			if meta.Site == "" {
				continue
			}
			got := meta.SiteURL("测试 text", "auto", "en")
			if strings.ContainsAny(got, "{}") {
				t.Errorf("%s/%s 的网页模板展开后仍有花括号：%q", meta.Category, meta.Kind, got)
			}
			if !strings.Contains(got, "%E6%B5%8B%E8%AF%95") {
				t.Errorf("%s/%s 的网页没有带上所查的词：%q", meta.Category, meta.Kind, got)
			}
		}
	}
}

// 英语单词优先使用有道发音 puts the dictionary voice first for one English
// word, and only there.
func TestPreferYoudaoWordReordersOnlyEnglishWords(t *testing.T) {
	providers := []settings.Provider{
		{ID: "tts-google", Kind: "google", Label: "Google 朗读"},
		{ID: "tts-youdao", Kind: "youdao", Label: "有道朗读"},
	}
	got := preferYoudaoWord(providers, "hello", "en-US")
	if len(got) != 2 || got[0].Kind != "youdao" || got[1].Kind != "google" {
		t.Fatalf("单词没有把有道排到前面: %v", got)
	}
	if providers[0].Kind != "google" {
		t.Error("原切片被修改了")
	}
	if got := preferYoudaoWord(providers, "hello world", "en-US"); got[0].Kind != "google" {
		t.Errorf("句子不该调整顺序: %v", got)
	}
	if got := preferYoudaoWord(providers, "hello", "zh-CN"); got[0].Kind != "google" {
		t.Errorf("非英语不该调整顺序: %v", got)
	}
}

func TestYoudaoFallbackRunsOnlyWhenNothingWasRead(t *testing.T) {
	called := 0
	old := youdaoFallbackOCR
	youdaoFallbackOCR = func(_ context.Context, _ []byte) ([]ocr.Block, error) {
		called++
		return []ocr.Block{{Text: "光"}}, nil
	}
	t.Cleanup(func() { youdaoFallbackOCR = old })

	failed := []ocr.Outcome{{Provider: "ocr-apple", Label: "苹果 OCR", Error: "没有文字"}}
	cfg := settings.Config{EnableYoudaoOCR: true}
	got := withYoudaoFallback(context.Background(), cfg, failed, []byte("png"))
	if called != 1 || len(got) != 2 {
		t.Fatalf("默认 OCR 失败时应补一次有道 OCR，得到 %d 次调用、%d 条结果", called, len(got))
	}
	last := got[1]
	if last.Label != "有道 OCR" || last.Error != "" || last.Text() != "光" {
		t.Errorf("补充结果不对: %#v", last)
	}

	called = 0
	read := []ocr.Outcome{{Provider: "ocr-apple", Label: "苹果 OCR", Blocks: []ocr.Block{{Text: "hello"}}}}
	if got := withYoudaoFallback(context.Background(), cfg, read, []byte("png")); len(got) != 1 || called != 0 {
		t.Errorf("已经识别出文字时不该再请求有道，得到 %d 次调用", called)
	}

	off := settings.Config{}
	if got := withYoudaoFallback(context.Background(), off, failed, []byte("png")); len(got) != 1 || called != 0 {
		t.Errorf("未启用时不该请求有道，得到 %d 次调用", called)
	}
}

func TestYoudaoFallbackKeepsTheOriginalFailureWhenItFailsToo(t *testing.T) {
	old := youdaoFallbackOCR
	youdaoFallbackOCR = func(_ context.Context, _ []byte) ([]ocr.Block, error) {
		return nil, errors.New("有道 OCR 没有识别出文字")
	}
	t.Cleanup(func() { youdaoFallbackOCR = old })

	failed := []ocr.Outcome{{Provider: "ocr-apple", Label: "苹果 OCR", Error: "没有文字"}}
	got := withYoudaoFallback(context.Background(), settings.Config{EnableYoudaoOCR: true},
		failed, []byte("png"))
	if len(got) != 2 {
		t.Fatalf("失败也应留下记录，得到 %d 条", len(got))
	}
	if got[0].Error != "没有文字" || got[1].Error == "" {
		t.Errorf("两条失败都要保留，得到 %#v", got)
	}
}

// An empty answer is EasyDict's noResult warning: the row folds and hides 重试.
// A service that failed to answer at all is a fault the button can fix.
func TestEmptyAnswerIsAWarningButAFailureIsNot(t *testing.T) {
	cases := []struct {
		name    string
		result  translate.Result
		wantErr string
		warning bool
	}{
		{"空结果", translate.Result{Provider: "tr-google"}, "返回空译文", true},
		{"流式服务没出声是故障", translate.Result{Provider: "tr-openai", Stream: true}, "返回空译文", false},
		{"服务自己说空", translate.Result{Provider: "tr-baidu", Error: "百度翻译返回空译文: {}"}, "百度翻译返回空译文: {}", true},
		{"苹果没有输出", translate.Result{Provider: "tr-apple", Error: "苹果翻译没有输出译文"}, "苹果翻译没有输出译文", true},
		{"网络失败", translate.Result{Provider: "tr-bing", Error: "必应翻译 HTTP 500"}, "必应翻译 HTTP 500", false},
		{"有译文", translate.Result{Provider: "tr-google", Text: "你好"}, "", false},
	}
	for _, c := range cases {
		got := c.result
		finishAnswer(&got)
		if got.Error != c.wantErr || got.Warning != c.warning {
			t.Errorf("%s: 得到 error=%q warning=%v，想要 error=%q warning=%v",
				c.name, got.Error, got.Warning, c.wantErr, c.warning)
		}
	}
}

// A recognized screenshot that is nothing but a link is returned as itself;
// anything else — a link inside a sentence, a bare domain — is still a query.
func TestWholeURLOnlyAcceptsOneWebLink(t *testing.T) {
	cases := []struct {
		text string
		want bool
	}{
		{"https://example.com/a?b=1", true},
		{"http://例子.cn", true},
		{"example.com", false},
		{"", false},
		{"见 https://example.com", false},
		{"mailto:someone@example.com", false},
	}
	for _, c := range cases {
		if got := WholeURL(c.text); got != c.want {
			t.Errorf("WholeURL(%q) = %v，想要 %v", c.text, got, c.want)
		}
	}
}

// Apple's dictionary lives in an app rather than a page: its card link is the
// dict:// scheme EasyDict's 在苹果词典中打开 button uses. Web dictionaries keep
// their declared page in Site and get no scheme link.
func TestAppleDictionaryOpensInItsApp(t *testing.T) {
	cases := []struct{ kind, text, want string }{
		{"apple", "hello", "dict://hello"},
		{"apple", "  hello world  ", "dict://hello%20world"},
		{"apple", "苹果", "dict://%E8%8B%B9%E6%9E%9C"},
		{"youdao", "hello", ""},
		{"", "hello", ""},
	}
	for _, c := range cases {
		if got := dictOpenURL(c.kind, c.text); got != c.want {
			t.Errorf("dictOpenURL(%q, %q) = %q，想要 %q", c.kind, c.text, got, c.want)
		}
	}
}
