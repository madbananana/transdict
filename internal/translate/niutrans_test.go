package translate

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"transdict/internal/settings"
)

func TestNiuTrans(t *testing.T) {
	const apiKey = "niu-key-9f3"

	epRestore := niutransEndpoint
	defer func() { niutransEndpoint = epRestore }()

	cases := []struct {
		name        string
		text        string
		src, dst    string
		handler     http.HandlerFunc
		want        string
		wantErrFlag string
	}{
		{
			name: "tgt_text parsed and form authenticated",
			text: "你好", src: "zh-CN", dst: "en",
			handler: func(w http.ResponseWriter, r *http.Request) {
				_ = r.ParseForm()
				if r.PostForm.Get("apikey") != apiKey {
					t.Errorf("apikey = %q", r.PostForm.Get("apikey"))
				}
				if r.PostForm.Get("from") != "zh" || r.PostForm.Get("to") != "en" {
					t.Errorf("from/to = %q/%q", r.PostForm.Get("from"), r.PostForm.Get("to"))
				}
				if r.PostForm.Get("src_text") != "你好" {
					t.Errorf("src_text = %q", r.PostForm.Get("src_text"))
				}
				io.WriteString(w, `{"from":"zh","to":"en","tgt_text":"Hello"}`)
			},
			want: "Hello",
		},
		{
			name: "error_code surfaces",
			text: "hi", src: "auto", dst: "en",
			handler: func(w http.ResponseWriter, r *http.Request) {
				io.WriteString(w, `{"error_code":"13002","error_msg":"apikey is invalid"}`)
			},
			wantErrFlag: "13002",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(tc.handler)
			defer srv.Close()
			niutransEndpoint = srv.URL

			p, err := New(settings.Provider{Kind: "niutrans", APIKey: apiKey})
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
				t.Fatalf("got %q want %q", out, tc.want)
			}
		})
	}
}
