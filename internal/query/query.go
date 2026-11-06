// Package query turns one user intent — a selection, a screenshot, a typed
// sentence — into the results of every service that context selected. It knows
// nothing about windows or Wails, which is what makes the same pipeline usable
// from a hotkey, the tray, a URL scheme or a test.
package query

import (
	"context"
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
	TargetLang string `json:"targetLang"`
	ImageURL   string `json:"cropDataURL,omitempty"`

	OCR       []ocr.Outcome      `json:"ocr,omitempty"`
	Dict      []dict.Result      `json:"dict,omitempty"`
	Translate []translate.Result `json:"translate,omitempty"`

	ElapsedMs int64  `json:"elapsedMs"`
	Error     string `json:"error,omitempty"`

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
		if !p.Enabled {
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

// declares reports whether a window runs a stage at all. 迷你窗 shows a single
// line of translation and 鼠标取词 wants a dictionary entry, so letting every
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
		res.SourceText = PickText(res.OCR)
	} else {
		res.SourceText = strings.TrimSpace(req.Text)
	}
	if res.SourceText == "" {
		// An image that asked nothing of any engine is a settings question, and
		// annotate has the configuration to say so; 未识别到文字 would blame the
		// picture for work the app never did.
		if res.Error == "" && (len(req.Image) == 0 || len(res.OCR) > 0) {
			res.Error = "未识别到文字"
		}
		res.ElapsedMs = time.Since(start).Milliseconds()
		return res
	}
	// Easydict answers 中文 typed into a 中文 target with 英文, not by echoing the
	// input. Detection has to wait until OCR has produced the text, hence this
	// point rather than withLangs.
	if flip := samePairTarget(res.SourceText, req.SourceLang, req.TargetLang); flip != "" {
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
func samePairTarget(text, sourceLang, targetLang string) string {
	if sourceLang != langs.Auto {
		return ""
	}
	if targetLang != "zh-CN" && targetLang != "zh-TW" {
		return ""
	}
	if looksChinese(text) {
		return "en"
	}
	return ""
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
	return out
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
	return r
}

func (e *Engine) translateStage(ctx context.Context, cfg settings.Config, req Request, res *Response,
	start time.Time, onPart func(*Response)) []translate.Result {
	providers := plan(cfg, req.Context, settings.CatTranslate)
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
			out[i] = e.render(ctx, p, res.SourceText, req)
			if out[i].Text == "" && out[i].Error == "" {
				out[i].Error = "返回空译文"
			}
			mu.Lock()
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
			// Reserve the row so a later answer cannot push an already visible
			// card down when its own (earlier-ordered) service finishes.
			snap.Translate = append(snap.Translate, translate.Result{
				Provider: providers[i].ID,
				Label:    providers[i].Label,
				Pending:  true,
			})
			continue
		}
		snap.Translate = append(snap.Translate, out[i])
		if snap.Detected == "" && out[i].Detected != "" {
			snap.Detected = out[i].Detected
		}
	}
	return snap
}

func (e *Engine) render(ctx context.Context, p settings.Provider, text string, req Request) translate.Result {
	impl, err := translate.New(p)
	if err != nil {
		return translate.Result{Provider: p.ID, Label: p.Label, Error: err.Error()}
	}
	stage, cancel := stageCtx(ctx, netp.Timeout(textTimeout))
	defer cancel()
	r := translate.RunQuery(stage, p.ID, p.Label, impl, text, req.SourceLang, req.TargetLang)
	if strings.TrimSpace(req.Prompt) != "" {
		if instructed, ok := impl.(translate.Promptable); ok {
			r = translate.RunQuery(stage, p.ID, p.Label,
				instructed.WithPrompt(req.Prompt), text, req.SourceLang, req.TargetLang)
		}
	}
	if meta, ok := translate.Meta(p.Kind); ok {
		r.Markdown = meta.Markdown
		r.Site = meta.SiteURL(text, autoName(req.SourceLang), req.TargetLang)
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
