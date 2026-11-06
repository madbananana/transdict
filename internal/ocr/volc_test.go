package ocr

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"transdict/internal/settings"
)

func TestVolcRecognizeMapsBlocks(t *testing.T) {
	var gotAuth, gotCT, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotCT = r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		_, _ = w.Write([]byte(cannedVolcResponse))
	}))
	defer srv.Close()

	impl, err := settingsProviderForOCR(settings.Provider{
		Kind: "volc", APIKey: "tk-test", BaseURL: srv.URL,
	})
	if err != nil {
		t.Fatal(err)
	}
	blocks, err := impl.Recognize(context.Background(), []byte("fake-png"))
	if err != nil {
		t.Fatal(err)
	}

	if gotAuth != "Bearer tk-test" {
		t.Fatalf("Authorization = %q", gotAuth)
	}
	if gotCT != "application/json" {
		t.Fatalf("Content-Type = %q", gotCT)
	}
	var body struct {
		Image   string `json:"image"`
		ReqJSON struct {
			DetectPipeline bool `json:"detect_pipeline"`
		} `json:"req_json"`
	}
	if err := json.Unmarshal([]byte(gotBody), &body); err != nil {
		t.Fatalf("body not JSON: %v (raw=%s)", err, gotBody)
	}
	if body.Image != base64.StdEncoding.EncodeToString([]byte("fake-png")) {
		t.Fatalf("image b64 mismatch: %q", body.Image)
	}
	if !body.ReqJSON.DetectPipeline {
		t.Fatal("req_json.detect_pipeline must be true")
	}

	want := []Block{
		{Text: "标题示例", X: 20, Y: 10, Width: 200, Height: 26, Score: 0.999},
		{Text: "second line", X: 30, Y: 50, Width: 150, Height: 20, Score: 0.981},
	}
	if !reflect.DeepEqual(blocks, want) {
		t.Fatalf("blocks = %+v\nwant %+v", blocks, want)
	}
}

func TestVolcPropagatesVendorCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":10001,"message":"ImageInvalid","request_id":"rq-1"}`))
	}))
	defer srv.Close()
	impl, err := settingsProviderForOCR(settings.Provider{
		Kind: "volc", APIKey: "k", BaseURL: srv.URL,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = impl.Recognize(context.Background(), []byte("x"))
	if err == nil {
		t.Fatal("expected error")
	}
	for _, want := range []string{"火山 veOCR", "10001", "ImageInvalid"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("err %q missing %q", err, want)
		}
	}
}

func TestVolcRequiresAPIKey(t *testing.T) {
	_, err := settingsProviderForOCR(settings.Provider{Kind: "volc"})
	if err == nil || !strings.Contains(err.Error(), "缺少 Access Token") {
		t.Fatalf("expected Chinese missing-token error, got %v", err)
	}
}

const cannedVolcResponse = `{
  "code": 10000,
  "message": "OK",
  "request_id": "20240101abc",
  "time_used": 123,
  "data": {
    "line_texts": ["标题示例", "second line"],
    "line_probs": [0.999, 0.981],
    "line_polygon_infos": [
      {"point_1":{"x":20,"y":10},"point_2":{"x":220,"y":10},"point_3":{"x":220,"y":36},"point_4":{"x":20,"y":36}},
      {"point_1":{"x":30,"y":50},"point_2":{"x":180,"y":50},"point_3":{"x":180,"y":70},"point_4":{"x":30,"y":70}}
    ]
  }
}`
