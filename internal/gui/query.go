package gui

import (
	"context"
	"errors"
	"strings"

	"transdict/internal/history"
	"transdict/internal/l10n"
	"transdict/internal/query"
	"transdict/internal/settings"
	"transdict/internal/translate"
	"transdict/internal/tts"
)

// Ask is the single entry point every query mode funnels through: a hotkey, the
// tray, the clipboard, the input box or a URL scheme all build a Request and get
// one Response back on the event channel.
func (s *Service) Ask(req query.Request) error {
	text := strings.TrimSpace(req.Text)
	if text == "" && len(req.Image) == 0 {
		return errors.New("没有内容可查询")
	}
	req.Text = text
	if req.Context == "" {
		req.Context = settings.ContextMain
	}
	if req.Mode == "" {
		req.Mode = req.Context
	}
	switch target(req.Context) {
	case WindowFloat:
		w, h := s.floatSize()
		s.ensure(WindowFloat, "/#/float", w, h, true)
		s.placeFloat(w, h)
	case WindowMini:
		s.ensure(WindowMini, "/#/mini", 380, 110, true)
	}
	// A query replaces whatever this window was still waiting on: the earlier
	// request is cancelled and its ticket stops matching, so a slow answer cannot
	// land after the newer one and take over the window, the history row or the
	// clipboard.
	runCtx, ticket, cancel := s.beginRun(req.Context)
	go func() {
		defer cancel()
		s.execute(runCtx, req, ticket)
	}()
	return nil
}

// QueryText runs text the caller already has: the input window and the
// dictionary lookup both arrive here.
func (s *Service) QueryText(text, context string) error {
	return s.Ask(query.Request{Text: text, Context: context, Mode: "input"})
}

// MiniQuery answers in the always-on-top mini window.
func (s *Service) MiniQuery(text string) error {
	return s.Ask(query.Request{Text: text, Context: settings.ContextMini, Mode: "mini"})
}

// ClipboardQuery reads the pasteboard and translates it — 剪贴板翻译, which needs
// no accessibility permission and therefore works in every application.
func (s *Service) ClipboardQuery() error {
	text := s.clipboardText()
	if text == "" {
		return errors.New("剪贴板为空")
	}
	return s.Ask(query.Request{Text: text, Context: settings.ContextClipboard, Mode: "clipboard"})
}

// ClipboardMini feeds the pasteboard to the mini window, which is the always-on
// reading companion: copy a sentence in any app, hit the shortcut, see the
// translation in a two-line panel pinned to the corner.
func (s *Service) ClipboardMini() error {
	text := s.clipboardText()
	if text == "" {
		return errors.New("剪贴板为空")
	}
	return s.MiniQuery(text)
}

func (s *Service) clipboardText() string {
	text, ok := s.wails.Clipboard.Text()
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
}

// Prompt runs an ad-hoc instruction (润色 / 总结 / 语气改写) over the AI-capable
// services of the current selection while keeping the source text on screen.
func (s *Service) Prompt(text, instruction string) error {
	if strings.TrimSpace(text) == "" {
		return errors.New("没有内容")
	}
	return s.Ask(promptRequest(text, instruction))
}

// promptRequest is the query behind 润色 and 总结. It must not be a
// DictionaryOnly request: that flag skips the translation stage, which is the
// only stage an instruction can run on, and the panel would answer "没有可用的
// 服务" for every chip. CategoryOf(translation) already keeps dictionaries out.
func promptRequest(text, instruction string) query.Request {
	return query.Request{Text: text, Context: settings.ContextTranslation,
		Mode: "prompt", Prompt: instruction}
}

// beginRun starts a new query for one context. Whatever that context was still
// running is cancelled, and the ticket it holds stops matching currentRun.
func (s *Service) beginRun(scope string) (context.Context, uint64, context.CancelFunc) {
	runCtx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.runs == nil {
		s.runs = map[string]uint64{}
		s.runCancel = map[string]context.CancelFunc{}
	}
	if prev := s.runCancel[scope]; prev != nil {
		prev()
	}
	s.runCancel[scope] = cancel
	s.runs[scope]++
	return runCtx, s.runs[scope], cancel
}

// currentRun reports whether the query holding ticket is still the one the
// window is waiting for.
func (s *Service) currentRun(scope string, ticket uint64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.runs[scope] == ticket
}

func (s *Service) execute(runCtx context.Context, req query.Request, ticket uint64) {
	stale := func() bool { return !s.currentRun(req.Context, ticket) }
	if stale() {
		return
	}
	cfg := s.cfg.Get()
	if req.SourceLang == "" {
		req.SourceLang = cfg.SourceLang
	}
	if req.TargetLang == "" {
		req.TargetLang = cfg.TargetLang
	}

	s.busy(req.Context, stageLabel(req))
	// A screenshot opens the panel on the crop itself. Waiting for the first
	// partial — OCR text — left the screen unchanged for as long as recognition
	// took, which is what made 截图翻译 feel slower than the gesture it is.
	if len(req.Image) > 0 {
		s.emitPartial(&query.Response{Context: req.Context, Mode: req.Mode,
			TargetLang: req.TargetLang, ImageURL: query.DataURL(req.Image), Partial: true})
	}
	// The text is shown the moment OCR finishes; the translations replace this
	// sketch when they land. annotate is skipped on purpose: its "no service"
	// verdict would fire while the translation stage has not even started.
	res := s.engine.RunPartial(runCtx, req, func(partial *query.Response) {
		if stale() {
			return
		}
		partial.Context, partial.Mode = req.Context, req.Mode
		s.emitPartial(partial)
		s.busy(req.Context, "翻译中…")
	})
	if stale() {
		return
	}
	res.Context = req.Context
	res.Mode = req.Mode
	s.annotate(res, cfg, req)
	s.emit(res)
}

func stageLabel(req query.Request) string {
	switch {
	case len(req.Image) > 0:
		return "识别文字中…"
	case req.Prompt != "":
		return "AI 处理中…"
	default:
		return "查询中…"
	}
}

// annotate turns "nothing ran" into a reason the user can act on, because an
// empty panel otherwise reads as a bug.
func (s *Service) annotate(res *query.Response, cfg settings.Config, req query.Request) {
	if res.SourceText == "" {
		// A screenshot that reached no engine at all is a settings question: say
		// which service the window asked for and leave the page as the next step.
		if res.Error == "" && len(req.Image) > 0 {
			res.Error = "没有可用的文字识别服务"
			res.Skipped = query.Skipped(cfg, req.Context, settings.CatOCR)
		}
		return
	}
	if res.Error != "" {
		return
	}
	hidden := false
	if cfg.HideSameLang && len(res.Translate) > 0 {
		kept := dropUnchanged(res.Translate, res.SourceText)
		hidden = len(kept) < len(res.Translate)
		res.Translate = kept
	}
	if len(res.Translate) == 0 && len(res.Dict) == 0 {
		if hidden {
			res.Error = "原文已是目标语言"
			return
		}
		res.Error = "没有可用的服务"
		res.Skipped = query.Skipped(cfg, req.Context, settings.CatTranslate)
		return
	}
	if len(res.Translate) == 0 && !req.DictionaryOnly {
		res.Error = "没有可用的翻译服务，请在设置中勾选"
	}
}

// dropUnchanged removes answers that repeat the input. A machine translator
// echoes the text when it is already in the target language, and 隐藏与原文相同的
// 译文 exists so a French sentence does not come back as five copies of itself.
func dropUnchanged(items []translate.Result, source string) []translate.Result {
	kept := make([]translate.Result, 0, len(items))
	for _, t := range items {
		if t.Error == "" && sameText(t.Text, source) {
			continue
		}
		kept = append(kept, t)
	}
	return kept
}

func sameText(a, b string) bool {
	return strings.Join(strings.Fields(a), " ") == strings.Join(strings.Fields(b), " ")
}

// LastResult re-sends the previous response so a window opened after the query —
// the floating and mini panels are lazy — can render what already happened. It is
// a fresh draw, so it words itself in the language the window will show.
func (s *Service) LastResult() *query.Response {
	res := s.snapshot()
	if res == nil {
		return nil
	}
	next := localizedResponse(*res)
	return &next
}

// PanelLast is LastResult for the main window alone. The panel must not adopt a
// 划词 answer just because it was the newest one; it shows 详情's promoted copy
// or the last query that really ran there.
func (s *Service) PanelLast() *query.Response {
	s.mu.Lock()
	res := s.panelLast
	s.mu.Unlock()
	if res == nil {
		return nil
	}
	next := localizedResponse(*res)
	return &next
}

// RetryOCR re-runs one recognition service on the stored crop, then rebuilds the
// rest of the response from the text it produced.
func (s *Service) RetryOCR(providerID string) error {
	s.mu.Lock()
	crop := s.lastCrop
	s.mu.Unlock()
	if len(crop) == 0 {
		return errors.New("截图已过期，请重新截取")
	}
	base := s.snapshot()
	s.busy(contextOf(base), "重新识别…")
	go func() {
		s.emit(s.engine.RetryOCR(context.Background(), base, providerID, crop))
	}()
	return nil
}

// Retry re-runs a single dictionary or translation service on the current text,
// leaving the other rows untouched.
func (s *Service) Retry(providerID string) error {
	base := s.snapshot()
	if base == nil || strings.TrimSpace(base.SourceText) == "" {
		return errors.New("没有可翻译的文本")
	}
	s.busy(base.Context, "查询中…")
	go func() {
		s.emit(s.engine.Retry(context.Background(), base, providerID))
	}()
	return nil
}

// ReTranslate re-runs every service of the current context.
func (s *Service) ReTranslate() error {
	base := s.snapshot()
	if base == nil || strings.TrimSpace(base.SourceText) == "" {
		return errors.New("没有可翻译的文本")
	}
	return s.Ask(query.Request{Text: base.SourceText, Context: base.Context, Mode: base.Mode})
}

// SetTargetLang switches the output language and refreshes what is on screen.
// With no answer on screen there is nothing to refresh; the choice is still
// saved, and reporting "没有可翻译的文本" for a preference change read as an
// error the user had to act on.
func (s *Service) SetTargetLang(code string) error {
	if code == "" {
		return errors.New("语言代码不能为空")
	}
	if _, err := s.cfg.Update(func(c *settings.Config) { c.TargetLang = code }); err != nil {
		return err
	}
	s.mu.Lock()
	empty := s.last == nil
	s.mu.Unlock()
	if empty {
		return nil
	}
	return s.ReTranslate()
}

// Speak returns audio for a passage. Playable folds a provider's bytes into a
// data: URL, because the bridge carries only Clip.URL.
func (s *Service) Speak(text, lang string, slow bool) tts.Result {
	return localizedSpeech(s.engine.Speak(text, lang, slow).Playable())
}

// History returns the newest matching entries.
func (s *Service) History(needle string, favoritesOnly bool, service string, limit int) []history.Entry {
	if s.history == nil {
		return nil
	}
	if limit <= 0 || limit > 2000 {
		limit = 200
	}
	return localizedHistory(s.history.List(history.Filter{
		Needle: needle, Favorites: favoritesOnly, Service: service,
	}, limit))
}

// HistoryServices names the services the log holds rows for, most used first.
// The dropdown shows Label and sends back Name, because Name is what the rows
// were recorded with.
func (s *Service) HistoryServices() []history.ServiceCount {
	if s.history == nil {
		return nil
	}
	out := s.history.Services()
	for i := range out {
		out[i].Label = l10n.T(out[i].Name)
	}
	return out
}

// StarHistory flips 收藏 and reports the new state, so the button a user pressed
// can show what they just made it — the panel has no list to read the row from.
func (s *Service) StarHistory(id string) (bool, error) {
	if s.history == nil {
		return false, errors.New("历史不可用")
	}
	on, err := s.history.Star(id)
	return on, err
}

func (s *Service) DeleteHistory(id string) error {
	if s.history == nil {
		return errors.New("历史不可用")
	}
	return s.history.Delete(id)
}

// ClearHistory drops entries older than days; days <= 0 empties the log but
// keeps favorites, which is what the button in the UI advertises.
func (s *Service) ClearHistory(days int) (int, error) {
	if s.history == nil {
		return 0, errors.New("历史不可用")
	}
	return s.history.Clear(days)
}

// ExportHistory renders the current filter for the clipboard.
func (s *Service) ExportHistory(needle string, favoritesOnly bool, service, format string) string {
	if s.history == nil {
		return ""
	}
	return s.history.Export(history.Filter{
		Needle: needle, Favorites: favoritesOnly, Service: service,
	}, format)
}

func contextOf(res *query.Response) string {
	if res == nil {
		return settings.ContextMain
	}
	return res.Context
}
