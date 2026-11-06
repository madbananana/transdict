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

func TestCaiyun(t *testing.T) {
	const token = "caiyun-token-abc"

	epRestore := caiyunEndpoint
	idRestore := caiyunRequestID
	caiyunRequestID = func() string { return "req-fixed" }
	defer func() {
		caiyunEndpoint = epRestore
		caiyunRequestID = idRestore
	}()

	cases := []struct {
		name        string
		text        string
		src, dst    string
		handler     http.HandlerFunc
		want        string
		wantErrFlag string
	}{
		{
			name: "X-Ca-Key header and trans_type form",
			text: "Hello", src: "en", dst: "zh-CN",
			handler: func(w http.ResponseWriter, r *http.Request) {
				if got := r.Header.Get("X-Ca-Key"); got != token {
					t.Errorf("X-Ca-Key = %q want %q", got, token)
				}
				_ = r.ParseForm()
				if r.PostForm.Get("trans_type") != "en2zh" {
					t.Errorf("trans_type = %q want en2zh", r.PostForm.Get("trans_type"))
				}
				if r.PostForm.Get("request_id") != "req-fixed" {
					t.Errorf("request_id = %q", r.PostForm.Get("request_id"))
				}
				if r.PostForm.Get("source") != "Hello" {
					t.Errorf("source = %q", r.PostForm.Get("source"))
				}
				io.WriteString(w, `{"target":["你好"],"trans_type":"en2zh","request_id":"req-fixed"}`)
			},
			want: "你好",
		},
		{
			name: "auto direction",
			text: "Hola", src: "auto", dst: "zh-CN",
			handler: func(w http.ResponseWriter, r *http.Request) {
				_ = r.ParseForm()
				if r.PostForm.Get("trans_type") != "auto2zh" {
					t.Errorf("trans_type = %q want auto2zh", r.PostForm.Get("trans_type"))
				}
				io.WriteString(w, `{"translation":"你好"}`)
			},
			want: "你好",
		},
		{
			name: "error_code surfaces",
			text: "hi", src: "en", dst: "zh-CN",
			handler: func(w http.ResponseWriter, r *http.Request) {
				io.WriteString(w, `{"error_code":"operation.notAuthorized","error_msg":"token invalid"}`)
			},
			wantErrFlag: "operation.notAuthorized",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(tc.handler)
			defer srv.Close()
			caiyunEndpoint = srv.URL

			p, err := New(settings.Provider{Kind: "caiyun", APIKey: token})
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
