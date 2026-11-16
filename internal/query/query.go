// Package query turns one user intent — a selection, a screenshot, a typed
// sentence — into the results of every service that context selected. It knows
// nothing about windows or Wails, which is what makes the same pipeline usable
// from a hotkey, the tray, a URL scheme or a test.
package query

import (
	"context"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"transdict/internal/dict"
	"transdict/internal/langs"
	"transdict/internal/netp"
	"transdict/internal/ocr"
	"transdict/internal/settings"
	"transdict/internal/translate"
	"transdict/internal/tts"
)

// Built-in waits for one service. A screenshot legitimately takes longer than a
// sentence, so the stages differ; 服务超时 replaces both, because the same
// number has to bound each provider's own HTTP client too — otherwise the
// smaller of the two silently wins.
const (
	ocrTimeout  = 2 * time.Minute
	textTimeout = 3 * time.Minute
)

// Request is one query. Text and Image are alternatives: an image goes through
// OCR first, then follows the same path as a typed query.
type Request struct {
	Context    string `json:"context"`
	Mode       string `json:"mode"`
	Text       string `json:"text"`
	SourceLang string `json:"sourceLang"`
	TargetLang string `json:"targetLang"`
	// Prompt overrides the service's own instruction, which is how 润色 and
	// 总结 reuse the AI providers without a separate pipeline.
	Prompt string `json:"prompt,omitempty"`
	// DictionaryOnly keeps a word lookup from firing every translator, matching
	// Easydict's 词典模式.
	DictionaryOnly bool `json:"dictionaryOnly,omitempty"`
	// Service names one provider kind to run alone, which is how the local HTTP
	// API answers an explicit serviceType. It bypasses the window selection:
	// the caller asked for Google, not for whatever this window runs.
	Service string `json:"service,omitempty"`
	// Replaceable records that the query came from a live selection in an
	// editable field, so a result card may offer 替换 (EasyDict's
	// showReplaceButton). It is decided where the selection is read, because by
	// the time the answer lands the field may no longer be focused.
	Replaceable bool `json:"replaceable,omitempty"`

	Image []byte `json:"-"`
}

// Response is what every service returned, not only the winner, so the UI can
// show failures side by side with successes.
type Response struct {
	ID         string `json:"id"`
	Context    string `json:"context"`
	Mode       string `json:"mode"`
	SourceText string `json:"sourceText"`
	Detected   string `json:"detected"`
	// SourceLang is the language the answer was read as, when the query named
	// one — a picker choice or a 识别为 correction. Empty means the services
	// detected it, and 重试 may detect again.
	SourceLang string `json:"sourceLang,omitempty"`
	TargetLang string `json:"targetLang"`
	ImageURL   string `json:"cropDataURL,omitempty"`

	OCR       []ocr.Outcome      `json:"ocr,omitempty"`
	Dict      []dict.Result      `json:"dict,omitempty"`
	Translate []translate.Result `json:"translate,omitempty"`

	ElapsedMs int64  `json:"elapsedMs"`
	Error     string `json:"error,omitempty"`

	// Replaceable lets every result card offer 替换: clicking it puts that card's
	// answer back over the selection the query was made from.
	Replaceable bool `json:"replaceable,omitempty"`

	// Partial marks a first sketch sent as soon as the text exists — for a
	// screenshot, right after OCR. The translations arrive in the final
	// response; a partial one keeps the progress line and must not be recorded,
	// auto-copied or read aloud.
	Partial bool `json:"partial,omitempty"`

	// Skipped names the services the context asked for but could not run, so the
	// panel can point at a setting instead of leaving an empty column. It is a
	// list rather than a sentence because the words around the names differ per
	// language, and only the UI knows which language it is drawing.
	Skipped []string `json:"skipped,omitempty"`
}

// Engine reads configuration on every run, so a settings change takes effect on
// the next query rather than requiring a restart.
type Engine struct {
	cfg *settings.Store
	// stops holds the cancel of every service still streaming, keyed by the
	// window and the service id. 停止生成 cancels one card; the rest of the
	// query keeps answering, as EasyDict's stop button does.
	stops sync.Map // string -> *stopHandle
}

// stopHandle makes the registry entry comparable so a finished run cannot
// remove the handle a retried run has already stored under the same key.
type stopHandle struct{ cancel context.CancelFunc }

func stopKey(context, provider string) string { return context + "\x00" + provider }

// StopTranslate cancels the streaming answer of one service in one window. It
// reports whether that service was still streaming: a card that already landed
// has nothing left to stop.
func (e *Engine) StopTranslate(context, provider string) bool {
	h, ok := e.stops.Load(stopKey(context, provider))
	if !ok {
		return false
	}
	h.(*stopHandle).cancel()
	return true
}

func (e *Engine) armStop(context, provider string, handle *stopHandle) {
	e.stops.Store(stopKey(context, provider), handle)
}

func (e *Engine) disarmStop(context, provider string, handle *stopHandle) {
	e.stops.CompareAndDelete(stopKey(context, provider), handle)
}

func New(cfg *settings.Store) *Engine { return &Engine{cfg: cfg} }

func (e *Engine) Config() settings.Config { return e.cfg.Get() }

// Types lists the services a category offers, for the settings UI. The list
// comes from the provider packages' registrations, so it cannot drift.
func Types(category string) []settings.ServiceType {
	switch category {
	case settings.CatOCR:
		return ocr.Types()
	case settings.CatTranslate:
		return translate.Types()
	case settings.CatDict:
		return dict.Types()
	case settings.CatTTS:
		return tts.Types()
	}
	return nil
}

func MetaOf(category, kind string) (settings.ServiceType, bool) {
	for _, t := range Types(category) {
		if t.Kind == kind {
			return t, true
		}
	}
	return settings.ServiceType{}, false
}

// selection resolves a context's checked providers against the ids the config
// actually has. A file from an older build can name services that were renamed
// or never offered, and a selection that matches nothing has to mean the same
// thing as an empty one — "everything enabled" — rather than "run nothing".
func selection(cfg settings.Config, context string) []string {
	ids := cfg.Selected(context)
	if len(ids) == 0 {
		return nil
	}
	known := map[string]bool{}
	for _, category := range []string{settings.CatOCR, settings.CatTranslate, settings.CatDict, settings.CatTTS} {
		for _, p := range cfg.List(category) {
			known[p.ID] = true
		}
	}
	var out []string
	for _, id := range ids {
		if known[id] {
			out = append(out, id)
		}
	}
	return out
}

// ready reports whether a service has what it needs to answer — a key, a folder,
// a model.
func serviceReady(category string, p settings.Provider) bool {
	switch category {
	case settings.CatOCR:
		return ocr.Ready(p)
	case settings.CatTranslate:
		return translate.Ready(p)
	case settings.CatDict:
		return dict.Ready(p)
	case settings.CatTTS:
		return tts.Ready(p)
	}
	return false
}

// chosen lists what a window picked for one stage, before readiness is considered
// so plan and Skipped start from the same list: a hint that names a service the
// window never asked for blames the user for a setting they did not touch.
//
// The checked list belongs to the window and holds every stage it was ever shown,
// so it says nothing about a stage it does not carry. 截图 OCR ticks the recognisers
// and nothing else: reading that as "translate with nobody" would leave the panel
// with the original text and no translation, and 朗读 would find no voice on any
// window, because no context lists one. A stage the window expressed no preference
// about runs every enabled service of it, exactly as an empty list does.
func chosen(cfg settings.Config, context, category string) []settings.Provider {
	enabled := map[string]settings.Provider{}
	var all []settings.Provider
	for _, p := range cfg.List(category) {
		// 始终关闭 is EasyDict's usage status for a service the user keeps in the
		// list but never wants queried automatically. An explicit API call still
		// names its kind and bypasses this.
		if !p.Enabled || p.Get(settings.FieldUsageStatus) == settings.UsageAlwaysOff {
			continue
		}
		enabled[p.ID] = p
		all = append(all, p)
	}
	var picked []settings.Provider
	for _, id := range selection(cfg, context) {
		if p, ok := enabled[id]; ok {
			picked = append(picked, p)
			delete(enabled, id)
		}
	}
	if len(picked) > 0 {
		// 始终开启 joins the window's list even when this window never ticked
		// it, which is the point of the setting.
		seen := make(map[string]bool, len(picked))
		for _, p := range picked {
			seen[p.ID] = true
		}
		for _, p := range all {
			if p.Get(settings.FieldUsageStatus) == settings.UsageAlwaysOn && !seen[p.ID] {
				picked = append(picked, p)
			}
		}
		return picked
	}
	return all
}

// plan picks the providers a context runs. An explicit selection wins and sets
// the display order; otherwise every enabled and configured provider of that
// category runs.
func plan(cfg settings.Config, context, category string) []settings.Provider {
	var out []settings.Provider
	for _, p := range chosen(cfg, context, category) {
		if serviceReady(category, p) {
			out = append(out, p)
		}
	}
	return out
}

// pickKind is the one provider an API caller named. The caller asked for a
// specific service, so the per-window checkboxes do not apply; a kind that is
// missing or not configured answers nothing, which the HTTP layer reports as an
// unsupported service rather than as an empty translation.
func pickKind(cfg settings.Config, category, kind string) []settings.Provider {
	for _, p := range cfg.List(category) {
		if p.Kind != kind {
			continue
		}
		if p.Enabled && serviceReady(category, p) {
			return []settings.Provider{p}
		}
		return nil
	}
	return nil
}

// declares reports whether a window runs a stage at all. 翻译并替换 shows a
// single line of translation and 鼠标取词 wants a dictionary entry, so letting every
// context drive every stage spends requests on results the window cannot show —
// and a paragraph of OCR text is not a word a dictionary can look up.
func declares(context, category string) bool {
	for _, c := range settings.CategoryOf(context) {
		if c == category {
			return true
		}
	}
	return false
}

// Skipped names the services a context asked for but cannot run, so the UI can
// explain why a result is missing rather than showing nothing.
func Skipped(cfg settings.Config, context, category string) []string {
	var names []string
	for _, p := range chosen(cfg, context, category) {
		// A kind this build never registered has no key for anyone to enter, so
		// naming it would blame the user for a service we do not ship here.
		if _, ok := MetaOf(category, p.Kind); !ok {
			continue
		}
		if !serviceReady(category, p) {
			names = append(names, p.Label)
		}
	}
	return names
}

// Prune drops providers whose kind no package registered in this build, and the
// per-context checked-list entries that named them. Such rows survive in a file
// written by another platform or an older version, and one that can never turn
// ready is worse than absent: 服务设置 would show a switch that does nothing.
func Prune(c *settings.Config) int {
	var gone []string
	keep := func(list []settings.Provider, category string) []settings.Provider {
		out := list[:0]
		for _, p := range list {
			if _, ok := MetaOf(category, p.Kind); ok {
				out = append(out, p)
				continue
			}
			gone = append(gone, p.ID)
		}
		return out
	}
	c.OCR = keep(c.OCR, settings.CatOCR)
	c.Translate = keep(c.Translate, settings.CatTranslate)
	c.Dict = keep(c.Dict, settings.CatDict)
	c.TTS = keep(c.TTS, settings.CatTTS)
	if len(gone) == 0 {
		return 0
	}
	dropped := map[string]bool{}
	for _, id := range gone {
		dropped[id] = true
	}
	for context, ids := range c.Selections {
		if len(ids) == 0 {
			continue
		}
		kept := ids[:0]
		for _, id := range ids {
			if !dropped[id] {
				kept = append(kept, id)
			}
		}
		c.Selections[context] = kept
	}
	return len(gone)
}

// Run executes a whole request. It never returns nil: a failure is data the
// panel shows.
func (e *Engine) Run(ctx context.Context, req Request) *Response {
	return e.run(ctx, req, nil)
}

// RunPartial is Run with a progress callback: when a request carries an image,
// the response built from OCR is handed over before the translation and
// dictionary stages start. The caller shows it immediately, which is what makes
// 截图翻译 feel like one gesture rather than a wait for every service.
func (e *Engine) RunPartial(ctx context.Context, req Request, onText func(*Response)) *Response {
	return e.run(ctx, req, onText)
}

func (e *Engine) run(ctx context.Context, req Request, onText func(*Response)) *Response {
	start := time.Now()
	cfg := e.cfg.Get()
	req = withLangs(req, cfg)

	res := &Response{ID: newID(), Context: req.Context, Mode: req.Mode,
		TargetLang: req.TargetLang, ImageURL: DataURL(req.Image)}

	if len(req.Image) > 0 {
		res.OCR = e.OCRStage(ctx, cfg, req)
		res.SourceText = OCRText(cfg, res.OCR)
	} else {
		res.SourceText = strings.TrimSpace(req.Text)
	}
	if res.SourceText == "" {
		// An image that asked nothing of any engine is a settings question, and
		// annotate has the configuration to say so; the empty-result line would
		// blame the picture for work the app never did. A recognized nothing is
		// answered with EasyDict's guidance: the usual fix is picking the
		// recognition language by hand.
		if res.Error == "" && len(req.Image) == 0 {
			res.Error = "未识别到文字"
		} else if res.Error == "" && len(res.OCR) > 0 {
			res.Error = "OCR 结果为空。⚠️ 请手动选择识别语言再次尝试。"
		}
		res.ElapsedMs = time.Since(start).Milliseconds()
		return res
	}
	// A recognized link is its own answer: EasyDict returns early with the URL
	// in the box, highlighted and clickable, instead of translating it into
	// gibberish. Only OCR does this — a URL typed into the box is still the
	// user's question.
	if len(req.Image) > 0 && WholeURL(res.SourceText) {
		res.ElapsedMs = time.Since(start).Milliseconds()
		return res
	}
	// Easydict answers 中文 typed into a 中文 target with 英文, not by echoing the
	// input. Detection has to wait until OCR has produced the text, hence this
	// point rather than withLangs.
	detected := detectedText(cfg, res.SourceText)
	if req.TargetLang == langs.Auto || req.SourceLang == langs.Auto {
		if better, ok := e.optimizeDetection(ctx, cfg, res.SourceText); ok {
			detected = better
			res.Detected = better
		}
	}
	if req.TargetLang == langs.Auto {
		// 自动选择 is resolved from the text: the pair the app is for is
		// Chinese↔English, and everything else answers in Chinese.
		req.TargetLang = autoTargetDetected(cfg, detected)
		res.TargetLang = req.TargetLang
	} else if flip := samePairTargetDetected(cfg, detected, req.SourceLang, req.TargetLang); flip != "" {
		req.TargetLang = flip
		res.TargetLang = flip
	}

	if onText != nil && len(req.Image) > 0 {
		partial := *res
		partial.Partial = true
		partial.Translate = nil
		partial.Dict = nil
		partial.ElapsedMs = time.Since(start).Milliseconds()
		onText(&partial)
	}

	if len(req.Image) > 0 && !cfg.AutoQueryOCR {
		// 图片 OCR 后自动查询 off: recognition is the whole answer in the panel,
		// and 全部重新查询 turns it into a translation when the user is ready.
		res.ElapsedMs = time.Since(start).Milliseconds()
		return res
	}

	// The two stages are independent: waiting for the slowest translator before
	// even starting the dictionary made a word lookup feel as slow as a
	// paragraph translation. They fill different fields of the answer.
	var stages sync.WaitGroup
	if !req.DictionaryOnly && declares(req.Context, settings.CatTranslate) {
		stages.Add(1)
		go func() {
			defer stages.Done()
			res.Translate = e.translateStage(ctx, cfg, req, res, start, onText)
		}()
	}
	// A sentence is not a headword: 有道 answers one with 网络释义 for the whole
	// sentence and Apple with 未收录, which reads as three failures under a
	// translation that was already right. 仅词典 skips the gate because there the
	// dictionary is exactly what was asked for.
	if declares(req.Context, settings.CatDict) && (req.DictionaryOnly || headword(res.SourceText)) {
		stages.Add(1)
		go func() {
			defer stages.Done()
			res.Dict = e.dictStage(ctx, cfg, req, res)
		}()
	}
	stages.Wait()
	res.ElapsedMs = time.Since(start).Milliseconds()
	return res
}

func withLangs(req Request, cfg settings.Config) Request {
	if req.SourceLang == "" {
		req.SourceLang = cfg.SourceLang
	}
	if req.TargetLang == "" {
		req.TargetLang = cfg.TargetLang
	}
	if req.SourceLang == "" {
		req.SourceLang = langs.Auto
	}
	if req.TargetLang == "" {
		req.TargetLang = "zh-CN"
	}
	return req
}

// samePairTarget mirrors Easydict's 中英 pair: with 自动检测 as the source and
// Chinese as the target, Chinese text translates to English rather than coming
// back unchanged (Google echoes the input when sl and tl are the same). Only
// Han without kana counts as Chinese, so Japanese keeps its 中文 target.
// samePairTarget catches the half of the pair the user already typed in: a
// 中文 target on 中文 text would translate a language into itself, so the
// answer flips to the other side of 第一语言/第二语言.
func samePairTarget(cfg settings.Config, text, sourceLang, targetLang string) string {
	return samePairTargetDetected(cfg, detectedText(cfg, text), sourceLang, targetLang)
}

// samePairTargetDetected is the same question with the language already named,
// which is how 语种识别 optimization feeds a service's answer in.
func samePairTargetDetected(cfg settings.Config, detected, sourceLang, targetLang string) string {
	if sourceLang != langs.Auto {
		return ""
	}
	first, second := userPair(cfg)
	if !sameLanguage(targetLang, second) {
		return ""
	}
	if sameLanguage(detected, second) {
		return first
	}
	return ""
}

// autoTarget resolves 自动选择 from the pair the user reads: text already in
// 第一语言 answers in 第二语言, and everything else answers in 第一语言. The
// defaults (English and 简体中文) reproduce Easydict's Chinese↔English swap.
func autoTarget(cfg settings.Config, text string) string {
	return autoTargetDetected(cfg, detectedText(cfg, text))
}

// autoTargetDetected resolves 自动选择 from an already named language.
func autoTargetDetected(cfg settings.Config, detected string) string {
	first, second := userPair(cfg)
	if first == second {
		return first
	}
	if sameLanguage(detected, first) {
		return second
	}
	return first
}

// detectedText is what the system recogniser and the reader's pair decide on
// their own, before 语种识别 optimization has a say.
func detectedText(cfg settings.Config, text string) string {
	first, second := userPair(cfg)
	return langs.DetectWithHints(text, first, second)
}

// userPair reads 第一语言/第二语言, filling in the app's usual pair when the
// settings predate them or a field was left blank.
func userPair(cfg settings.Config) (string, string) {
	first := strings.TrimSpace(cfg.FirstLang)
	if first == "" {
		first = "en"
	}
	second := strings.TrimSpace(cfg.SecondLang)
	if second == "" {
		second = "zh-CN"
	}
	return first, second
}

// sameLanguage treats the two Chinese scripts as one language: 繁体 text must
// not decide that a 简体 target is already the language it was written in.
func sameLanguage(a, b string) bool {
	zh := func(c string) bool { return c == "zh-CN" || c == "zh-TW" }
	return a == b || (zh(a) && zh(b))
}

// A Chinese name inside an English paragraph is not a Chinese paragraph: OCR
// output routinely mixes "Submitted by: 全勇陈" into English, and counting any
// single Han character flipped those queries into English→English echoes. Han
// therefore has to carry a real share of the letters before the text counts as
// Chinese.
func looksChinese(s string) bool {
	han, other, kana, hangul := 0, 0, 0, 0
	for _, r := range s {
		switch {
		case unicode.Is(unicode.Han, r):
			han++
		case unicode.In(r, unicode.Hiragana, unicode.Katakana):
			kana++
		case unicode.Is(unicode.Hangul, r):
			hangul++
		case unicode.IsLetter(r):
			other++
		}
	}
	return han > 0 && kana == 0 && hangul == 0 && han*5 >= han+other
}

func (e *Engine) OCRStage(ctx context.Context, cfg settings.Config, req Request) []ocr.Outcome {
	providers := plan(cfg, req.Context, settings.CatOCR)
	out := make([]ocr.Outcome, len(providers))
	var wg sync.WaitGroup
	for i, p := range providers {
		wg.Add(1)
		go func(i int, p settings.Provider) {
			defer wg.Done()
			out[i] = e.recognize(ctx, p, req.Image, req.SourceLang)
		}(i, p)
	}
	wg.Wait()
	return withYoudaoFallback(ctx, cfg, out, req.Image)
}

// youdaoFallbackOCR is the keyless Youdao image call behind 有道 OCR 识别.
// A var so tests can answer the request without the network.
var youdaoFallbackOCR = ocr.YoudaoWebOCR

// withYoudaoFallback is Easydict's 有道 OCR 识别: when none of the configured
// services read a character, the picture gets one more try through Youdao's
// public endpoint. The outcome joins the others, so the OCR card still shows
// which service supplied the text — and when the fallback fails too, every
// provider's error stays visible.
func withYoudaoFallback(ctx context.Context, cfg settings.Config, out []ocr.Outcome, img []byte) []ocr.Outcome {
	if !cfg.EnableYoudaoOCR || len(img) == 0 || PickText(out) != "" {
		return out
	}
	stage, cancel := stageCtx(ctx, netp.Timeout(ocrTimeout))
	defer cancel()
	start := time.Now()
	blocks, err := youdaoFallbackOCR(stage, img)
	res := ocr.Outcome{Provider: "youdao-ocr", Label: "有道 OCR", Blocks: blocks,
		ElapsedMs: time.Since(start).Milliseconds()}
	if err != nil {
		res.Blocks = nil
		res.Error = netp.Reason(err)
	}
	return append(out, res)
}

// recognize builds and calls one OCR provider. Exported through RetryOCR so a
// single service can be re-run without spending money on the others again.
// sourceLang is a hint, not a constraint: 自动检测 leaves it empty so an engine
// that can detect languages runs its full model set.
func (e *Engine) recognize(ctx context.Context, p settings.Provider, img []byte, sourceLang string) ocr.Outcome {
	impl, err := ocr.New(p)
	if err != nil {
		return ocr.Outcome{Provider: p.ID, Label: p.Label, Error: err.Error()}
	}
	stage, cancel := stageCtx(ctx, netp.Timeout(ocrTimeout))
	defer cancel()
	stage = ocr.ContextWithLang(stage, sourceLang)
	return ocr.RunQuery(stage, p.ID, p.Label, impl, img)
}

// A dictionary entry is a word or a short phrase. Past these bounds the text is
// a sentence, and asking a dictionary about it spends a round trip on an answer
// no card can use.
const (
	headwordMaxRunes = 24
	headwordMaxWords = 4
)

// headword reports whether a query still looks like something a dictionary
// could hold. Trailing sentence punctuation is trimmed first — a copied word
// often carries the period that ended the sentence around it.
func headword(text string) bool {
	text = strings.TrimSpace(text)
	text = strings.TrimRight(text, ".。!！?？")
	if text == "" || utf8.RuneCountInString(text) > headwordMaxRunes {
		return false
	}
	// Internal punctuation means the text was assembled into a sentence.
	if strings.ContainsAny(text, "，,；;：:、。！？!?…\n\r\t") {
		return false
	}
	return len(strings.Fields(text)) <= headwordMaxWords
}

func (e *Engine) dictStage(ctx context.Context, cfg settings.Config, req Request, res *Response) []dict.Result {
	providers := plan(cfg, req.Context, settings.CatDict)
	if req.Service != "" {
		providers = pickKind(cfg, settings.CatDict, req.Service)
	}
	out := make([]dict.Result, len(providers))
	var wg sync.WaitGroup
	for i, p := range providers {
		wg.Add(1)
		go func(i int, p settings.Provider) {
			defer wg.Done()
			out[i] = e.explain(ctx, p, res.SourceText, req.SourceLang, req.TargetLang)
		}(i, p)
	}
	wg.Wait()
	return out
}

func (e *Engine) explain(ctx context.Context, p settings.Provider, text, src, dst string) dict.Result {
	impl, err := dict.New(p)
	if err != nil {
		return dict.Result{Provider: p.ID, Label: p.Label, Error: err.Error()}
	}
	stage, cancel := stageCtx(ctx, netp.Timeout(textTimeout))
	defer cancel()
	r := dict.RunQuery(stage, p.ID, p.Label, impl, text, src, dst)
	if meta, ok := dict.Meta(p.Kind); ok {
		r.Site = meta.SiteURL(text, src, dst)
	}
	r.OpenURL = dictOpenURL(p.Kind, text)
	return r
}

// dictOpenURL is where a local dictionary looks a word up. Apple's dictionary
// registers dict://, which EasyDict uses for its 在苹果词典中打开 link; the web
// dictionaries already declare a page in Site.
func dictOpenURL(kind, text string) string {
	if kind != "apple" {
		return ""
	}
	return "dict://" + url.PathEscape(strings.TrimSpace(text))
}

func (e *Engine) translateStage(ctx context.Context, cfg settings.Config, req Request, res *Response,
	start time.Time, onPart func(*Response)) []translate.Result {
	providers := plan(cfg, req.Context, settings.CatTranslate)
	if req.Service != "" {
		providers = pickKind(cfg, settings.CatTranslate, req.Service)
	}
	// A service with no route for this pair is left out rather than drawn as a
	// red card: 有道 does not translate 日语 into 韩语, and saying so beside two
	// services that answered only adds noise. A construction failure still shows
	// up, because that is a configuration problem the user can fix.
	kept := providers[:0]
	for _, p := range providers {
		if impl, err := translate.New(p); err == nil {
			if checker, ok := impl.(translate.PairChecker); ok &&
				!checker.SupportsPair(req.SourceLang, req.TargetLang) {
				continue
			}
			// 翻译/句子分析/查单词 switches bound what an AI service answers; a
			// query none of them covers should leave it out, not fail it.
			if typer, ok := impl.(translate.QueryTyper); ok &&
				!typer.SupportsQuery(res.SourceText, req.SourceLang, req.TargetLang) {
				continue
			}
		}
		kept = append(kept, p)
	}
	providers = kept
	out := make([]translate.Result, len(providers))
	done := make([]bool, len(providers))
	var mu sync.Mutex
	var wg sync.WaitGroup
	for i, p := range providers {
		wg.Add(1)
		go func(i int, p settings.Provider) {
			defer wg.Done()
			// Each service gets its own cancel so 停止生成 can end one card
			// without cutting the answer the others are still producing.
			pctx, pcancel := context.WithCancel(ctx)
			defer pcancel()
			handle := &stopHandle{cancel: pcancel}
			e.armStop(req.Context, p.ID, handle)
			defer e.disarmStop(req.Context, p.ID, handle)
			var lastEmit time.Time
			// A streamed answer updates the card it will end up in, without
			// marking that service done: the progress line and 重试 state still
			// belong to a service that has not finished. Emitting is throttled
			// because SSE delivers tokens, and a snapshot per token would spend
			// the whole window redrawing.
			emit := func(partial string) {
				mu.Lock()
				out[i].Provider, out[i].Label = p.ID, p.Label
				out[i].Text = partial
				out[i].ElapsedMs = time.Since(start).Milliseconds()
				if onPart != nil && time.Since(lastEmit) >= 80*time.Millisecond {
					lastEmit = time.Now()
					onPart(translationSnapshot(res, providers, out, done, start))
				}
				mu.Unlock()
			}
			r := e.render(pctx, p, res.SourceText, req, cfg.FirstLang, emit)
			// 停止生成 is not a failure: keep the words that already reached
			// the card and let the service end as if it had finished there.
			// The partial text lives in out under the lock the emit path uses.
			if pctx.Err() == context.Canceled {
				mu.Lock()
				r.Text = out[i].Text
				mu.Unlock()
				r.Error = ""
				r.ElapsedMs = time.Since(start).Milliseconds()
			}
			finishAnswer(&r)
			mu.Lock()
			out[i] = r
			done[i] = true
			// A slow service must not hold the others hostage: each answer is
			// sent as soon as it lands, in provider order so a card never moves.
			// The lock is held across the callback so an older snapshot cannot
			// arrive after a newer one.
			if onPart != nil {
				onPart(translationSnapshot(res, providers, out, done, start))
			}
			mu.Unlock()
		}(i, p)
	}
	wg.Wait()

	for i := range out {
		if res.Detected == "" && out[i].Detected != "" {
			res.Detected = out[i].Detected
		}
	}
	return out
}

// WholeURL reports whether the text is nothing but one web link, which is
// EasyDict's isURL check on recognized text: a scheme is required, so
// "example.com" typed by hand is still a question.
func WholeURL(text string) bool {
	u, err := url.Parse(strings.TrimSpace(text))
	if err != nil {
		return false
	}
	return (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

// finishAnswer classifies an empty translation. A service that replied with
// nothing is EasyDict's noResult warning — but only for one that cannot stream:
// a streamer that stayed silent is a fault, keeps the red mark and 重试. A
// transport failure is always a fault and keeps the button.
func finishAnswer(r *translate.Result) {
	if r.Text == "" && r.Error == "" {
		r.Error = "返回空译文"
	}
	r.Warning = r.Text == "" && !r.Stream && noResultError(r.Error)
}

func noResultError(msg string) bool {
	return strings.Contains(msg, "空译文") || strings.Contains(msg, "没有输出译文")
}

// translationSnapshot carries the answers that are already in, without the
// fields the dictionary stage may still be writing. A partial response is
// replaced by the next one, so it must never be recorded as the final answer.
func translationSnapshot(res *Response, providers []settings.Provider, out []translate.Result, done []bool, start time.Time) *Response {
	snap := &Response{
		ID:         res.ID,
		Context:    res.Context,
		Mode:       res.Mode,
		SourceText: res.SourceText,
		Detected:   res.Detected,
		TargetLang: res.TargetLang,
		ImageURL:   res.ImageURL,
		OCR:        res.OCR,
		Error:      res.Error,
		ElapsedMs:  time.Since(start).Milliseconds(),
		Partial:    true,
	}
	for i := range out {
		if !done[i] {
			// A streamed answer is already readable, so its row keeps the text
			// instead of the loading dots. A service that has said nothing yet
			// still reserves its row so a later answer cannot push a visible card
			// down when an earlier-ordered service finishes.
			if !done[i] && strings.TrimSpace(out[i].Text) != "" {
				row := out[i]
				row.Streaming = true
				snap.Translate = append(snap.Translate, row)
				continue
			}
			row := translate.Result{
				Provider: providers[i].ID,
				Label:    providers[i].Label,
				Pending:  true,
			}
			// The model menu is part of the card from the first frame: a service
			// that is still thinking already has a chosen model.
			row.Model, row.Models = translate.ModelInfo(providers[i])
			snap.Translate = append(snap.Translate, row)
			continue
		}
		snap.Translate = append(snap.Translate, out[i])
		if snap.Detected == "" && out[i].Detected != "" {
			snap.Detected = out[i].Detected
		}
	}
	return snap
}

func (e *Engine) render(ctx context.Context, p settings.Provider, text string, req Request,
	firstLang string, onText func(string)) translate.Result {
	impl, err := translate.New(p)
	if err != nil {
		return translate.Result{Provider: p.ID, Label: p.Label, Error: err.Error()}
	}
	// A custom prompt may say ${{firstLanguage}}, which lives in the app config
	// rather than on the service row.
	if aware, ok := impl.(translate.FirstLanguage); ok {
		impl = aware.WithFirstLanguage(firstLang)
	}
	stage, cancel := stageCtx(ctx, netp.Timeout(textTimeout))
	defer cancel()
	r := translate.RunQueryStream(stage, p.ID, p.Label, impl, text, req.SourceLang, req.TargetLang, onText)
	if strings.TrimSpace(req.Prompt) != "" {
		if instructed, ok := impl.(translate.Promptable); ok {
			r = translate.RunQueryStream(stage, p.ID, p.Label,
				instructed.WithPrompt(req.Prompt), text, req.SourceLang, req.TargetLang, onText)
		}
	}
	if meta, ok := translate.Meta(p.Kind); ok {
		r.Markdown = meta.Markdown
		r.Site = meta.SiteURL(text, autoName(req.SourceLang), req.TargetLang)
	}
	r.Model, r.Models = translate.ModelInfo(p)
	// 参见 👉 belongs to a refused key or a spent quota: EasyDict's promptURL
	// is the page that fixes exactly that, and no other failure has one.
	if r.Error != "" {
		base := p.BaseURL
		if base == "" {
			if meta, ok := translate.Meta(p.Kind); ok {
				base = meta.Default.BaseURL
			}
		}
		r.PromptTitle, r.PromptURL = translate.PromptFor(p.Kind, base, r.Error)
	}
	return r
}

// autoName is how a service URL says "guess the language": an empty sl= parameter
// is a broken link, not a guess, so the word the site itself uses goes in.
func autoName(code string) string {
	if strings.TrimSpace(code) == "" {
		return "auto"
	}
	return code
}

// FirstTranslation is what 翻译并替换 types back into the source field: the
// earliest service that answered, in the order the panel shows them.
func FirstTranslation(res *Response) string {
	if res == nil {
		return ""
	}
	for _, t := range res.Translate {
		if t.Error == "" && strings.TrimSpace(t.Text) != "" {
			return t.Text
		}
	}
	return ""
}

// Speak reads text aloud, walking the ready TTS providers in order until one
// produces audio. A voice that only pronounces dictionary entries, or whose
// upstream rotated away, must not mute a window that has another service
// configured; when every voice fails the first one's error is the one shown,
// because it is the one the user asked for.
func (e *Engine) Speak(text, lang string, slow bool) tts.Result {
	cfg := e.cfg.Get()
	providers := plan(cfg, settings.ContextMain, settings.CatTTS)
	providers = defaultVoice(providers, cfg.DefaultTTS)
	if cfg.PreferYoudaoWord {
		providers = preferYoudaoWord(providers, text, lang)
	}
	if len(providers) == 0 {
		return tts.Result{Error: "没有可用的朗读服务"}
	}
	ctx, cancel := stageCtx(context.Background(), netp.Timeout(textTimeout))
	defer cancel()

	var first *tts.Result
	for _, p := range providers {
		impl, err := tts.New(p)
		if err != nil {
			continue
		}
		res := tts.RunQuery(ctx, p.ID, p.Label, impl, text, lang, slow)
		if res.Error == "" {
			return res
		}
		if first == nil {
			r := res
			first = &r
		}
	}
	if first != nil {
		return *first
	}
	return tts.Result{Error: "没有可用的朗读服务"}
}

// defaultVoice is 默认 TTS 服务: the chosen provider is tried first, and the rest
// keep their configured order so a broken voice still falls back.
func defaultVoice(providers []settings.Provider, id string) []settings.Provider {
	if id == "" {
		return providers
	}
	out := make([]settings.Provider, 0, len(providers))
	for _, p := range providers {
		if p.ID == id {
			out = append(out, p)
		}
	}
	for _, p := range providers {
		if p.ID != id {
			out = append(out, p)
		}
	}
	return out
}

// preferYoudaoWord is 英语单词优先使用有道发音: 有道's dictionary recording reads a
// single English word better than a generic voice, so for that one case it goes
// first. A sentence, a CJK word or a missing 有道 service leaves the order alone.
func preferYoudaoWord(providers []settings.Provider, text, lang string) []settings.Provider {
	if !strings.HasPrefix(lang, "en") || !singleWord(text) {
		return providers
	}
	for i, p := range providers {
		if p.Kind != settings.ProviderKind("youdao") {
			continue
		}
		moved := make([]settings.Provider, 0, len(providers))
		moved = append(moved, p)
		moved = append(moved, providers[:i]...)
		moved = append(moved, providers[i+1:]...)
		return moved
	}
	return providers
}

func singleWord(text string) bool {
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}
	for _, r := range text {
		if unicode.IsSpace(r) || (!unicode.IsLetter(r) && r != '-' && r != '\'') {
			return false
		}
	}
	return true
}

func stageCtx(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	return context.WithTimeout(parent, timeout)
}

// PickText prefers the outcome with the most recognized characters, which in
// practice means the vision model wins over classical OCR on dense screenshots.
func PickText(outcomes []ocr.Outcome) string {
	best := ""
	for _, o := range outcomes {
		if o.Error != "" {
			continue
		}
		if t := o.Text(); len([]rune(t)) > len([]rune(best)) {
			best = t
		}
	}
	return strings.TrimSpace(best)
}

// OCRText is the text a screenshot produced, after 启用 OCR 文本规范化 if it is
// on. The recogniser's own blocks stay untouched so the OCR section can still
// show what each service read.
func OCRText(cfg settings.Config, outcomes []ocr.Outcome) string {
	text := PickText(outcomes)
	if cfg.OCRTextNormalize {
		return ocr.NormalizeText(text)
	}
	return text
}
