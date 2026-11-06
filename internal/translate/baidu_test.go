package translate

import (
	"context"
	"crypto/md5"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"transdict/internal/settings"
)

func TestBaidu(t *testing.T) {
	const (
		appID = "20250101000000001"
		key   = "test-secret-key"
		salt  = "1435660288"
	)
	restoreNonce := baiduNonce
	baiduNonce = func() string { return salt }
	defer func() { baiduNonce = restoreNonce }()

	sign := func(q string) string {
		return fmt.Sprintf("%x", md5.Sum([]byte(appID+q+salt+key)))
	}

	cases := []struct {
		name        string
		text        string
		src, dst    string
		handler     http.HandlerFunc
		want        string
		wantDetect  string
		wantErrFlag string
	}{
		{
			name: "success verifies deterministic signature",
			text: "hello world",
			src:  "en", dst: "zh-CN",
			handler: func(w http.ResponseWriter, r *http.Request) {
				_ = r.ParseForm()
				if got := r.PostForm.Get("sign"); got != sign(r.PostForm.Get("q")) {
					t.Errorf("sign mismatch: got %q want %q", got, sign(r.PostForm.Get("q")))
				}
				if r.PostForm.Get("appid") != appID || r.PostForm.Get("salt") != salt {
					t.Errorf("auth params wrong: %+v", r.PostForm)
				}
				if r.PostForm.Get("from") != "en" || r.PostForm.Get("to") != "zh" {
					t.Errorf("lang params wrong: from=%q to=%q", r.PostForm.Get("from"), r.PostForm.Get("to"))
				}
				fmt.Fprintf(w, `{"from":"en","to":"zh","trans_result":[{"src":%q,"dst":"你好世界"}]}`, r.PostForm.Get("q"))
			},
			want:       "你好世界",
			wantDetect: "en",
		},
		{
			name: "vendor error code surfaces",
			text: "hi", src: "auto", dst: "en",
			handler: func(w http.ResponseWriter, r *http.Request) {
				io.WriteString(w, `{"error_code":"54003","error_msg":"Invalid Sign"}`)
			},
			wantErrFlag: "54003",
		},
		{
			name: "long text is split and rejoined with newlines",
			text: strings.Repeat("A", 1500) + "\n" + strings.Repeat("B", 1200),
			src:  "auto", dst: "en",
			handler: func(w http.ResponseWriter, r *http.Request) {
				_ = r.ParseForm()
				q := r.PostForm.Get("q")
				if len([]rune(q)) > 2000 {
					t.Errorf("chunk exceeds 2000 runes: %d", len([]rune(q)))
				}
				// Echo the source text so the reassembled result must equal the input.
				fmt.Fprintf(w, `{"from":"en","trans_result":[{"src":%q,"dst":%q}]}`, q, q)
			},
			want:       strings.Repeat("A", 1500) + "\n" + strings.Repeat("B", 1200),
			wantDetect: "en",
		},
	}

	epRestore := baiduEndpoint
	defer func() { baiduEndpoint = epRestore }()

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(tc.handler)
			defer srv.Close()
			baiduEndpoint = srv.URL

			p, err := New(settings.Provider{Kind: "baidu", AppID: appID, APIKey: key})
			if err != nil {
				t.Fatal(err)
			}
			out, detected, err := p.Translate(context.Background(), tc.text, tc.src, tc.dst)
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
			if detected != tc.wantDetect {
				t.Fatalf("detected %q want %q", detected, tc.wantDetect)
			}
		})
	}
}
