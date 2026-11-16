package tts

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"transdict/internal/settings"
)

var testMP3 = []byte{0xFF, 0xFB, 0x90, 0x64, 0x00, 0x00}

// bingServer serves the translator page and tfettts, counting page loads so a
// token refresh is visible.
func bingServer(t *testing.T, posts *int32, status int) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/translator", func(w http.ResponseWriter, r *http.Request) {
		issued := time.Now().UnixMilli()
		fmt.Fprintf(w, `<html><script>IG:"IG-1",data-iid="translator.5029",
var params_AbusePreventionHelper = [%d,"tok",3600000];</script></html>`, issued)
	})
	mux.HandleFunc("/tfettts", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(posts, 1)
		if got := r.URL.Query().Get("IG"); got != "IG-1" {
			t.Errorf("IG = %q, want IG-1", got)
		}
		if got := r.URL.Query().Get("IID"); got != "translator.5029" {
			t.Errorf("IID = %q", got)
		}
		if got := r.FormValue("token"); got != "tok" {
			t.Errorf("token = %q", got)
		}
		if got := r.FormValue("key"); got == "" {
			t.Error("key missing")
		}
		if !strings.Contains(r.FormValue("ssml"), "<speak") {
			t.Errorf("ssml = %q", r.FormValue("ssml"))
		}
		if status != http.StatusOK {
			w.WriteHeader(status)
			return
		}
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write(testMP3)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func newBingForTest(t *testing.T, srv *httptest.Server) *bing {
	t.Helper()
	p, err := New(settings.Provider{Kind: "bing", BaseURL: srv.URL + "/translator"})
	if err != nil {
		t.Fatal(err)
	}
	return p.(*bing)
}

func TestBingSpeaksWithTokenFromTranslatorPage(t *testing.T) {
	var posts int32
	srv := bingServer(t, &posts, http.StatusOK)
	b := newBingForTest(t, srv)

	clip, err := b.Speak(context.Background(), "hello", "en", false)
	if err != nil {
		t.Fatal(err)
	}
	if string(clip.Data) != string(testMP3) || clip.MIME != "audio/mpeg" {
		t.Errorf("clip = %+v", clip)
	}
	if posts != 1 {
		t.Errorf("posts = %d, want 1", posts)
	}
}

func TestBingRefreshesAnExpiredTokenOnce(t *testing.T) {
	var posts int32
	srv := bingServer(t, &posts, http.StatusForbidden)
	b := newBingForTest(t, srv)

	if _, err := b.Speak(context.Background(), "hello", "en", false); err == nil {
		t.Fatal("两次请求都被拒绝时应当报错")
	}
	if posts != 2 {
		t.Errorf("posts = %d, want 2 (one retry after 403)", posts)
	}
}

func TestBingSSMLPicksVoiceAndEscapes(t *testing.T) {
	if got := bingSSML("a & b < c", "en", false); !strings.Contains(got, "en-US-JennyNeural") ||
		!strings.Contains(got, "a &amp; b &lt; c") || !strings.Contains(got, "rate='-10%'") {
		t.Errorf("en SSML = %s", got)
	}
	if got := bingSSML("你好", "zh-TW", true); !strings.Contains(got, "zh-TW-HsiaoChenNeural") ||
		!strings.Contains(got, "rate='-40%'") {
		t.Errorf("zh-TW SSML = %s", got)
	}
	if got := bingSSML("x", "tl", false); !strings.Contains(got, "fil-PH-AlingNeural") {
		t.Errorf("Filipino voice = %s", got)
	}
	if got := bingSSML("x", "xx-unknown", false); !strings.Contains(got, "en-US-JennyNeural") {
		t.Errorf("unknown language should fall back to English: %s", got)
	}
}

func TestBingTrimsLongText(t *testing.T) {
	long := strings.Repeat("字", 5000)
	ssml := bingSSML(clampRunes(long, 2000), "zh-CN", false)
	if n := strings.Count(ssml, "字"); n != 2000 {
		t.Errorf("CJK characters = %d, want 2000", n)
	}
	if n := len([]rune(clampRunes(strings.Repeat("a", 9000), bingMaxRunes))); n != 7000 {
		t.Errorf("Latin characters = %d, want 7000", n)
	}
}

// TestBingLive talks to the real translator page; it runs only when asked,
// because CI has no network guarantee and Bing rotates its token format.
func TestBingLive(t *testing.T) {
	if testing.Short() || os.Getenv("TRANSDUCT_LIVE_BING") == "" {
		t.Skip("set TRANSDUCT_LIVE_BING=1 to query bing.com")
	}
	p, err := New(settings.Provider{Kind: "bing"})
	if err != nil {
		t.Fatal(err)
	}
	clip, err := p.Speak(context.Background(), "hello world", "en", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(clip.Data) < 1000 || !isMP3(clip.Data) {
		t.Fatalf("clip = %d bytes, %q", len(clip.Data), clip.MIME)
	}
	t.Logf("bing audio: %d bytes", len(clip.Data))
}
