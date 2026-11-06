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
