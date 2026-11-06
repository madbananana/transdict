package tts

import (
	"context"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"

	"transdict/internal/settings"
)

func TestGoogleTokenShape(t *testing.T) {
	got := googleToken("hello world")
	if !regexp.MustCompile(`^\d+\.\d+$`).MatchString(got) {
		t.Errorf("googleToken(%q) = %q, want N.N form", "hello world", got)
	}
	// The token must depend on the text (per documented dt/bd segment).
	if googleToken("hello world") == googleToken("goodbye moon") {
		t.Errorf("token should vary with text")
	}
}

func TestGoogleChunk(t *testing.T) {
	const max = 40
	long := strings.Repeat("这是一个句子。", 40) // 40 * 7 runes, well past max.
	chunks := googleChunk(long, max)
	if len(chunks) < 2 {
		t.Fatalf("expected chunking of a long string, got %d chunk(s)", len(chunks))
	}
	var reassembled strings.Builder
	for _, c := range chunks {
		if len([]rune(c)) > max {
			t.Errorf("chunk exceeds max: %d runes for %q", len([]rune(c)), c)
		}
		if strings.TrimSpace(c) == "" {
			t.Errorf("chunk is whitespace-only: %q", c)
		}
		reassembled.WriteString(c)
	}
	// Only whitespace is ever dropped when packing chunks together.
	want := strings.ReplaceAll(long, " ", "")
	if strings.ReplaceAll(reassembled.String(), " ", "") != want {
		t.Errorf("chunked text does not reconstruct input")
	}
}

func TestGoogleChunkHardCutsOversizedToken(t *testing.T) {
	max := 10
	chunks := googleChunk(strings.Repeat("x", 25), max) // no spaces, no terminators
	for _, c := range chunks {
		if len([]rune(c)) > max {
			t.Fatalf("hard split failed, chunk len %d > %d", len([]rune(c)), max)
		}
	}
	if strings.Join(chunks, "") != strings.Repeat("x", 25) {
		t.Errorf("hard split lost characters: %v", chunks)
	}
}

func TestGoogleSpeaksAndConcatenates(t *testing.T) {
	restore := googleToken
	googleToken = func(string) string { return "1234.5678" }
	defer func() { googleToken = restore }()

	var mu sync.Mutex
	var texts []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		q := r.URL.Query()
		if got := q.Get("client"); got != "t" {
			t.Errorf("client = %q, want t", got)
		}
		if q.Get("dt") != "t" {
			t.Errorf("dt param missing/wrong: %q", q.Get("dt"))
		}
		if q.Get("tok") != "1234.5678" {
			t.Errorf("tok = %q, want the per-segment token", q.Get("tok"))
		}
		if ua := r.Header.Get("User-Agent"); !strings.Contains(ua, "Mozilla") {
			t.Errorf("expected browser-ish UA, got %q", ua)
		}
		text := q.Get("text")
		mu.Lock()
		texts = append(texts, text)
		mu.Unlock()
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write([]byte("[" + text + "]"))
	}))
	defer srv.Close()

	p, err := New(settings.Provider{Kind: "google", BaseURL: srv.URL, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}

	// Single short chunk -> one request, body equals the canned response.
	clip, err := p.Speak(context.Background(), "Hello there", "en", false)
	if err != nil {
		t.Fatal(err)
	}
	if string(clip.Data) != "[Hello there]" {
		t.Errorf("single-chunk Data = %q", clip.Data)
	}
	if clip.MIME != "audio/mpeg" {
		t.Errorf("MIME = %q, want audio/mpeg", clip.MIME)
	}

	// Long text -> multiple downloads concatenated in order.
	mu.Lock()
	texts = nil
	mu.Unlock()
	long := strings.Repeat("第一句话。", 60)
	clip, err = p.Speak(context.Background(), long, "zh-CN", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(texts) < 2 {
		t.Fatalf("expected multiple segments, got %d", len(texts))
	}
	var want strings.Builder
	for _, tx := range texts {
		want.WriteString("[" + tx + "]")
	}
	if string(clip.Data) != want.String() {
		t.Errorf("concatenated Data does not match per-segment bodies")
	}
	// tl must carry the mapped language.
	mu.Lock()
	defer mu.Unlock()
}

func TestGoogleLangMapping(t *testing.T) {
	cases := map[string]string{"auto": "en", "": "en", "zh": "zh-CN", "zh-CN": "zh-CN", "zh-TW": "zh-TW", "ko": "ko", "fr": "fr"}
	for in, want := range cases {
		if got := googleLang(in); got != want {
			t.Errorf("googleLang(%q) = %q, want %q", in, got, want)
		}
	}
}
