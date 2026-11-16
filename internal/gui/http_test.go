package gui

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"transdict/internal/dict"
)

func TestHTTPServerGreetsOnTheRootPath(t *testing.T) {
	s := &Service{cfg: testStore(t)}
	rec := httptest.NewRecorder()
	s.httpHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET / = %d, want 200", rec.Code)
	}
	if body := rec.Body.String(); !strings.Contains(body, "Transdict") {
		t.Errorf("GET / body = %q, want the server greeting", body)
	}
	// The greeting is the root only: an unknown path is a 404, not the greeting.
	miss := httptest.NewRecorder()
	s.httpHandler().ServeHTTP(miss, httptest.NewRequest(http.MethodGet, "/nope", nil))
	if miss.Code != http.StatusNotFound {
		t.Errorf("GET /nope = %d, want 404", miss.Code)
	}
}

func TestHTTPTranslateRejectsWhatItCannotAnswer(t *testing.T) {
	s := &Service{cfg: testStore(t)}
	cases := []struct {
		name string
		body string
	}{
		{"empty text", `{"text":"","serviceType":"Google","targetLanguage":"zh-CN"}`},
		{"unknown service", `{"text":"hello","serviceType":"Nope","targetLanguage":"zh-CN"}`},
		{"not JSON", `not json`},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/translate", strings.NewReader(tc.body))
		s.httpHandler().ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", tc.name, rec.Code)
		}
	}
}

func TestHTTPEndpointsRejectMissingInput(t *testing.T) {
	s := &Service{cfg: testStore(t)}
	for path, body := range map[string]string{
		"/ocr":    `{}`,
		"/detect": `{"text":"  "}`,
	} {
		rec := httptest.NewRecorder()
		s.httpHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("POST %s = %d, want 400", path, rec.Code)
		}
	}
}

func TestServiceKindReadsEasydictServiceNames(t *testing.T) {
	cfg := testStore(t).Get()
	cases := []struct {
		name     string
		kind     string
		category string
	}{
		{"Google", "google", "translate"},
		{"youdao", "youdao", "translate"},
		{"OpenAI", "openai", "translate"},
		{"AppleDictionary", "apple", "dict"},
		{"Apple", "apple", "translate"},
		{"microsoft", "bing", "translate"},
	}
	for _, tc := range cases {
		kind, category, ok := serviceKind(cfg, tc.name)
		if !ok || kind != tc.kind || category != tc.category {
			t.Errorf("serviceKind(%q) = %q/%q/%v, want %q/%q", tc.name, kind, category, ok, tc.kind, tc.category)
		}
	}
	if _, _, ok := serviceKind(cfg, "Nope"); ok {
		t.Error("an unknown serviceType was accepted")
	}
}

func TestDictionaryEntryKeepsEasydictJSONShape(t *testing.T) {
	entry := dictionaryEntryOf(dict.Entry{
		Word:      "hello",
		Phonetics: []dict.Phonetic{{Lang: "en", Symbol: "həˈləʊ", Audio: "https://audio.test/hello"}},
		Definitions: []dict.Definition{
			{Pos: "int.", Meanings: []string{"你好", "喂"}},
		},
		Exchange: []string{"复数：hellos", "过去式: helloed, hellos"},
		Exams:    []string{"CET4"},
	})
	if len(entry.Phonetics) != 1 || entry.Phonetics[0].Value != "həˈləʊ" || entry.Phonetics[0].Language != "en" {
		t.Errorf("phonetics = %+v", entry.Phonetics)
	}
	if len(entry.Parts) != 1 || entry.Parts[0].Part != "int." || len(entry.Parts[0].Means) != 2 {
		t.Errorf("parts = %+v", entry.Parts)
	}
	if len(entry.Exchanges) != 2 || entry.Exchanges[1].Name != "过去式" ||
		len(entry.Exchanges[1].Words) != 2 {
		t.Errorf("exchanges = %+v", entry.Exchanges)
	}
	if len(entry.Tags) != 1 || entry.Tags[0] != "CET4" {
		t.Errorf("tags = %+v", entry.Tags)
	}

	data, err := json.Marshal(translationResponse{
		TranslatedText:  "你好",
		SourceLanguage:  "en",
		HTMLStrings:     []string{"<p>你好</p>"},
		DictionaryEntry: entry,
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, key := range []string{`"translatedText"`, `"sourceLanguage"`, `"HTMLStrings"`, `"dictionaryEntry"`, `"speakURL"`} {
		if !strings.Contains(string(data), key) {
			t.Errorf("response JSON is missing %s: %s", key, data)
		}
	}
}

func TestDictionaryEntryFromSkipsAWordlessAnswer(t *testing.T) {
	wordless := []dict.Result{{Entries: []dict.Entry{{Word: "hello", Text: "a greeting"}}}}
	if got := dictionaryEntryFrom(wordless); got != nil {
		t.Errorf("an entry with only text became %+v, want no dictionaryEntry", got)
	}
	word := []dict.Result{{Entries: []dict.Entry{{Word: "hello",
		Definitions: []dict.Definition{{Pos: "int.", Meanings: []string{"你好"}}}}}}}
	got := dictionaryEntryFrom(word)
	if got == nil || len(got.Parts) != 1 {
		t.Fatalf("dictionaryEntryFrom(word) = %+v, want one part", got)
	}
}

func TestSyncHTTPServerListensAndStops(t *testing.T) {
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	port := probe.Addr().(*net.TCPAddr).Port
	_ = probe.Close()

	store := testStore(t)
	s := &Service{cfg: store}
	cfg := store.Get()
	cfg.EnableHTTPServer = true
	cfg.HTTPPort = strconv.Itoa(port)
	s.syncHTTPServer(cfg)

	url := "http://127.0.0.1:" + cfg.HTTPPort + "/"
	var lastErr error
	for i := 0; i < 50; i++ {
		res, err := http.Get(url)
		if err == nil {
			body, _ := io.ReadAll(res.Body)
			_ = res.Body.Close()
			if res.StatusCode != http.StatusOK || !strings.Contains(string(body), "Transdict") {
				t.Fatalf("GET / = %d %q", res.StatusCode, body)
			}
			lastErr = nil
			break
		}
		lastErr = err
		time.Sleep(20 * time.Millisecond)
	}
	if lastErr != nil {
		t.Fatalf("server never answered: %v", lastErr)
	}

	cfg.EnableHTTPServer = false
	s.syncHTTPServer(cfg)
	if _, err := http.Get(url); err == nil {
		t.Error("the server still answers after 启用 HTTP 服务 was turned off")
	}
}
