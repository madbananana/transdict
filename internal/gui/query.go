package gui

import (
	"context"
	"errors"
	"log"
	"regexp"
	"strings"
	"unicode"

	"transdict/internal/capture"
	"transdict/internal/dict"
	"transdict/internal/history"
	"transdict/internal/l10n"
	"transdict/internal/langs"
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
	// 润色/总结 ask about the text itself; cleaning it would change what the
	// user pointed at. Every other text query gets the two cleanup options.
	if req.Prompt == "" {
		text = cleanQueryText(text, s.cfg.Get())
	}
	req.Text = text
	if req.Context == "" {
		req.Context = settings.ContextMain
	}
	if req.Mode == "" {
		req.Mode = req.Context
	}
	s.copySelection(req.Mode, text)
	s.run(req)
	return nil
}

var (
	commentPrefix = regexp.MustCompile(`^\s*(//+|#+|\*+)\s*`)
	commentBlock  = regexp.MustCompile(`(?s)/\*+(.*?)\*+/`)
	lineBreak     = regexp.MustCompile(`[ \t]*\r?\n[ \t]*`)
)

// cleanQueryText applies Easydict's two query-time options: 查询时自动移除代码
// 注释符号 and 查询时自动替换换行符. It runs before the text is stored, so the
// history row and the source card show what the services were actually asked.
func cleanQueryText(text string, cfg settings.Config) string {
	// Same order as Easydict's handleInputText: words first, then comment
	// markers, then line breaks.
	if cfg.AutoWordSegment {
		text = segmentWords(text)
	}
	if cfg.AutoRemoveComments {
		text = stripComments(text)
	}
	if cfg.ReplaceNewlineSpace {
		text = lineBreak.ReplaceAllString(text, " ")
	}
	return strings.TrimSpace(text)
}

// TransformText applies one of the titlebar 快捷动作 to the query text. The
// rewriting lives here rather than in the window so the menu, the query-time
// options and the dictionary fallback all share one definition of 分词,
// 去除注释 and 换行转空格.
func (s *Service) TransformText(text, action string) string {
	switch action {
	case "newline":
		return strings.TrimSpace(lineBreak.ReplaceAllString(text, " "))
	case "comments":
		return strings.TrimSpace(stripComments(text))
	case "words":
		return strings.TrimSpace(segmentWords(text))
	}
	return text
}

// knownWord is Dictionary Services on macOS; a variable so the segmentation
// tests do not depend on which dictionaries the machine has installed.
var knownWord = dict.IsEnglishWord

// quotePairs is what a quoted selection may be wrapped in. A quoted token loses
// its quotes rather than being split: 'UIKit' is a name, not a sentence.
var quotePairs = map[rune]rune{
	'"': '"', '\'': '\'', '“': '”', '‘': '’', '「': '」', '『': '』',
}

// segmentWords is 查询时自动分词, narrower than the label suggests: only a
// single token is split, because a sentence already has its spaces. A real word
// the dictionary knows (iPad, LaTeX) stays whole — that check is what keeps the
// splitter from breaking names apart.
func segmentWords(text string) string {
	if !isSingleWord(text) {
		return text
	}
	if quoted, ok := trimQuotePair(text); ok {
		return quoted
	}
	if !needsSegmentation(text) || knownWord(text) {
		return text
	}
	return splitIdentifier(text)
}

func isSingleWord(text string) bool {
	return text != "" && !strings.ContainsFunc(text, unicode.IsSpace)
}

// needsSegmentation looks for the two marks an identifier carries: snake_case
// underscores, or a capital that starts a word inside a longer token.
func needsSegmentation(text string) bool {
	if strings.ContainsRune(text, '_') {
		return true
	}
	runes := []rune(text)
	for i := 1; i < len(runes); i++ {
		if !unicode.IsUpper(runes[i]) {
			continue
		}
		if unicode.IsLower(runes[i-1]) {
			return true
		}
		if i+1 < len(runes) && unicode.IsLower(runes[i+1]) {
			return true
		}
	}
	return false
}

func splitIdentifier(text string) string {
	parts := strings.Split(strings.ReplaceAll(text, "_", " "), " ")
	kept := parts[:0]
	for _, p := range parts {
		if p != "" {
			kept = append(kept, splitCamel(p))
		}
	}
	return strings.Join(kept, " ")
}

// splitCamel follows Easydict's rule: a space goes before every capital that
// follows a lowercase letter, and before a capital that starts a new word after
// an acronym ("GetHTTPCode" → "Get HTTP Code").
func splitCamel(word string) string {
	runes := []rune(word)
	var b strings.Builder
	for i, r := range runes {
		if i > 0 && unicode.IsUpper(r) {
			prev := runes[i-1]
			if unicode.IsLower(prev) || (i+1 < len(runes) && unicode.IsLower(runes[i+1])) {
				b.WriteByte(' ')
			}
		}
		b.WriteRune(r)
	}
	return b.String()
}

func trimQuotePair(text string) (string, bool) {
	runes := []rune(text)
	if len(runes) < 3 {
		return text, false
	}
	closing, ok := quotePairs[runes[0]]
	if !ok || runes[len(runes)-1] != closing {
		return text, false
	}
	return string(runes[1 : len(runes)-1]), true
}

// stripComments drops the markers a code selection carries. When every line is
// a comment, the prefixes go and the lines become one sentence; otherwise the
// text is code with maybe an inline /* ... */ note, and only that note is
// unwrapped.
func stripComments(text string) string {
	lines := strings.Split(text, "\n")
	commented := 0
	for _, ln := range lines {
		if strings.TrimSpace(ln) == "" {
			continue
		}
		if !commentPrefix.MatchString(ln) {
			return unwrapCommentBlocks(text)
		}
		commented++
	}
	if commented == 0 {
		return text
	}
	kept := make([]string, 0, len(lines))
	for _, ln := range lines {
		if ln = strings.TrimSpace(commentPrefix.ReplaceAllString(ln, "")); ln != "" {
			kept = append(kept, ln)
		}
	}
	return joinLines(kept)
}

func unwrapCommentBlocks(text string) string {
	return commentBlock.ReplaceAllStringFunc(text, func(block string) string {
		inner := commentBlock.FindStringSubmatch(block)[1]
		var kept []string
		for _, ln := range strings.Split(inner, "\n") {
			if ln = strings.TrimSpace(commentPrefix.ReplaceAllString(ln, "")); ln != "" {
				kept = append(kept, ln)
			}
		}
		return joinLines(kept)
	})
}

// joinLines glues comment lines back into one sentence, keeping a space only
// between two Latin boundaries: Chinese lines would read with a gap otherwise.
func joinLines(lines []string) string {
	var b strings.Builder
	for i, ln := range lines {
		if i > 0 && needsSpace(lines[i-1], ln) {
			b.WriteByte(' ')
		}
		b.WriteString(ln)
	}
	return b.String()
}

func needsSpace(before, after string) bool {
	left, right := []rune(before), []rune(after)
	if len(left) == 0 || len(right) == 0 {
		return false
	}
	return left[len(left)-1] < 128 && right[0] < 128
}

// run starts the pipeline. A query replaces whatever this window was still
// waiting on: the earlier request is cancelled and its ticket stops matching, so
// a slow answer cannot land after the newer one and take over the window, the
// history row or the clipboard. It leaves window placement alone — 遮挡后重识别
// happens inside the open window.
func (s *Service) run(req query.Request) {
	runCtx, ticket, cancel := s.beginRun(req.Context)
	go func() {
		defer cancel()
		s.execute(runCtx, req, ticket)
	}()
}

// QueryText runs text the caller already has: the input window and the
// dictionary lookup both arrive here.
func (s *Service) QueryText(text, context string) error {
	return s.Ask(query.Request{Text: text, Context: context, Mode: "input"})
}

// QueryInApp is EasyDict's query_in_app: the context menu on a query box runs
// the word it was opened on, so a word read in an answer can be looked up
// without retyping it. Every query lands in the same window, so there is no
// window to choose between.
func (s *Service) QueryInApp(text, from string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return errors.New("没有内容")
	}
	return s.Ask(query.Request{Text: text, Context: settings.ContextMain, Mode: "input"})
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

func (s *Service) clipboardText() string {
	text, ok := s.wails.Clipboard.Text()
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
}

// ClipboardOCR recognizes the picture on the pasteboard — 剪贴板 OCR. It is the
// screenshot gesture without the screenshot: a picture already copied from a
// PDF, a chat or another screen goes to the same OCR 窗口 EasyDict opens.
func (s *Service) ClipboardOCR() error {
	img, err := capture.ClipboardImage()
	if err != nil {
		return err
	}
	go s.runOCRPreview(img)
	return nil
}

// OpenLastOCR is 显示 OCR 窗口: the preview is reopened with whatever was
// recognized last, or empty before the first recognition. EasyDict opens the
// window unconditionally, so this does too.
func (s *Service) OpenLastOCR() error {
	s.showOCRPreview()
	return nil
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
	res.Replaceable = req.Replaceable
	// The named source survives into the answer so 重试 asks the same reading.
	// 自动检测 leaves it empty and a retry is free to detect again.
	if req.SourceLang != "" && req.SourceLang != langs.Auto {
		res.SourceLang = req.SourceLang
	}
	s.annotate(res, cfg, req)
	// 调试日志 keeps the shape of an answer — stages, counts, elapsed — and
	// never the text the user asked about.
	failed := 0
	for _, t := range res.Translate {
		if t.Error != "" {
			failed++
		}
	}
	for _, d := range res.Dict {
		if d.Error != "" {
			failed++
		}
	}
	panel := ""
	if res.Error != "" {
		panel = ", panel error"
	}
	log.Printf("transdict: query %s: %d translate (%d failed), %d dict, %d ocr, %dms%s",
		req.Context, len(res.Translate), failed, len(res.Dict), len(res.OCR), res.ElapsedMs, panel)
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
	if len(req.Image) > 0 && !cfg.AutoQueryOCR && len(res.OCR) > 0 {
		// 图片 OCR 后自动查询 off: the recognized text is the whole answer, so the
		// missing translations are not a missing service.
		return
	}
	if len(req.Image) > 0 && query.WholeURL(res.SourceText) {
		// A recognized link is its own answer: the engine returned early on
		// purpose, so no translation is missing and no service is to blame.
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

// LastResult re-sends the previous response so a window opened after the query
// can render what already happened. It is a fresh draw, so it words itself in
// the language the window will show.
func (s *Service) LastResult() *query.Response {
	res := s.snapshot()
	if res == nil {
		return nil
	}
	next := localizedResponse(*res)
	return &next
}

// PanelLast is LastResult for the translation window, which is now the only
// window that renders an answer.
func (s *Service) PanelLast() *query.Response {
	return s.LastResult()
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

// StopTranslate ends the streaming answer of one service in one window, the
// way EasyDict's stop.circle does: what has already streamed stays on the card
// and the service finishes there. A service that has landed has nothing to
// stop, so an unknown id is not an error.
func (s *Service) StopTranslate(context, providerID string) error {
	s.engine.StopTranslate(context, providerID)
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
	return s.Ask(query.Request{Text: base.SourceText, Context: base.Context, Mode: base.Mode,
		SourceLang: base.SourceLang, Replaceable: base.Replaceable})
}

// ForgetLast drops the stored answer. 清空全部 hides it in the window, and
// without this 重试 would bring it back from behind the curtain.
func (s *Service) ForgetLast() {
	s.mu.Lock()
	s.last = nil
	s.lastOCR = nil
	s.mu.Unlock()
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
	return localizedSpeech(s.engine.Speak(text, speakLang(lang, s.cfg.Get().Pronunciation), slow).Playable())
}

// speakLang applies 英文发音 to the code a provider receives. Only English has
// two voices; every other language keeps the code it detected.
func speakLang(lang, pronunciation string) string {
	if !strings.HasPrefix(lang, "en") {
		return lang
	}
	if settings.ClampPronunciation(pronunciation) == settings.PronunciationUK {
		return "en-GB"
	}
	return "en-US"
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
	if err == nil {
		s.command("history-changed")
	}
	return on, err
}

// FavoriteQuery is 快捷动作 → 添加到收藏. Unlike the 收藏 button on a history
// row it does not need that row to exist, so it also works with 记录查询历史
// off; the user asked for this one explicitly.
func (s *Service) FavoriteQuery(text, translated, detected, targetLang string) (bool, error) {
	if s.history == nil {
		return false, errors.New("历史不可用")
	}
	on, err := s.history.Favorite(text, translated, detected, targetLang)
	if err == nil {
		s.command("history-changed")
	}
	return on, err
}

func (s *Service) DeleteHistory(id string) error {
	if s.history == nil {
		return errors.New("历史不可用")
	}
	err := s.history.Delete(id)
	if err == nil {
		s.command("history-changed")
	}
	return err
}

// ClearHistory drops entries older than days; days <= 0 empties the log but
// keeps favorites, which is what the button in the UI advertises.
func (s *Service) ClearHistory(days int) (int, error) {
	if s.history == nil {
		return 0, errors.New("历史不可用")
	}
	n, err := s.history.Clear(days)
	if err == nil {
		s.command("history-changed")
	}
	return n, err
}

// ClearFavorites drops every starred entry, which is what the 收藏 tab's
// 清空 button does after its confirmation.
func (s *Service) ClearFavorites() (int, error) {
	if s.history == nil {
		return 0, errors.New("历史不可用")
	}
	n, err := s.history.ClearFavorites()
	if err == nil {
		s.command("history-changed")
	}
	return n, err
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
