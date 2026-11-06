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

func TestAlibaba(t *testing.T) {
	const (
		id     = "LTAIexample"
		secret = "secretexample"
		nonce  = "nonce-fixed"
		ts     = "2021-01-01T00:00:00Z"
	)
	nonceRestore := aliNonce
	tsRestore := aliTimestamp
	aliNonce = func() string { return nonce }
	aliTimestamp = func() string { return ts }
	defer func() {
		aliNonce = nonceRestore
		aliTimestamp = tsRestore
	}()

	epRestore := alibabaEndpoint
	defer func() { alibabaEndpoint = epRestore }()

	cases := []struct {
		name        string
		text        string
		src, dst    string
		handler     http.HandlerFunc
		want        string
		wantErrFlag string
	}{
		{
			name: "rpc signature is deterministic",
			text: "Hello", src: "en", dst: "zh-CN",
			handler: func(w http.ResponseWriter, r *http.Request) {
				_ = r.ParseForm()
				if r.PostForm.Get("Action") != "TranslateGeneral" || r.PostForm.Get("Version") != "2018-10-12" {
					t.Errorf("action/version wrong: %q/%q", r.PostForm.Get("Action"), r.PostForm.Get("Version"))
				}
				if r.PostForm.Get("SignatureMethod") != "HMAC-SHA1" || r.PostForm.Get("SignatureVersion") != "1.0" {
					t.Error("signature method/version wrong")
				}
				if r.PostForm.Get("SignatureNonce") != nonce || r.PostForm.Get("Timestamp") != ts {
					t.Errorf("nonce/timestamp not pinned: %q/%q", r.PostForm.Get("SignatureNonce"), r.PostForm.Get("Timestamp"))
				}
				params := map[string]string{}
				for k, v := range r.PostForm {
					if len(v) > 0 && k != "Signature" {
						params[k] = v[0]
					}
				}
				want := alibabaSignature(secret, params)
				if got := r.PostForm.Get("Signature"); got != want {
					t.Errorf("signature mismatch: got %q want %q", got, want)
				}
				io.WriteString(w, `{"Data":{"Translated":"你好","Lang":"en"},"RequestId":"x"}`)
			},
			want: "你好",
		},
		{
			name: "rpc error code surfaces",
			text: "hi", src: "auto", dst: "en",
			handler: func(w http.ResponseWriter, r *http.Request) {
				io.WriteString(w, `{"Code":"Forbidden","Message":"AccessKeyNotFound"}`)
			},
			wantErrFlag: "AccessKeyNotFound",
		},
		{
			name: "http error surfaces",
			text: "hi", src: "en", dst: "zh-CN",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusForbidden)
				io.WriteString(w, `{"Code":"Throttling"}`)
			},
			wantErrFlag: "HTTP 403",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(tc.handler)
			defer srv.Close()
			alibabaEndpoint = srv.URL

			p, err := New(settings.Provider{Kind: "alibaba", AppID: id, Secret: secret})
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
