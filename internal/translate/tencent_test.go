package translate

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"transdict/internal/settings"
)

func TestTencent(t *testing.T) {
	const (
		secretID  = "AKIDexample"
		secretKey = "Gu5t9example"
		fixedTS   = int64(1551113065)
	)
	restoreTS := tencentTimestamp
	tencentTimestamp = func() int64 { return fixedTS }
	defer func() { tencentTimestamp = restoreTS }()

	epRestore := tencentEndpoint
	defer func() { tencentEndpoint = epRestore }()

	// success handler: verify required headers and the deterministic signature,
	// then reply with TargetText.
	success := func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-TC-Action"); got != "TextTranslate" {
			t.Errorf("X-TC-Action = %q", got)
		}
		if got := r.Header.Get("X-TC-Version"); got != "2018-03-21" {
			t.Errorf("X-TC-Version = %q", got)
		}
		if got := r.Header.Get("X-TC-Timestamp"); got != fmt.Sprint(fixedTS) {
			t.Errorf("X-TC-Timestamp = %q want %d", got, fixedTS)
		}
		if got := r.Header.Get("X-TC-Region"); got == "" {
			t.Error("X-TC-Region missing")
		}
		body, _ := io.ReadAll(r.Body)
		want := tencentAuthorization(secretID, secretKey, "tmt", r.Host, fixedTS, body)
		got := r.Header.Get("Authorization")
		if !strings.HasPrefix(got, "TC3-HMAC-SHA256 Credential="+secretID+"/2019-02-25/tmt/tc3_request") {
			t.Errorf("Authorization credential scope wrong: %q", got)
		}
		if got != want {
			t.Errorf("non-deterministic signature: got %q want %q", got, want)
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"Response":{"TargetText":"你好，世界","RequestId":"req-1"}}`)
	}

	failure := func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"Response":{"Error":{"Code":"FailedOperation","Message":"InvalidSecretId"}}}`)
	}

	cases := []struct {
		name        string
		text        string
		src, dst    string
		handler     http.HandlerFunc
		want        string
		wantErrFlag string
	}{
		{name: "signed success", text: "Hello, world", src: "en", dst: "zh-CN", handler: success, want: "你好，世界"},
		{name: "api error surfaced", text: "hi", src: "auto", dst: "en", handler: failure, wantErrFlag: "InvalidSecretId"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(tc.handler)
			defer srv.Close()
			tencentEndpoint = srv.URL

			p, err := New(settings.Provider{Kind: "tencent", AppID: secretID, Secret: secretKey})
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
