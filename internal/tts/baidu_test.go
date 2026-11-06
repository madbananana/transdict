package tts

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"transdict/internal/settings"
)

// baiduSrv starts one server for both the token and the synth endpoints and
// installs it into the package vars, restoring everything when the test ends.
func baiduSrv(t *testing.T, token func(w http.ResponseWriter, r *http.Request),
	synth func(w http.ResponseWriter, r *http.Request)) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			token(w, r)
		case "/tts":
			synth(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	oldToken, oldTTS, oldNow, oldCUID := baiduTokenURL, baiduTTSURL, baiduNow, baiduCUID
	baiduTokenURL = srv.URL + "/token"
	baiduTTSURL = srv.URL + "/tts"
	baiduNow = time.Now
	baiduCUID = "test-cuid"
	baiduTokenMu.Lock()
	baiduTokens = map[string]baiduToken{}
	baiduTokenMu.Unlock()
	t.Cleanup(func() {
		srv.Close()
		baiduTokenURL, baiduTTSURL, baiduNow, baiduCUID = oldToken, oldTTS, oldNow, oldCUID
		baiduTokenMu.Lock()
		baiduTokens = map[string]baiduToken{}
		baiduTokenMu.Unlock()
	})
	return srv
}

func okToken(w http.ResponseWriter, r *http.Request) {
	_, _ = w.Write([]byte(`{"access_token":"tok-1","expires_in":2592000}`))
}

func newBaidu(t *testing.T) Provider {
	t.Helper()
	p, err := New(settings.Provider{Kind: "baidu", Enabled: true,
		APIKey: "key-1", Secret: "secret-1"})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// The provider used to sign itself against a public route that no longer
// answers; a call with no keys has to say so instead of failing upstream.
func TestBaiduWithoutKeysSaysWhatIsMissing(t *testing.T) {
	p, err := New(settings.Provider{Kind: "baidu", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.Speak(context.Background(), "你好", "zh-CN", false)
	if err == nil || !strings.Contains(err.Error(), "API Key") {
		t.Fatalf("err = %v, want an explanation about the missing key", err)
	}
}

func TestBaiduSpeaksWithTokenAndDoubleEncodedText(t *testing.T) {
	var gotTex, gotTok, gotCUID, gotLAN, gotSPD, gotCTP, gotAUE string
	baiduSrv(t, okToken, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("ParseForm: %v", err)
		}
		gotTex = r.PostForm.Get("tex")
		gotTok = r.PostForm.Get("tok")
		gotCUID = r.PostForm.Get("cuid")
		gotLAN = r.PostForm.Get("lan")
		gotSPD = r.PostForm.Get("spd")
		gotCTP = r.PostForm.Get("ctp")
		gotAUE = r.PostForm.Get("aue")
		w.Header().Set("Content-Type", "audio/mp3")
		_, _ = w.Write([]byte("MP3BYTES"))
	})

	clip, err := newBaidu(t).Speak(context.Background(), "你好 世界", "zh-CN", true)
	if err != nil {
		t.Fatal(err)
	}
	if string(clip.Data) != "MP3BYTES" || clip.MIME != "audio/mp3" {
		t.Errorf("clip = %q %q, want canned MP3", clip.MIME, clip.Data)
	}
	// The API URL-decodes tex itself, so the form value must still be escaped
	// when the server decodes the form.
	if unescaped, err := url.QueryUnescape(gotTex); err != nil || unescaped != "你好 世界" {
		t.Errorf("tex = %q, want the double-encoded text (decodes to %q): %v", gotTex, unescaped, err)
	}
	if gotTok != "tok-1" {
		t.Errorf("tok = %q, want the cached token", gotTok)
	}
	if gotCUID != "test-cuid" || gotCTP != "1" || gotAUE != "3" {
		t.Errorf("cuid/ctp/aue = %q/%q/%q, want test-cuid/1/3", gotCUID, gotCTP, gotAUE)
	}
	if gotLAN != "zh" || gotSPD != "3" {
		t.Errorf("lan/spd = %q/%q, want zh/3 for slow Chinese", gotLAN, gotSPD)
	}
}

func TestBaiduMapsEnglish(t *testing.T) {
	var gotLAN, gotSPD string
	baiduSrv(t, okToken, func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotLAN = r.PostForm.Get("lan")
		gotSPD = r.PostForm.Get("spd")
		_, _ = w.Write([]byte("x"))
	})
	if _, err := newBaidu(t).Speak(context.Background(), "hello", "en", false); err != nil {
		t.Fatal(err)
	}
	if gotLAN != "en" || gotSPD != "5" {
		t.Errorf("lan/spd = %q/%q, want en/5", gotLAN, gotSPD)
	}
}

// A token lasts 30 days, so a 朗读 must not ask for a new one every time.
func TestBaiduTokenIsReusedUntilItExpires(t *testing.T) {
	var tokenCalls int32
	baiduSrv(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&tokenCalls, 1)
		okToken(w, r)
	}, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("x"))
	})

	p := newBaidu(t)
	for i := 0; i < 3; i++ {
		if _, err := p.Speak(context.Background(), "hi", "en", false); err != nil {
			t.Fatal(err)
		}
	}
	if got := atomic.LoadInt32(&tokenCalls); got != 1 {
		t.Errorf("token requests = %d, want 1 for three reads", got)
	}
	// Well past the 30 days, the cache has to be refreshed rather than kept.
	base := time.Now()
	baiduNow = func() time.Time { return base.Add(31 * 24 * time.Hour) }
	if _, err := p.Speak(context.Background(), "hi", "en", false); err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&tokenCalls); got != 2 {
		t.Errorf("token requests after expiry = %d, want 2", got)
	}
}

func TestBaiduTokenFailureShowsTheServiceAnswer(t *testing.T) {
	baiduSrv(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid_client","error_description":"unknown client id"}`))
	}, func(w http.ResponseWriter, r *http.Request) {
		t.Error("synth endpoint must not be called without a token")
	})
	_, err := newBaidu(t).Speak(context.Background(), "hi", "en", false)
	if err == nil || !strings.Contains(err.Error(), "unknown client id") {
		t.Fatalf("err = %v, want the service's reason", err)
	}
}

func TestBaiduJSONErrorIsChinese(t *testing.T) {
	baiduSrv(t, okToken, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"err_no":502,"err_msg":"parameter error."}`))
	})
	_, err := newBaidu(t).Speak(context.Background(), "hi", "en", false)
	if err == nil || !strings.Contains(err.Error(), "百度朗读错误") || !strings.Contains(err.Error(), "502") {
		t.Errorf("error not Chinese/informative: %v", err)
	}
}

func TestBaiduNon200IsChinese(t *testing.T) {
	baiduSrv(t, okToken, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusForbidden)
	})
	_, err := newBaidu(t).Speak(context.Background(), "hi", "en", false)
	if err == nil || !strings.Contains(err.Error(), "HTTP 403") {
		t.Fatalf("expected Chinese 403 error, got %v", err)
	}
}

// The open API speaks Chinese and English; Japanese has to fall through to the
// next provider instead of being read with Chinese phonetics.
func TestBaiduRefusesLanguagesItCannotPronounce(t *testing.T) {
	var calls int32
	baiduSrv(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		okToken(w, r)
	}, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		_, _ = w.Write([]byte("x"))
	})
	_, err := newBaidu(t).Speak(context.Background(), "こんにちは", "ja", false)
	if err == nil || !strings.Contains(err.Error(), "不支持") {
		t.Fatalf("err = %v, want a refusal naming the language", err)
	}
	if got := atomic.LoadInt32(&calls); got != 0 {
		t.Errorf("network calls = %d, want none for an unsupported language", got)
	}
}

func TestBaiduEmptyAudioIsReported(t *testing.T) {
	baiduSrv(t, okToken, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/mp3")
	})
	_, err := newBaidu(t).Speak(context.Background(), "hi", "en", false)
	if err == nil || !strings.Contains(err.Error(), "未返回音频数据") {
		t.Fatalf("err = %v, want the empty-audio message", err)
	}
}
