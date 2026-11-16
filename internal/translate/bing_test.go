package translate

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"transdict/internal/settings"
)

// edge.microsoft.com answers 404 on some networks while bing.com itself is
// reachable. The page mints a token for its own host, and a missing fallback
// put 获取令牌失败 on the card where a translation belonged.
func TestBingFallsBackToTheTranslatorPage(t *testing.T) {
	defer func(a, b, p string) { bingAuthURL, bingAPIURL, bingPageURL = a, b, p }(bingAuthURL, bingAPIURL, bingPageURL)

	var pageHits, webHits int64
	issued := time.Now().Add(time.Hour).UnixMilli()
	mux := http.NewServeMux()
	mux.HandleFunc("/auth", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	mux.HandleFunc("/translator", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&pageHits, 1)
		fmt.Fprintf(w, `<html><script>var params_AbusePreventionHelper = [%d,"SIGNED-KEY",3600000];</script><script>IG:"ABC123"</script>`, issued)
	})
	mux.HandleFunc("/ttranslatev3", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&webHits, 1)
		if err := r.ParseForm(); err != nil {
			t.Errorf("表单解析失败: %v", err)
		}
		if got := r.URL.Query().Get("IG"); got != "ABC123" {
			t.Errorf("IG = %q, want 页面里的值", got)
		}
		want := map[string]string{
			"fromLang": "auto-detect",
			"to":       "zh-Hans",
			"text":     "Hello",
			"key":      strconv.FormatInt(issued, 10),
			"token":    "SIGNED-KEY",
		}
		for k, v := range want {
			if got := r.PostForm.Get(k); got != v {
				t.Errorf("%s = %q, want %q", k, got, v)
			}
		}
		io.WriteString(w, `[{"translations":[{"text":"你好","to":"zh-Hans"}],"detectedLanguage":{"language":"en"}}]`)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()
	bingAuthURL = srv.URL + "/auth"
	bingAPIURL = srv.URL + "/api"
	bingPageURL = srv.URL + "/translator"

	p, err := New(settings.Provider{Kind: "bing"})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		out, detected, err := p.Translate(context.Background(), "Hello", "auto", "zh-CN")
		if err != nil {
			t.Fatalf("第 %d 次: %v", i+1, err)
		}
		if out != "你好" || detected != "en" {
			t.Errorf("第 %d 次 = %q detected=%q", i+1, out, detected)
		}
	}
	if got := atomic.LoadInt64(&pageHits); got != 1 {
		t.Errorf("页面请求 %d 次, want 1（令牌应缓存）", got)
	}
	if got := atomic.LoadInt64(&webHits); got != 2 {
		t.Errorf("翻译请求 %d 次, want 2", got)
	}
}

// EasyDict's Cookie Key（可选）is pasted in when a network treats scripted
// requests as abusive; the translation POST has to carry it or the setting does
// nothing. The token page stays cookieless, as it is in EasyDict.
func TestBingSendsTheConfiguredCookie(t *testing.T) {
	authRestore, apiRestore, pageRestore := bingAuthURL, bingAPIURL, bingPageURL
	defer func() { bingAuthURL, bingAPIURL, bingPageURL = authRestore, apiRestore, pageRestore }()

	for _, tc := range []struct{ name, cookie string }{
		{"configured cookie goes out", "MUID=1; SRCHD=2"},
		{"empty cookie stays off", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var authCookie, translateCookie string
			mux := http.NewServeMux()
			mux.HandleFunc("/auth", func(w http.ResponseWriter, r *http.Request) {
				authCookie = r.Header.Get("Cookie")
				io.WriteString(w, "TEST-JWT")
			})
			mux.HandleFunc("/translate", func(w http.ResponseWriter, r *http.Request) {
				translateCookie = r.Header.Get("Cookie")
				io.WriteString(w, `[{"translations":[{"text":"你好","to":"zh-Hans"}]}]`)
			})
			srv := httptest.NewServer(mux)
			defer srv.Close()
			bingAuthURL = srv.URL + "/auth"
			bingAPIURL = srv.URL + "/translate"
			bingPageURL = srv.URL + "/translator"
			bingTokensMu.Lock()
			bingTokens = map[string]*bingCredential{}
			bingTokensMu.Unlock()

			p, err := New(settings.Provider{
				Kind:  "bing",
				Extra: map[string]string{settings.FieldCookie: tc.cookie},
			})
			if err != nil {
				t.Fatal(err)
			}
			if _, _, err := p.Translate(context.Background(), "Hello", "auto", "zh-CN"); err != nil {
				t.Fatal(err)
			}
			if translateCookie != tc.cookie {
				t.Errorf("翻译请求 Cookie = %q, want %q", translateCookie, tc.cookie)
			}
			if authCookie != "" {
				t.Errorf("令牌请求 Cookie = %q, want 空（EasyDict 的页面请求不带 Cookie）", authCookie)
			}
		})
	}
}

func TestBingOffersTheCookieField(t *testing.T) {
	meta, ok := Meta("bing")
	if !ok {
		t.Fatal("bing 没有注册")
	}
	for _, f := range meta.Fields {
		if f.Key == settings.FieldCookie {
			if f.Label != "Cookie Key（可选）" || !f.Secret {
				t.Errorf("Cookie 字段 = %+v, want 可选且遮蔽", f)
			}
			return
		}
	}
	t.Fatal("必应翻译缺少 Cookie Key 字段")
}

func TestBing(t *testing.T) {
	authRestore := bingAuthURL
	apiRestore := bingAPIURL
	pageRestore := bingPageURL
	defer func() {
		bingAuthURL = authRestore
		bingAPIURL = apiRestore
		bingPageURL = pageRestore
	}()

	long := strings.Repeat("a", 4000) + "\n" + strings.Repeat("b", 3000)

	cases := []struct {
		name        string
		text        string
		src, dst    string
		canned      string // literal response, or "ECHO" to translate by echoing the body
		want        string
		wantErrFlag string
	}{
		{
			name: "canned translation",
			text: "Hello", src: "en", dst: "zh-CN",
			canned: `[{"translations":[{"text":"你好","to":"zh-Hans"}]}]`,
			want:   "你好",
		},
		{
			name: "long text split but token fetched once",
			text: long, src: "auto", dst: "en",
			canned: "ECHO",
			want:   long,
		},
		{
			name: "auth failure surfaces",
			text: "hi", src: "en", dst: "zh-CN",
			canned:      "",
			wantErrFlag: "获取令牌失败",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var authHits int64
			mux := http.NewServeMux()
			mux.HandleFunc("/auth", func(w http.ResponseWriter, r *http.Request) {
				atomic.AddInt64(&authHits, 1)
				if tc.wantErrFlag != "" {
					w.WriteHeader(http.StatusForbidden)
					return
				}
				io.WriteString(w, "TEST-JWT")
			})
			mux.HandleFunc("/translate", func(w http.ResponseWriter, r *http.Request) {
				if got := r.Header.Get("Authorization"); got != "Bearer TEST-JWT" {
					t.Errorf("Authorization = %q", got)
				}
				if r.URL.Query().Get("api-version") != "3.0" {
					t.Errorf("api-version missing")
				}
				body, _ := io.ReadAll(r.Body)
				var in []map[string]string
				if err := json.Unmarshal(body, &in); err != nil || len(in) == 0 {
					t.Fatalf("bad translate body: %v", err)
				}
				text := in[0]["Text"]
				if tc.canned == "ECHO" {
					json.NewEncoder(w).Encode([]map[string]any{
						{"translations": []map[string]string{{"text": text, "to": "en"}}},
					})
					return
				}
				io.WriteString(w, tc.canned)
			})

			srv := httptest.NewServer(mux)
			defer srv.Close()
			bingAuthURL = srv.URL + "/auth"
			bingAPIURL = srv.URL + "/translate"
			// The page is the fallback when auth fails; pointing it at the test
			// server keeps that path off the real network.
			bingPageURL = srv.URL + "/translator"

			p, err := New(settings.Provider{Kind: "bing"})
			if err != nil {
				t.Fatal(err)
			}
			out, _, err := p.Translate(context.Background(), tc.text, tc.src, tc.dst)
			if tc.wantErrFlag != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErrFlag) {
					t.Fatalf("want error containing %q, got %v", tc.wantErrFlag, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if out != tc.want {
				t.Fatalf("got %q want %q", truncate(out), truncate(tc.want))
			}
			if hits := atomic.LoadInt64(&authHits); hits != 1 {
				t.Fatalf("auth token should be cached: got %d fetches", hits)
			}
		})
	}
}

// A 401 from the web route carries {"ShowCaptcha":false}, not a key error.
// Quoting that body leaves the user with no idea what to change, so the card
// names EasyDict's Cookie Key setting instead.
func TestBingExplainsTheAntiAbuseRefusal(t *testing.T) {
	defer func(a, b, p string) { bingAuthURL, bingAPIURL, bingPageURL = a, b, p }(bingAuthURL, bingAPIURL, bingPageURL)

	mux := http.NewServeMux()
	mux.HandleFunc("/auth", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	mux.HandleFunc("/translator", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `<script>var params_AbusePreventionHelper = [%d,"SIGNED-KEY",3600000];</script><script>IG:"ABC123"</script>`,
			time.Now().Add(time.Hour).UnixMilli())
	})
	mux.HandleFunc("/ttranslatev3", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		io.WriteString(w, `{"ShowCaptcha":false}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	bingAuthURL = srv.URL + "/auth"
	bingAPIURL = srv.URL + "/api"
	bingPageURL = srv.URL + "/translator"

	p, err := New(settings.Provider{Kind: "bing"})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = p.Translate(context.Background(), "hello", "auto", "zh-CN")
	if err == nil || !strings.Contains(err.Error(), "Cookie Key") {
		t.Errorf("err = %v, want the Cookie Key guidance", err)
	}
	if strings.Contains(err.Error(), "ShowCaptcha") {
		t.Errorf("err = %v, want the raw body replaced", err)
	}
}

// A bearer token can die before its cached deadline. The first 401 clears the
// cache and the retry mints a new token instead of leaving Bing dead for the
// remaining minutes of the cache window.
func TestBingRetriesAStaleBearer(t *testing.T) {
	defer func(a, b, p string) { bingAuthURL, bingAPIURL, bingPageURL = a, b, p }(bingAuthURL, bingAPIURL, bingPageURL)

	var authHits int64
	mux := http.NewServeMux()
	mux.HandleFunc("/auth", func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt64(&authHits, 1)
		fmt.Fprintf(w, "JWT-%d", n)
	})
	mux.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer JWT-2" {
			w.WriteHeader(http.StatusUnauthorized)
			io.WriteString(w, `{"error":{"code":401001}}`)
			return
		}
		io.WriteString(w, `[{"translations":[{"text":"你好","to":"zh-Hans"}],"detectedLanguage":{"language":"en"}}]`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	bingAuthURL = srv.URL + "/auth"
	bingAPIURL = srv.URL + "/api"
	bingPageURL = srv.URL + "/translator"

	p, err := New(settings.Provider{Kind: "bing"})
	if err != nil {
		t.Fatal(err)
	}
	out, _, err := p.Translate(context.Background(), "hello", "auto", "zh-CN")
	if err != nil {
		t.Fatalf("Translate: %v", err)
	}
	if out != "你好" {
		t.Errorf("out = %q", out)
	}
	if got := atomic.LoadInt64(&authHits); got != 2 {
		t.Errorf("auth hits = %d, want 2 (one retry)", got)
	}
}
