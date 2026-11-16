package query

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"transdict/internal/dict"
	"transdict/internal/ocr"
	"transdict/internal/settings"
	"transdict/internal/translate"
	"transdict/internal/tts"
)

// Three services that answer without a network, a key or a screen. They exist to
// prove which stages a run drives: the settings tables can be reasoned about from
// a plan, but 截图翻译 producing a translation at all is a pipeline fact.
func init() {
	ocr.Register(settings.ServiceType{Kind: "test-ocr", Name: "测试识别"},
		func(settings.Provider) (ocr.Provider, error) { return readAloud{}, nil })
	translate.Register(settings.ServiceType{Kind: "test-tr", Name: "测试翻译"},
		func(settings.Provider) (translate.Provider, error) { return echo{}, nil })
	translate.Register(settings.ServiceType{Kind: "test-slow", Name: "慢翻译"},
		func(settings.Provider) (translate.Provider, error) { return slowEcho{}, nil })
	translate.Register(settings.ServiceType{Kind: "test-fast", Name: "快翻译"},
		func(settings.Provider) (translate.Provider, error) { return echo{}, nil })
	// A service that only answers into Chinese, the way 有道 refuses 日语→韩语.
	translate.Register(settings.ServiceType{Kind: "test-pair", Name: "仅中文翻译"},
		func(settings.Provider) (translate.Provider, error) { return chineseOnly{}, nil })
	translate.Register(settings.ServiceType{Kind: "test-stream", Name: "流式翻译"},
		func(settings.Provider) (translate.Provider, error) { return streamEcho{}, nil })
	// A stream that never finishes on its own, so 停止生成 has something to end.
	translate.Register(settings.ServiceType{Kind: "test-halt", Name: "长流翻译"},
		func(settings.Provider) (translate.Provider, error) { return streamHalt{}, nil })
	dict.Register(settings.ServiceType{Kind: "test-dict", Name: "测试词典"},
		func(settings.Provider) (dict.Provider, error) { return gloss{}, nil })
	tts.Register(settings.ServiceType{Kind: "test-tts", Name: "测试朗读"},
		func(settings.Provider) (tts.Provider, error) { return silent{}, nil })
	tts.Register(settings.ServiceType{Kind: "test-tts-b", Name: "第二个朗读"},
		func(settings.Provider) (tts.Provider, error) { return silentB{}, nil })
	tts.Register(settings.ServiceType{Kind: "test-tts-mute", Name: "无声朗读"},
		func(settings.Provider) (tts.Provider, error) { return muted{}, nil })
}

// A model that answers in Markdown and a dictionary with a page of its own are
// described by their registration, not by their reply: the panel has to know how
// to draw an answer before it reads one.
func init() {
	translate.Register(settings.ServiceType{
		Kind: "test-ai", Name: "测试模型", Markdown: true,
		Site: "https://ai.test/?sl={source}&tl={target}&text={text}",
	}, func(settings.Provider) (translate.Provider, error) { return echo{}, nil })
	dict.Register(settings.ServiceType{
		Kind: "test-page", Name: "测试网页词典", Site: "https://dict.test/?w={text}",
	}, func(settings.Provider) (dict.Provider, error) { return gloss{}, nil })
	translate.Register(settings.ServiceType{Kind: "test-prompt", Name: "测试润色"},
		func(settings.Provider) (translate.Provider, error) { return prompted{}, nil })
}

type readAloud struct{}

func (readAloud) Label() string { return "测试识别" }
func (readAloud) Recognize(context.Context, []byte) ([]ocr.Block, error) {
	return []ocr.Block{{Text: "hello world"}}, nil
}

type echo struct{}

func (echo) Label() string { return "测试翻译" }
func (echo) Translate(_ context.Context, text, _, _ string) (string, string, error) {
	return "译文:" + text, "en", nil
}

type chineseOnly struct{}

func (chineseOnly) Label() string { return "仅中文翻译" }
func (chineseOnly) Translate(_ context.Context, text, _, _ string) (string, string, error) {
	return "译文:" + text, "ja", nil
}

func (chineseOnly) SupportsPair(_, target string) bool {
	return target == "zh-CN" || target == "zh-TW"
}

// A pair the service cannot answer is left out of the answer entirely: an error
// card beside two working services says nothing useful. The target alone can
// prove it — nothing here translates into 韩语 — while a supported target still
// runs when the source is being detected.
func TestAProviderWithoutARouteIsLeftOut(t *testing.T) {
	cfg := pipelineConfig(map[string][]string{settings.ContextMain: {"tr", "pair"}})
	cfg.Translate = append(cfg.Translate, enabled("pair", settings.CatTranslate, "test-pair"))
	e := fixture(t, cfg)

	refused := e.Run(context.Background(), Request{
		Context: settings.ContextMain, Text: "こんにちは", SourceLang: "ja", TargetLang: "ko",
	})
	if len(refused.Translate) != 1 || refused.Translate[0].Provider != "tr" {
		t.Fatalf("日语→韩语 = %+v, want 只剩能翻译的服务", refused.Translate)
	}

	auto := e.Run(context.Background(), Request{
		Context: settings.ContextMain, Text: "hello", SourceLang: "auto", TargetLang: "zh-CN",
	})
	if len(auto.Translate) != 2 {
		t.Fatalf("自动识别→中文 = %d 条, want 2（目标支持时仍要运行）", len(auto.Translate))
	}
}

// 自动选择 resolves once the text is known: Chinese answers in English and
// everything else in Chinese, the pair the app exists for.
func TestAutoTargetFollowsTheText(t *testing.T) {
	e := fixture(t, pipelineConfig(map[string][]string{settings.ContextMain: {"tr"}}))

	zh := e.Run(context.Background(), Request{
		Context: settings.ContextMain, Text: "今天天气很好", SourceLang: "auto", TargetLang: "auto",
	})
	if zh.TargetLang != "en" {
		t.Errorf("中文 → 自动选择 = %q, want en", zh.TargetLang)
	}
	en := e.Run(context.Background(), Request{
		Context: settings.ContextMain, Text: "hello", SourceLang: "auto", TargetLang: "auto",
	})
	if en.TargetLang != "zh-CN" {
		t.Errorf("英文 → 自动选择 = %q, want zh-CN", en.TargetLang)
	}
}

type gloss struct{}

func (gloss) Label() string { return "测试词典" }
func (gloss) Explain(_ context.Context, text, _, _ string) ([]dict.Entry, error) {
	return []dict.Entry{{Word: text, Text: "释义"}}, nil
}

type silent struct{}

func (silent) Label() string { return "测试朗读" }
func (silent) Speak(context.Context, string, string, bool) (*tts.Clip, error) {
	return &tts.Clip{MIME: "audio/mpeg", URL: "https://speech.test/hello.mp3"}, nil
}

type silentB struct{}

func (silentB) Label() string { return "第二个朗读" }
func (silentB) Speak(context.Context, string, string, bool) (*tts.Clip, error) {
	return &tts.Clip{MIME: "audio/mpeg", URL: "https://speech.test/default.mp3"}, nil
}

type muted struct{}

func (muted) Label() string { return "无声朗读" }
func (muted) Speak(context.Context, string, string, bool) (*tts.Clip, error) {
	return nil, errors.New("这段文本没有音频")
}

// A voice that cannot read this text must not mute the window: the next
// configured service is tried, and only when all of them fail does the answer
// carry the error the user asked for first.
func TestSpeakFallsBackToTheNextVoice(t *testing.T) {
	cfg := pipelineConfig(nil)
	cfg.TTS = []settings.Provider{
		enabled("mute", settings.CatTTS, "test-tts-mute"),
		enabled("tts", settings.CatTTS, "test-tts"),
	}
	res := fixture(t, cfg).Speak("hello", "en", false)
	if res.Error != "" {
		t.Fatalf("朗读失败 = %q, want 第二个服务念出来", res.Error)
	}
	if res.Provider != "tts" || res.Clip.Source() == "" {
		t.Fatalf("结果 = %+v, want 第二个服务的音频", res)
	}
}

// 默认 TTS 服务 moves one configured voice to the front; it does not remove the
// fallback behind it, so an installed-but-broken default still speaks.
func TestSpeakPrefersTheDefaultVoice(t *testing.T) {
	cfg := pipelineConfig(nil)
	cfg.TTS = []settings.Provider{
		enabled("first", settings.CatTTS, "test-tts"),
		enabled("default", settings.CatTTS, "test-tts-b"),
	}
	cfg.DefaultTTS = "default"
	res := fixture(t, cfg).Speak("hello", "en", false)
	if res.Error != "" || res.Provider != "default" {
		t.Fatalf("结果 = %+v, want 默认服务先念", res)
	}
	if res.Clip.URL != "https://speech.test/default.mp3" {
		t.Errorf("音频 = %q, want 默认服务的音频", res.Clip.URL)
	}

	cfg.DefaultTTS = ""
	res = fixture(t, cfg).Speak("hello", "en", false)
	if res.Provider != "first" {
		t.Errorf("没有默认服务时 = %q, want 配置顺序里的第一个", res.Provider)
	}
}

func TestSpeakReportsTheFirstVoiceWhenAllFail(t *testing.T) {
	cfg := pipelineConfig(nil)
	cfg.TTS = []settings.Provider{
		enabled("mute", settings.CatTTS, "test-tts-mute"),
		enabled("mute2", settings.CatTTS, "test-tts-mute"),
	}
	res := fixture(t, cfg).Speak("hello", "en", false)
	if res.Provider != "mute" || !strings.Contains(res.Error, "没有音频") {
		t.Fatalf("结果 = %+v, want 第一个服务的错误", res)
	}
}

// barrierTr answers only after the dictionary stage has started, and the
// dictionary releases it. A serial pipeline would leave the translator waiting
// until its own guard fires — which is the regression this pair guards against.
type barrierTr struct{ release <-chan struct{} }

func (barrierTr) Label() string { return "测试翻译" }
func (b barrierTr) Translate(_ context.Context, text, _, _ string) (string, string, error) {
	select {
	case <-b.release:
		return "译文:" + text, "en", nil
	case <-time.After(2 * time.Second):
		return "", "", errors.New("词典阶段没有与翻译并行开始")
	}
}

type releaseDict struct{ release chan struct{} }

func (releaseDict) Label() string { return "测试词典" }
func (r releaseDict) Explain(_ context.Context, text, _, _ string) ([]dict.Entry, error) {
	close(r.release)
	return []dict.Entry{{Word: text, Text: "释义"}}, nil
}

func enabled(id, category, kind string) settings.Provider {
	return settings.Provider{ID: id, Category: category, Kind: kind, Label: id, Enabled: true}
}

// fixture writes the config into a file under the test's temp directory, so the
// engine reads exactly what a fresh install with these services would read.
func fixture(t *testing.T, cfg settings.Config) *Engine {
	t.Helper()
	store, err := settings.NewStoreAt(t.TempDir() + "/config.json")
	if err != nil {
		t.Fatalf("打开测试配置: %v", err)
	}
	if _, err := store.Update(func(c *settings.Config) { *c = cfg }); err != nil {
		t.Fatalf("写入测试配置: %v", err)
	}
	return New(store)
}

func pipelineConfig(selections map[string][]string) settings.Config {
	return settings.Config{
		OCR:        []settings.Provider{enabled("ocr", settings.CatOCR, "test-ocr")},
		Translate:  []settings.Provider{enabled("tr", settings.CatTranslate, "test-tr")},
		Dict:       []settings.Provider{enabled("dict", settings.CatDict, "test-dict")},
		TTS:        []settings.Provider{enabled("tts", settings.CatTTS, "test-tts")},
		SourceLang: "auto", TargetLang: "zh-CN",
		Selections: selections,
		// A fresh install translates what it read; the zero value here would make
		// every screenshot test look like 图片 OCR 后自动查询 was switched off.
		AutoQueryOCR: true,
	}
}

// The 截图 OCR window ticks the recognisers, and nothing else. That is a preference
// about how the text is read, not a decision to leave it untranslated.
func TestScreenshotQueryTranslatesWhatItRead(t *testing.T) {
	e := fixture(t, pipelineConfig(map[string][]string{
		settings.ContextScreenshot: {"ocr"},
	}))

	res := e.Run(context.Background(), Request{
		Context: settings.ContextScreenshot, Mode: "screenshot", Image: []byte("png-bytes"),
	})
	if res.SourceText != "hello world" {
		t.Fatalf("识别原文 = %q, want %q", res.SourceText, "hello world")
	}
	if len(res.Translate) != 1 || res.Translate[0].Text != "译文:hello world" {
		t.Fatalf("截图翻译的译文 = %+v, want 一条译文（阶段没勾选不等于阶段不运行）", res.Translate)
	}
	// A paragraph of recognized text is not a headword, so the dictionary stage
	// stays out of a screenshot run.
	if len(res.Dict) != 0 {
		t.Errorf("截图不应查词典，得到 %+v", res.Dict)
	}
}

// 图片 OCR 后自动查询 off: the text is the whole answer, so neither the
// translators nor the dictionary are asked. The crop is still shown, which is
// what makes the recognized text usable at all.
func TestScreenshotCanStopAtTheRecognizedText(t *testing.T) {
	cfg := pipelineConfig(map[string][]string{settings.ContextScreenshot: {"ocr", "tr"}})
	cfg.AutoQueryOCR = false
	e := fixture(t, cfg)

	var partial *Response
	res := e.RunPartial(context.Background(), Request{
		Context: settings.ContextScreenshot, Mode: "screenshot", Image: []byte("png-bytes"),
	}, func(p *Response) { partial = p })
	if res.SourceText != "hello world" {
		t.Fatalf("识别原文 = %q, want %q", res.SourceText, "hello world")
	}
	if len(res.Translate) != 0 || len(res.Dict) != 0 {
		t.Errorf("关闭自动查询后 = 译文 %d 词典 %d, want 0 与 0", len(res.Translate), len(res.Dict))
	}
	if res.Error != "" {
		t.Errorf("停止在识别文字不该报错，得到 %q", res.Error)
	}
	if res.ImageURL == "" {
		t.Error("截图仍应带上裁切图")
	}
	// The sketch still goes out first, so the panel shows the crop while the
	// final answer replaces it with the text.
	if partial == nil || partial.SourceText != "hello world" || !partial.Partial {
		t.Errorf("第一帧 = %+v, want 带识别文字的草稿", partial)
	}
}

// The same request on a context that shows one line must not pay for a dictionary
// round trip it cannot display.
func TestTextQueryRunsOnlyTheStagesAWindowShows(t *testing.T) {
	e := fixture(t, pipelineConfig(map[string][]string{
		settings.ContextReplace: {"tr"},
		settings.ContextMain:    {"dict", "tr"},
	}))

	replace := e.Run(context.Background(), Request{Context: settings.ContextReplace, Mode: "replace", Text: "hello"})
	if len(replace.Translate) != 1 || len(replace.Dict) != 0 {
		t.Errorf("翻译并替换 = 译文 %d 词典 %d, want 1 与 0", len(replace.Translate), len(replace.Dict))
	}

	main := e.Run(context.Background(), Request{Context: settings.ContextMain, Mode: "input", Text: "hello"})
	if len(main.Translate) != 1 || len(main.Dict) != 1 {
		t.Errorf("主窗口 = 译文 %d 词典 %d, want 1 与 1", len(main.Translate), len(main.Dict))
	}
}

// A word lookup shows the dictionary card as soon as the dictionary answers; it
// must not wait for the slowest translator. The barrier makes the ordering a
// fact rather than a timing measurement.
func TestDictionaryStartsWithoutWaitingForTranslation(t *testing.T) {
	release := make(chan struct{})
	translate.Register(settings.ServiceType{Kind: "test-tr-barrier", Name: "测试翻译"},
		func(settings.Provider) (translate.Provider, error) { return barrierTr{release: release}, nil })
	dict.Register(settings.ServiceType{Kind: "test-dict-release", Name: "测试词典"},
		func(settings.Provider) (dict.Provider, error) { return releaseDict{release: release}, nil })

	cfg := pipelineConfig(map[string][]string{settings.ContextMain: {"tr", "dict"}})
	cfg.Translate = []settings.Provider{enabled("tr", settings.CatTranslate, "test-tr-barrier")}
	cfg.Dict = []settings.Provider{enabled("dict", settings.CatDict, "test-dict-release")}
	e := fixture(t, cfg)

	res := e.Run(context.Background(), Request{Context: settings.ContextMain, Mode: "input", Text: "hello"})
	if len(res.Translate) != 1 || res.Translate[0].Error != "" {
		t.Fatalf("译文 = %+v, want 词典并行释放后得到译文", res.Translate)
	}
	if len(res.Dict) != 1 {
		t.Fatalf("词典 %d 条, want 1", len(res.Dict))
	}
}

// 朗读 is a button on a window whose tick list holds text services, so a voice has
// to be findable even though no context lists one.
func TestSpeechFindsAVoiceForAnyWindow(t *testing.T) {
	e := fixture(t, pipelineConfig(map[string][]string{
		settings.ContextMain: {"dict", "tr"},
	}))
	got := e.Speak("hello", "en", false)
	if got.Error != "" {
		t.Fatalf("朗读失败：%q", got.Error)
	}
	if got.Provider != "tts" || got.Clip.URL == "" {
		t.Errorf("朗读结果 = %+v, want 服务 tts 与一段音频", got)
	}
}

// A card that quotes a model's answer as plain text shows the user asterisks and
// fences, and a card with no way out to the service's own page leaves them
// believing whatever came back.
func TestAnswerCarriesHowItShouldBeShown(t *testing.T) {
	// Nothing is ticked, so both declared services and both plain ones run and the
	// two shapes can be compared inside one response.
	cfg := pipelineConfig(nil)
	cfg.Translate = append(cfg.Translate, enabled("ai", settings.CatTranslate, "test-ai"))
	cfg.Dict = append(cfg.Dict, enabled("page", settings.CatDict, "test-page"))
	e := fixture(t, cfg)

	res := e.Run(context.Background(), Request{Context: settings.ContextMain, Mode: "input", Text: "a b"})
	if len(res.Translate) != 2 || len(res.Dict) != 2 {
		t.Fatalf("译文 %d 词典 %d, want 2 与 2", len(res.Translate), len(res.Dict))
	}
	var ai, plain *translate.Result
	for i := range res.Translate {
		if res.Translate[i].Provider == "ai" {
			ai = &res.Translate[i]
		} else {
			plain = &res.Translate[i]
		}
	}
	if ai == nil || plain == nil {
		t.Fatalf("译文 = %+v, want 两条", res.Translate)
	}
	if !ai.Markdown {
		t.Errorf("模型译文 Markdown = false, want true（面板会原样引用 ** 与 ```）")
	}
	if ai.Site != "https://ai.test/?sl=auto&tl=zh-CN&text=a+b" {
		t.Errorf("模型译文网页 = %q", ai.Site)
	}
	// A registration that declares nothing must not guess: an ordinary translator
	// rendering as prose would hide its own wording.
	if plain.Markdown || plain.Site != "" {
		t.Errorf("普通译文被加工：Markdown=%v 网页=%q", plain.Markdown, plain.Site)
	}
	var page bool
	for _, d := range res.Dict {
		if d.Provider == "page" {
			page = d.Site == "https://dict.test/?w=a+b"
			if !page {
				t.Errorf("词典网页 = %q, want 带上下文的链接", d.Site)
			}
		} else if d.Site != "" {
			t.Errorf("未声明网页的词典给出 %q", d.Site)
		}
	}
	if !page {
		t.Error("词典结果缺少网页链接")
	}
}

// 截图翻译 shows the recognized text before any translator answers, then each
// answer as it lands. Without this the panel sits empty for the whole network
// round trip, which is what made the feature feel slow.
func TestScreenshotReportsTextBeforeTranslations(t *testing.T) {
	e := fixture(t, pipelineConfig(map[string][]string{
		settings.ContextScreenshot: {"ocr", "tr"},
	}))

	var partials []*Response
	res := e.RunPartial(context.Background(), Request{
		Context: settings.ContextScreenshot, Mode: "screenshot", Image: []byte("png-bytes"),
	}, func(p *Response) { partials = append(partials, p) })

	if len(partials) != 2 {
		t.Fatalf("中间结果 %d 次, want 2 (原文 + 一个译文)", len(partials))
	}
	first := partials[0]
	if !first.Partial || first.SourceText != "hello world" || len(first.Translate) != 0 || len(first.Dict) != 0 {
		t.Errorf("第一次中间结果 = %+v, want 原文已到、译文未到", first)
	}
	second := partials[1]
	if !second.Partial || len(second.Translate) != 1 || second.Translate[0].Text == "" {
		t.Errorf("第二次中间结果 = %+v, want 译文已到", second)
	}
	if res.Partial || len(res.Translate) != 1 {
		t.Errorf("最终结果 = %+v, want 完整译文且非 partial", res)
	}
}

// A text request has its source already, so the first partial it sends is a
// finished translation rather than an empty sketch.
func TestTextQueryStreamsFinishedTranslations(t *testing.T) {
	e := fixture(t, pipelineConfig(map[string][]string{settings.ContextMain: {"tr"}}))
	var partials []*Response
	res := e.RunPartial(context.Background(), Request{
		Context: settings.ContextMain, Mode: "input", Text: "hello",
	}, func(p *Response) { partials = append(partials, p) })

	if len(partials) != 1 {
		t.Fatalf("中间结果 %d 次, want 1", len(partials))
	}
	p := partials[0]
	if !p.Partial || p.SourceText != "hello" || len(p.Translate) != 1 {
		t.Errorf("中间结果 = %+v, want 源文与已完成的译文", p)
	}
	if res.Partial || len(res.Translate) != 1 {
		t.Errorf("最终结果 = %+v, want 完整且非 partial", res)
	}
}

// slowDelay makes the slow provider in the streaming test take noticeably
// longer than the fast one. It is package state because the registry factory
// cannot receive per-test arguments.
var slowDelay = 150 * time.Millisecond

type slowEcho struct{}

func (slowEcho) Label() string { return "慢翻译" }
func (slowEcho) Translate(_ context.Context, text, _, _ string) (string, string, error) {
	time.Sleep(slowDelay)
	return "慢:" + text, "en", nil
}

// TestFastServiceIsShownBeforeSlowOneFinishes is the regression this change is
// for: waiting for every translator before sending anything made a 90ms answer
// wait on a 900ms one.
func TestFastServiceIsShownBeforeSlowOneFinishes(t *testing.T) {
	cfg := pipelineConfig(map[string][]string{settings.ContextMain: {"slow", "fast"}})
	cfg.Translate = []settings.Provider{
		enabled("slow", settings.CatTranslate, "test-slow"),
		enabled("fast", settings.CatTranslate, "test-fast"),
	}
	e := fixture(t, cfg)

	start := time.Now()
	var mu sync.Mutex
	var firstAnswer time.Duration
	res := e.RunPartial(context.Background(), Request{
		Context: settings.ContextMain, Mode: "input", Text: "hello",
	}, func(p *Response) {
		if len(p.Translate) == 0 {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		if firstAnswer == 0 {
			firstAnswer = time.Since(start)
		}
	})

	mu.Lock()
	seen := firstAnswer
	mu.Unlock()
	if seen == 0 || seen >= slowDelay {
		t.Errorf("第一个译文在 %v 才出现, want 快服务于慢的 %v 之前", seen, slowDelay)
	}
	if len(res.Translate) != 2 || res.Translate[0].Provider != "slow" || res.Translate[1].Provider != "fast" {
		t.Errorf("最终译文顺序 = %+v, want 配置顺序 slow, fast", res.Translate)
	}
}

// streamPace is longer than the 80ms throttle translateStage uses, so every
// chunk of the fake answer reaches the window instead of being merged away.
var streamPace = 100 * time.Millisecond

type streamEcho struct{}

func (streamEcho) Label() string { return "流式翻译" }

func (streamEcho) Translate(ctx context.Context, text, src, dst string) (string, string, error) {
	return streamEcho{}.TranslateStream(ctx, text, src, dst, nil)
}

func (streamEcho) TranslateStream(ctx context.Context, text, _, _ string, onText func(string)) (string, string, error) {
	chunks := []string{"译", "译文", "译文:" + text}
	for i, chunk := range chunks {
		if i > 0 {
			select {
			case <-ctx.Done():
				return "", "", ctx.Err()
			case <-time.After(streamPace):
			}
		}
		if onText != nil {
			onText(chunk)
		}
	}
	return chunks[len(chunks)-1], "en", nil
}

// streamHalt sends one piece and then waits for the caller to stop it, the way
// a long AI answer keeps generating until 停止生成 is pressed.
type streamHalt struct{}

func (streamHalt) Label() string { return "长流翻译" }

func (streamHalt) Translate(ctx context.Context, text, src, dst string) (string, string, error) {
	return streamHalt{}.TranslateStream(ctx, text, src, dst, nil)
}

func (streamHalt) TranslateStream(ctx context.Context, _, _, _ string, onText func(string)) (string, string, error) {
	if onText != nil {
		onText("半")
	}
	<-ctx.Done()
	return "", "", ctx.Err()
}

// A streaming service is readable before it is finished: the card grows word by
// word instead of showing loading dots until the last token.
func TestStreamedAnswerReachesTheCardBeforeItFinishes(t *testing.T) {
	cfg := pipelineConfig(map[string][]string{settings.ContextMain: {"stream"}})
	cfg.Translate = []settings.Provider{enabled("stream", settings.CatTranslate, "test-stream")}
	e := fixture(t, cfg)

	var mu sync.Mutex
	var seen []string
	sawStreaming := false
	res := e.RunPartial(context.Background(), Request{
		Context: settings.ContextMain, Mode: "input", Text: "hello",
	}, func(p *Response) {
		if len(p.Translate) == 0 || p.Translate[0].Pending {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		if p.Translate[0].Streaming {
			sawStreaming = true
		}
		text := p.Translate[0].Text
		if len(seen) == 0 || seen[len(seen)-1] != text {
			seen = append(seen, text)
		}
	})

	if !sawStreaming {
		t.Error("中途没有一张卡标出 停止生成")
	}

	mu.Lock()
	texts := append([]string(nil), seen...)
	mu.Unlock()

	if len(texts) < 2 {
		t.Fatalf("流式中途只推送了 %v, want 译文完成前的中间文本", texts)
	}
	for i, text := range texts {
		if !strings.HasPrefix("译文:hello", text) {
			t.Errorf("第 %d 个快照 = %q, want 最终译文的片段", i, text)
		}
	}
	if len(texts[len(texts)-1]) <= len(texts[0]) {
		t.Errorf("快照没有增长: %v", texts)
	}
	if res.Partial || len(res.Translate) != 1 || res.Translate[0].Text != "译文:hello" || res.Translate[0].Pending {
		t.Errorf("最终结果 = %+v, want 完整译文且非 partial", res)
	}
	if len(res.Translate) == 1 && res.Translate[0].Streaming {
		t.Errorf("完成的译文仍标着 streaming: %+v", res.Translate[0])
	}
}

// 停止生成 ends one service where it stands: the words already on the card stay,
// the row is not an error, and a second press has nothing left to stop.
func TestStopEndsOneStreamWithTheTextThatArrived(t *testing.T) {
	cfg := pipelineConfig(map[string][]string{settings.ContextMain: {"halt"}})
	cfg.Translate = []settings.Provider{enabled("halt", settings.CatTranslate, "test-halt")}
	e := fixture(t, cfg)

	arrived := make(chan struct{})
	var once sync.Once
	done := make(chan *Response, 1)
	go func() {
		done <- e.RunPartial(context.Background(), Request{
			Context: settings.ContextMain, Mode: "input", Text: "hello",
		}, func(p *Response) {
			if len(p.Translate) > 0 && p.Translate[0].Text == "半" {
				once.Do(func() { close(arrived) })
			}
		})
	}()

	select {
	case <-arrived:
	case <-time.After(2 * time.Second):
		t.Fatal("没有等到第一段流式文本")
	}
	if !e.StopTranslate(settings.ContextMain, "halt") {
		t.Fatal("流式服务应可停止")
	}

	select {
	case res := <-done:
		if res.Partial {
			t.Errorf("停止后的结果仍是 partial: %+v", res)
		}
		if len(res.Translate) != 1 || res.Translate[0].Text != "半" || res.Translate[0].Error != "" {
			t.Errorf("停止后应保留已到的文本且不算错误, 实际 %+v", res.Translate)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("停止后查询没有结束")
	}
	if e.StopTranslate(settings.ContextMain, "halt") {
		t.Error("结束后不应再报告可停止")
	}
}

// A sentence is not a headword: 有道 answers one with 网络释义 for the whole
// sentence and Apple with 未收录, which is three cards of noise under a
// translation that was already right.
func TestDictionaryStageSkipsSentences(t *testing.T) {
	e := fixture(t, pipelineConfig(map[string][]string{
		settings.ContextMain: {"dict", "tr"},
	}))

	word := e.Run(context.Background(), Request{
		Context: settings.ContextMain, Mode: "input", Text: "serendipity."})
	if len(word.Dict) != 1 {
		t.Errorf("单词查询词典 %d 条, want 1（句末句号不该让单词变成句子）", len(word.Dict))
	}

	sentence := e.Run(context.Background(), Request{
		Context: settings.ContextMain, Mode: "input",
		Text: "The quick brown fox jumps over the lazy dog."})
	if len(sentence.Dict) != 0 {
		t.Errorf("整句查询词典 %d 条, want 0", len(sentence.Dict))
	}
	if len(sentence.Translate) != 1 {
		t.Errorf("整句译文 %d 条, want 1（只应跳过词典阶段）", len(sentence.Translate))
	}

	only := e.Run(context.Background(), Request{
		Context: settings.ContextMain, Mode: "input",
		Text: "The quick brown fox jumps over the lazy dog.", DictionaryOnly: true})
	if len(only.Dict) != 1 {
		t.Errorf("仅词典查询 %d 条, want 1（用户明确要的是词典）", len(only.Dict))
	}
}

func TestHeadwordGate(t *testing.T) {
	cases := []struct {
		text string
		want bool
	}{
		{"hello", true},
		{"  apple. ", true},
		{"as soon as possible", true},
		{"你好", true},
		{"hello, world", false},
		{"The quick brown fox jumps over the lazy dog.", false},
		{"", false},
		{"   ", false},
		{"第一句。第二句", false},
	}
	for _, c := range cases {
		if got := headword(c.text); got != c.want {
			t.Errorf("headword(%q) = %v, want %v", c.text, got, c.want)
		}
	}
}

// prompted answers with the instruction it was given, which is what 润色 and
// 总结 depend on: the same AI provider, a different instruction per request.
type prompted struct{ prompt string }

func (prompted) Label() string                                 { return "测试润色" }
func (p prompted) WithPrompt(prompt string) translate.Provider { return prompted{prompt} }
func (p prompted) Translate(_ context.Context, text, _, _ string) (string, string, error) {
	return p.prompt + ":" + text, "en", nil
}

func TestPromptRunsTheInstructionOnTheTranslator(t *testing.T) {
	cfg := pipelineConfig(map[string][]string{
		settings.ContextTranslation: {"polish"},
	})
	cfg.Translate = []settings.Provider{enabled("polish", settings.CatTranslate, "test-prompt")}
	e := fixture(t, cfg)

	res := e.Run(context.Background(), Request{
		Context: settings.ContextTranslation, Mode: "prompt", Text: "hello", Prompt: "润色"})
	if len(res.Translate) != 1 || res.Translate[0].Text != "润色:hello" {
		t.Fatalf("润色结果 = %+v, want 一条带指令的译文", res.Translate)
	}
}
