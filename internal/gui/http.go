package gui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"transdict/internal/dict"
	"transdict/internal/l10n"
	"transdict/internal/langs"
	"transdict/internal/ocr"
	"transdict/internal/query"
	"transdict/internal/selection"
	"transdict/internal/settings"
)

// The local API is Easydict's HTTP server: a handful of endpoints another app
// can call to translate, recognize text and read the selection. It binds to
// loopback only — this is an integration surface, not a network service — and
// starts and stops with 启用 HTTP 服务 without a relaunch.

// maxHTTPBody is Easydict's /ocr cap. Everything else is a few sentences.
const (
	maxHTTPBody = 1 << 20
	maxOCRBody  = 10 << 20
)

// syncHTTPServer is a settings watcher, so flipping the switch or changing the
// port takes effect immediately. A port that is already taken is reported in
// the log and in the panel rather than silently pretending to listen.
func (s *Service) syncHTTPServer(cfg settings.Config) {
	port := settings.DefaultHTTPPort
	if p, err := settings.NormalizeHTTPPort(cfg.HTTPPort); err == nil {
		port = p
	}

	s.mu.Lock()
	running, samePort := s.http != nil, s.httpPort == port
	s.mu.Unlock()

	if !cfg.EnableHTTPServer {
		if running {
			s.stopHTTPServer()
		}
		return
	}
	if running && samePort {
		return
	}
	if running {
		s.stopHTTPServer()
	}

	listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", port))
	if err != nil {
		log.Printf("transdict: HTTP 服务启动失败: %v", err)
		if s.wails != nil {
			s.busy(settings.ContextMain, fmt.Sprintf(l10n.T("💥开启本地服务器失败，port: %s"), port))
		}
		return
	}
	srv := &http.Server{Handler: s.httpHandler(), ReadHeaderTimeout: 10 * time.Second}
	s.mu.Lock()
	s.http = srv
	s.httpPort = port
	s.mu.Unlock()

	go func() {
		if err := srv.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("transdict: HTTP 服务: %v", err)
		}
	}()
}

func (s *Service) stopHTTPServer() {
	s.mu.Lock()
	srv := s.http
	s.http = nil
	s.httpPort = ""
	s.mu.Unlock()
	if srv != nil {
		_ = srv.Close()
	}
}

func (s *Service) httpHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.httpHello)
	mux.HandleFunc("POST /translate", s.httpTranslate)
	mux.HandleFunc("POST /streamTranslate", s.httpStreamTranslate)
	mux.HandleFunc("POST /ocr", s.httpOCR)
	mux.HandleFunc("POST /detect", s.httpDetect)
	mux.HandleFunc("GET /selectedText", s.httpSelectedText)
	return mux
}

func (s *Service) httpHello(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("Hello, Welcome to Transdict server!"))
}

// The request and response bodies mirror Easydict's TranslationRequest /
// TranslationResponse, so a client written for its server works here unchanged.
type translationRequest struct {
	Text                 string   `json:"text"`
	SourceLanguage       *string  `json:"sourceLanguage"`
	TargetLanguage       string   `json:"targetLanguage"`
	ServiceType          string   `json:"serviceType"`
	AppleDictionaryNames []string `json:"appleDictionaryNames"`
	QueryType            uint     `json:"queryType"`
}

type translationResponse struct {
	TranslatedText  string           `json:"translatedText"`
	SourceLanguage  string           `json:"sourceLanguage"`
	HTMLStrings     []string         `json:"HTMLStrings,omitempty"`
	DictionaryEntry *dictionaryEntry `json:"dictionaryEntry,omitempty"`
}

type ocrRequest struct {
	ImageData      []byte  `json:"imageData"`
	SourceLanguage *string `json:"sourceLanguage"`
}

type ocrResponse struct {
	OCRText        string `json:"ocrText"`
	SourceLanguage string `json:"sourceLanguage"`
}

type detectRequest struct {
	Text string `json:"text"`
}

type detectResponse struct {
	SourceLanguage string `json:"sourceLanguage"`
}

type selectedTextResponse struct {
	SelectedText *string `json:"selectedText,omitempty"`
}

func (s *Service) httpTranslate(w http.ResponseWriter, r *http.Request) {
	var req translationRequest
	if !decodeJSON(w, r, &req, maxHTTPBody) {
		return
	}
	text := strings.TrimSpace(req.Text)
	if text == "" {
		writeHTTPError(w, http.StatusBadRequest, errors.New("text 不能为空"))
		return
	}
	cfg := s.cfg.Get()
	kind, category, ok := serviceKind(cfg, req.ServiceType)
	if !ok {
		writeHTTPError(w, http.StatusBadRequest, fmt.Errorf("不支持的服务：%s", req.ServiceType))
		return
	}
	src, dst := "", strings.TrimSpace(req.TargetLanguage)
	if req.SourceLanguage != nil {
		src = strings.TrimSpace(*req.SourceLanguage)
	}
	if dst == "" {
		dst = cfg.TargetLang
	}

	res := s.runHTTP(r.Context(), category, kind, text, src, dst)
	if category == settings.CatDict {
		answer := dictionaryText(res.Dict)
		entry := dictionaryEntryFrom(res.Dict)
		// A dictionary can answer without a word result — Apple returns one
		// block of text — and that text is still an answer.
		if answer == "" && entry == nil {
			writeHTTPError(w, http.StatusBadGateway, firstDictError(res.Dict))
			return
		}
		writeJSON(w, http.StatusOK, translationResponse{
			TranslatedText:  answer,
			SourceLanguage:  sourceLanguageOf(cfg, res, text, src),
			DictionaryEntry: entry,
		})
		return
	}
	if len(res.Translate) == 0 {
		writeHTTPError(w, http.StatusBadGateway, errors.New("没有可用的翻译服务"))
		return
	}
	tr := res.Translate[0]
	if tr.Error != "" {
		writeHTTPError(w, http.StatusBadGateway, errors.New(tr.Error))
		return
	}
	writeJSON(w, http.StatusOK, translationResponse{
		TranslatedText: tr.Text,
		SourceLanguage: sourceLanguageOf(cfg, res, text, src),
	})
}

// httpStreamTranslate answers the way Easydict does for a service that cannot
// stream: this build's providers all return a whole answer, so a streaming
// client is pointed at /translate instead of being left with an empty body.
func (s *Service) httpStreamTranslate(w http.ResponseWriter, r *http.Request) {
	writeHTTPError(w, http.StatusBadRequest,
		errors.New("streamTranslate 只支持流式服务，请改用 /translate"))
}

func (s *Service) httpOCR(w http.ResponseWriter, r *http.Request) {
	var req ocrRequest
	if !decodeJSON(w, r, &req, maxOCRBody) {
		return
	}
	if len(req.ImageData) == 0 {
		writeHTTPError(w, http.StatusBadRequest, errors.New("imageData 不能为空"))
		return
	}
	src := ""
	if req.SourceLanguage != nil {
		src = strings.TrimSpace(*req.SourceLanguage)
	}
	q := query.Request{Context: settings.ContextScreenshot, Mode: "http",
		Image: req.ImageData, SourceLang: src}
	out := s.engine.OCRStage(r.Context(), s.cfg.Get(), q)
	text := query.OCRText(s.cfg.Get(), out)
	if text == "" {
		writeHTTPError(w, http.StatusBadGateway, ocrFailure(out))
		return
	}
	writeJSON(w, http.StatusOK, ocrResponse{
		OCRText:        text,
		SourceLanguage: sourceLanguageOfText(s.cfg.Get(), text, src),
	})
}

func (s *Service) httpDetect(w http.ResponseWriter, r *http.Request) {
	var req detectRequest
	if !decodeJSON(w, r, &req, maxHTTPBody) {
		return
	}
	if strings.TrimSpace(req.Text) == "" {
		writeHTTPError(w, http.StatusBadRequest, errors.New("text 不能为空"))
		return
	}
	writeJSON(w, http.StatusOK, detectResponse{SourceLanguage: detectLanguage(s.cfg.Get(), req.Text)})
}

func (s *Service) httpSelectedText(w http.ResponseWriter, r *http.Request) {
	var pb selection.Pasteboard
	if s.wails != nil {
		pb = pasteboard{s}
	}
	text, err := selection.Grab(pb)
	if err != nil {
		// "Nothing highlighted" is an answer, not a failure: the client gets
		// null and decides what to do.
		if errors.Is(err, selection.ErrNothingSelected) || errors.Is(err, selection.ErrTimeout) {
			writeJSON(w, http.StatusOK, selectedTextResponse{})
			return
		}
		writeHTTPError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, selectedTextResponse{SelectedText: &text})
}

// runHTTP runs one named service without opening a window or recording history:
// an API call is a request from another program, not something the user asked
// for, so it must leave no trace in the panel.
func (s *Service) runHTTP(ctx context.Context, category, kind, text, src, dst string) *query.Response {
	contextName := settings.ContextTranslation
	if category == settings.CatDict {
		contextName = settings.ContextODict
	}
	return s.engine.Run(ctx, query.Request{
		Context:        contextName,
		Mode:           "http",
		Text:           text,
		SourceLang:     src,
		TargetLang:     dst,
		Service:        kind,
		DictionaryOnly: category == settings.CatDict,
	})
}

// serviceAliases maps the names Easydict's enum uses onto this build's provider
// kinds. Apple's dictionary and translation services share the kind "apple",
// so the alias also names the category the caller meant. The rest match by
// lowercased kind or label.
type serviceAlias struct {
	kind     string
	category string
}

var serviceAliases = map[string]serviceAlias{
	"appledictionary":     {kind: "apple", category: settings.CatDict},
	"appledict":           {kind: "apple", category: settings.CatDict},
	"apple":               {kind: "apple", category: settings.CatTranslate},
	"appletranslate":      {kind: "apple", category: settings.CatTranslate},
	"microsoft":           {kind: "bing"},
	"microsofttranslator": {kind: "bing"},
	"youdaotranslate":     {kind: "youdao"},
	"chatgpt":             {kind: "openai"},
}

// serviceKind finds the configured service a serviceType names, in the order
// that matters to /translate: translators first, then dictionaries.
func serviceKind(cfg settings.Config, name string) (kind, category string, ok bool) {
	want := strings.ToLower(strings.TrimSpace(name))
	only := ""
	if alias, found := serviceAliases[want]; found {
		want, only = alias.kind, alias.category
	}
	if want == "" {
		return "", "", false
	}
	categories := []string{settings.CatTranslate, settings.CatDict}
	if only != "" {
		categories = []string{only}
	}
	for _, cat := range categories {
		for _, p := range cfg.List(cat) {
			if strings.ToLower(p.Kind) == want ||
				strings.ToLower(p.Label) == want ||
				strings.ToLower(p.ID) == want {
				return p.Kind, cat, true
			}
		}
	}
	return "", "", false
}

func sourceLanguageOf(cfg settings.Config, res *query.Response, text, src string) string {
	if res.Detected != "" {
		return res.Detected
	}
	return sourceLanguageOfText(cfg, text, src)
}

func sourceLanguageOfText(cfg settings.Config, text, src string) string {
	if src != "" && src != langs.Auto {
		return src
	}
	return detectLanguage(cfg, text)
}

func firstDictError(results []dict.Result) error {
	for _, r := range results {
		if r.Error != "" {
			return errors.New(r.Error)
		}
	}
	return errors.New("没有可用的词典服务")
}

// dictionaryText is what /translate answers for a dictionary service: the entry
// as plain text, which is what a caller expecting translatedText can display.
func dictionaryText(results []dict.Result) string {
	for _, r := range results {
		if r.Error != "" {
			continue
		}
		for _, e := range r.Entries {
			if text := e.Plain(); text != "" {
				return text
			}
		}
	}
	return ""
}

func ocrFailure(outcomes []ocr.Outcome) error {
	for _, o := range outcomes {
		if o.Error != "" {
			return errors.New(o.Error)
		}
	}
	return errors.New("OCR 结果为空。⚠️ 请手动选择识别语言再次尝试。")
}

func decodeJSON(w http.ResponseWriter, r *http.Request, v any, limit int64) bool {
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeHTTPError(w, http.StatusBadRequest, err)
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeHTTPError(w http.ResponseWriter, code int, err error) {
	writeJSON(w, code, map[string]string{"error": err.Error()})
}

// dictionaryEntry is Easydict's DictionaryEntry JSON: the word result a
// dictionary service returns, in the shape its API clients already parse.
type dictionaryEntry struct {
	Phonetics   []wordPhonetic `json:"phonetics,omitempty"`
	Parts       []wordPart     `json:"parts,omitempty"`
	Exchanges   []wordExchange `json:"exchanges,omitempty"`
	SimpleWords []simpleWord   `json:"simpleWords,omitempty"`
	Tags        []string       `json:"tags,omitempty"`
	Etymology   string         `json:"etymology,omitempty"`
	Synonyms    []wordPart     `json:"synonyms,omitempty"`
	Antonyms    []wordPart     `json:"antonyms,omitempty"`
	Collocation []wordPart     `json:"collocation,omitempty"`
}

type wordPhonetic struct {
	Word     string `json:"word"`
	Language string `json:"language"`
	Value    string `json:"value,omitempty"`
	SpeakURL string `json:"speakURL,omitempty"`
	Accent   string `json:"accent,omitempty"`
}

type wordPart struct {
	Part  string   `json:"part,omitempty"`
	Means []string `json:"means"`
}

type wordExchange struct {
	Name  string   `json:"name"`
	Words []string `json:"words"`
}

type simpleWord struct {
	Part  string   `json:"part,omitempty"`
	Word  string   `json:"word"`
	Means []string `json:"means,omitempty"`
}

// dictionaryEntryFrom keeps the first usable entry: /translate answers one
// service, so there is no card list to return.
func dictionaryEntryFrom(results []dict.Result) *dictionaryEntry {
	for _, r := range results {
		if r.Error != "" || len(r.Entries) == 0 {
			continue
		}
		entry := dictionaryEntryOf(r.Entries[0])
		if entry.empty() {
			continue
		}
		return entry
	}
	return nil
}

// empty reports that the schema has nothing to carry. A provider that answers
// with one block of text — Apple Dictionary does — has no phonetics, parts or
// exchanges, and an empty object would read as a broken entry.
func (d *dictionaryEntry) empty() bool {
	return len(d.Phonetics) == 0 && len(d.Parts) == 0 && len(d.Exchanges) == 0 &&
		len(d.SimpleWords) == 0 && len(d.Tags) == 0
}

func dictionaryEntryOf(e dict.Entry) *dictionaryEntry {
	out := &dictionaryEntry{Tags: e.Exams}
	for _, p := range e.Phonetics {
		if p.Symbol == "" && p.Audio == "" {
			continue
		}
		out.Phonetics = append(out.Phonetics, wordPhonetic{
			Word:     e.Word,
			Language: p.Lang,
			Value:    p.Symbol,
			SpeakURL: p.Audio,
		})
	}
	for _, d := range e.Definitions {
		if len(d.Meanings) == 0 {
			continue
		}
		out.Parts = append(out.Parts, wordPart{Part: d.Pos, Means: d.Meanings})
	}
	out.Exchanges = exchangesOf(e.Exchange)
	return out
}

// exchangesOf splits 有道's "过去式：went" lines back into the name/words pair
// Easydict's schema uses. A line without a label has no place in the shape and
// is skipped.
func exchangesOf(lines []string) []wordExchange {
	var out []wordExchange
	for _, line := range lines {
		name, values, ok := strings.Cut(line, "：")
		if !ok {
			name, values, ok = strings.Cut(line, ":")
		}
		if !ok {
			continue
		}
		var words []string
		for _, w := range strings.FieldsFunc(values, func(r rune) bool {
			return r == ',' || r == '，' || r == '、'
		}) {
			if w = strings.TrimSpace(w); w != "" {
				words = append(words, w)
			}
		}
		if len(words) == 0 {
			continue
		}
		out = append(out, wordExchange{Name: strings.TrimSpace(name), Words: words})
	}
	return out
}
