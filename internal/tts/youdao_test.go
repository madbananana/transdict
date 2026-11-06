package tts

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"transdict/internal/settings"
)

// The le= hint is not cosmetic: without it dictvoice answers 200 with a short
// silent clip for 中文, 日语 and 韩语, so the voice the user hears is the one the
// request names.
func TestYoudaoRequestsTheNativeVoice(t *testing.T) {
	var got []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		got = append(got, q.Get("audio")+"|"+q.Get("type")+"|"+q.Get("le"))
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write([]byte("ID3fake"))
	}))
	defer srv.Close()

	cases := []struct {
		name     string
		lang     string
		text     string
		wantType string
		wantLE   string
	}{
		{"english is US voice", "en", "hello", "2", "en"},
		{"chinese keeps its native voice", "zh-CN", "你好", "1", "zh"},
		{"traditional chinese", "zh-TW", "你好", "1", "zh"},
		{"japanese", "ja", "こんにちは", "1", "ja"},
		{"undetected sends no hint", "auto", "hello", "1", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got = nil
			p, err := New(settings.Provider{Kind: "youdao", BaseURL: srv.URL, Enabled: true})
			if err != nil {
				t.Fatal(err)
			}
			clip, err := p.Speak(context.Background(), tc.text, tc.lang, false)
			if err != nil {
				t.Fatal(err)
			}
			want := tc.text + "|" + tc.wantType + "|" + tc.wantLE
			if len(got) != 1 || got[0] != want {
				t.Fatalf("请求 = %v, want [%q]", got, want)
			}
			if string(clip.Data) != "ID3fake" {
				t.Errorf("clip data = %q, want the fetched bytes", clip.Data)
			}
			if clip.URL != "" {
				t.Errorf("clip URL = %q, want empty: the bytes are already downloaded", clip.URL)
			}
			if clip.MIME != "audio/mpeg" {
				t.Errorf("MIME = %q, want audio/mpeg", clip.MIME)
			}
		})
	}
}

func TestYoudaoClampsLongText(t *testing.T) {
	var sent string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sent = r.URL.Query().Get("audio")
		_, _ = w.Write([]byte("ID3fake"))
	}))
	defer srv.Close()

	p, _ := New(settings.Provider{Kind: "youdao", BaseURL: srv.URL})
	long := strings.Repeat("字", youdaoMaxAudioRunes+500)
	if _, err := p.Speak(context.Background(), long, "zh-CN", false); err != nil {
		t.Fatal(err)
	}
	if n := len([]rune(sent)); n != youdaoMaxAudioRunes {
		t.Errorf("audio rune length = %d, want clamped to %d", n, youdaoMaxAudioRunes)
	}
}

// A sentence dictvoice cannot pronounce is an error, not an empty clip: the
// engine uses it to move on to the next configured voice.
func TestYoudaoReportsWhenTheEntryHasNoAudio(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"status":500,"msg":"returned null audio"}`))
	}))
	defer srv.Close()

	p, _ := New(settings.Provider{Kind: "youdao", BaseURL: srv.URL})
	_, err := p.Speak(context.Background(), "今天天气很好，我们去公园散步吧。", "zh-CN", false)
	if err == nil {
		t.Fatal("expected an error for a phrase dictvoice cannot read")
	}
	if !strings.Contains(err.Error(), "词条") {
		t.Errorf("error should say the endpoint only reads entries, got %q", err)
	}
}

func TestYoudaoRejectsANonAudioAnswer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":200}`))
	}))
	defer srv.Close()

	p, _ := New(settings.Provider{Kind: "youdao", BaseURL: srv.URL})
	if _, err := p.Speak(context.Background(), "hello", "en", false); err == nil {
		t.Fatal("expected an error for a JSON body")
	}
}

func TestYoudaoEmptyTextErrors(t *testing.T) {
	p, _ := New(settings.Provider{Kind: "youdao"})
	if _, err := p.Speak(context.Background(), "   ", "en", false); err == nil {
		t.Fatal("expected error for empty text")
	} else if !strings.Contains(err.Error(), "空") {
		t.Errorf("error should be Chinese, got %q", err)
	}
}
