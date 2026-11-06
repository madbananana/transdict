package translate

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"transdict/internal/settings"
)

func TestVolc(t *testing.T) {
	const apiKey = "volc-token-123"

	baseRestore := volcBase
	defer func() { volcBase = baseRestore }()

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
			name: "translated_list layout",
			text: "Hello", src: "en", dst: "zh-CN",
			handler: func(w http.ResponseWriter, r *http.Request) {
				if got := r.Header.Get("Authorization"); got != apiKey {
					t.Errorf("Authorization = %q want %q", got, apiKey)
				}
				if ct := r.Header.Get("Content-Type"); ct != "application/json" {
					t.Errorf("Content-Type = %q", ct)
				}
				body, _ := io.ReadAll(r.Body)
				var req map[string]any
				if err := json.Unmarshal(body, &req); err != nil {
					t.Fatalf("bad request json: %v", err)
				}
				if req["source_language"] != "en" || req["target_language"] != "zh" || req["text"] != "Hello" {
					t.Errorf("request body wrong: %+v", req)
				}
				io.WriteString(w, `{"code":0,"message":"success","data":{"translated_list":[["你好"]],"detected_language_list":["en"]}}`)
			},
			want:       "你好",
			wantDetect: "en",
		},
		{
			name: "translation_list layout with empty source",
			text: "Hello", src: "auto", dst: "ja",
			handler: func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				var req map[string]any
				_ = json.Unmarshal(body, &req)
				if s, _ := req["source_language"].(string); s != "" {
					t.Errorf("auto should send empty source_language, got %q", s)
				}
				io.WriteString(w, `{"code":0,"data":{"translation_list":[{"translation":"こんにちは","detected_source_language":"en"}]}}`)
			},
			want:       "こんにちは",
			wantDetect: "en",
		},
		{
			name: "non-zero code with no data is an error",
			text: "hi", src: "en", dst: "zh-CN",
			handler: func(w http.ResponseWriter, r *http.Request) {
				io.WriteString(w, `{"code":10001,"message":"Invalid AppKey"}`)
			},
			wantErrFlag: "Invalid AppKey",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(tc.handler)
			defer srv.Close()
			volcBase = srv.URL

			p, err := New(settings.Provider{Kind: "volc", APIKey: apiKey})
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
			if out != tc.want || detected != tc.wantDetect {
				t.Fatalf("got (%q,%q) want (%q,%q)", out, detected, tc.want, tc.wantDetect)
			}
		})
	}
}
