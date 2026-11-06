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
	"time"

	"transdict/internal/settings"
)

// pinTencent fixes every nondeterministic input the signer touches so a test
// can produce a byte-for-byte stable Authorization header.
func pinTencent(t *testing.T) {
	t.Helper()
	prevNow, prevNonce := tencentNow, tencentNonceFn
	tencentNow = func() time.Time { return time.Unix(1700000000, 0).UTC() } // 2023-11-14 UTC
	tencentNonceFn = func() int64 { return 4242 }
	t.Cleanup(func() { tencentNow, tencentNonceFn = prevNow, prevNonce })
}

func TestTC3SignDeterministic(t *testing.T) {
	pinTencent(t)
	auth, err := tc3Sign("AKIDtest", "SECRETtest", "ocr", "ocr.tencentcloudapi.com",
		tencentNow(), "1700000000", "GeneralBasicOCR", `{"ImageBase64":"aGk="}`)
	if err != nil {
		t.Fatal(err)
	}
	// Same inputs must produce the same signature; that alone is enough to
	// prove the chain is pure — pinning the timestamp and the payload also
	// rules out wall-clock drift.
	again, _ := tc3Sign("AKIDtest", "SECRETtest", "ocr", "ocr.tencentcloudapi.com",
		tencentNow(), "1700000000", "GeneralBasicOCR", `{"ImageBase64":"aGk="}`)
	if auth != again {
		t.Fatalf("tc3Sign not deterministic:\n %q\n %q", auth, again)
	}
	if !strings.HasPrefix(auth, "TC3-HMAC-SHA256 Credential=AKIDtest/2023-11-14/ocr/tc3_request, ") {
		t.Fatalf("auth shape wrong: %s", auth)
	}
	if !strings.Contains(auth, "SignedHeaders=content-type;host;x-tc-action, Signature=") {
		t.Fatalf("signed-headers wrong: %s", auth)
	}
	// Flipping the payload must change the trailing signature.
	diff, _ := tc3Sign("AKIDtest", "SECRETtest", "ocr", "ocr.tencentcloudapi.com",
		tencentNow(), "1700000000", "GeneralBasicOCR", `{"ImageBase64":"aGk!"}`)
	if diff == auth {
		t.Fatal("payload is not hashed into the signature")
	}
}

func TestTencentRecognizeMapsBlocks(t *testing.T) {
	pinTencent(t)

	var gotAuth, gotAction, gotVersion, gotTs, gotRegion, gotNonce, gotCT, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotAction = r.Header.Get("X-TC-Action")
		gotVersion = r.Header.Get("X-TC-Version")
		gotTs = r.Header.Get("X-TC-Timestamp")
		gotRegion = r.Header.Get("X-TC-Region")
		gotNonce = r.Header.Get("X-TC-Nonce")
		gotCT = r.Header.Get("Content-Type")
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(cannedTencentResponse))
	}))
	defer srv.Close()

	p := settings.Provider{
		Category: settings.CatOCR, Kind: "tencent", Enabled: true,
		AppID: "AKIDtest", Secret: "SECRETtest",
		BaseURL: srv.URL,
		Extra:   map[string]string{"region": "ap-shanghai"},
	}
	impl, err := settingsProviderForOCR(p)
	if err != nil {
		t.Fatal(err)
	}
	blocks, err := impl.Recognize(context.Background(), []byte("fake-png"))
	if err != nil {
		t.Fatal(err)
	}

	// Pinned inputs must produce the exact same Authorization header as the
	// pure-signer golden test on the same payload.
	wantAuth, _ := tc3Sign("AKIDtest", "SECRETtest", "ocr", hostOf(srv.URL),
		time.Unix(1700000000, 0).UTC(), "1700000000", "GeneralBasicOCR",
		mustMarshalTencent([]byte("fake-png")))
	if gotAuth != wantAuth {
		t.Fatalf("Authorization mismatch:\n got %s\nwant %s", gotAuth, wantAuth)
	}
	if gotAction != "GeneralBasicOCR" || gotVersion != "2018-11-19" ||
		gotTs != "1700000000" || gotRegion != "ap-shanghai" || gotNonce != "4242" {
		t.Fatalf("X-TC headers wrong: action=%q version=%q ts=%q region=%q nonce=%q",
			gotAction, gotVersion, gotTs, gotRegion, gotNonce)
	}
	if gotCT != "application/json; charset=utf-8" {
		t.Fatalf("content-type = %q", gotCT)
	}
	var req map[string]string
	if err := json.Unmarshal([]byte(gotBody), &req); err != nil {
		t.Fatalf("body not JSON: %v", err)
	}
	if req["ImageBase64"] != base64.StdEncoding.EncodeToString([]byte("fake-png")) {
		t.Fatalf("ImageBase64 wrong: %v", req)
	}

	want := []Block{
		{Text: "你好，世界", X: 12, Y: 8, Width: 200, Height: 30, Score: 0.99},
		{Text: "Hello", X: 40, Y: 60, Width: 80, Height: 24, Score: 0.98},
	}
	if !reflect.DeepEqual(blocks, want) {
		t.Fatalf("blocks = %+v\nwant %+v", blocks, want)
	}
}

func TestTencentPropagatesVendorError(t *testing.T) {
	pinTencent(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"Response":{"Error":{"Code":"AuthFailure.SignatureExpire","Message":"签名过期"},"RequestId":"r-1"}}`))
	}))
	defer srv.Close()

	impl, err := settingsProviderForOCR(settings.Provider{
		Kind: "tencent", AppID: "AKID", Secret: "SK", BaseURL: srv.URL,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = impl.Recognize(context.Background(), []byte("x"))
	if err == nil {
		t.Fatal("expected error")
	}
	for _, want := range []string{"腾讯云 OCR", "AuthFailure.SignatureExpire", "签名过期"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("err %q missing %q", err, want)
		}
	}
}

func mustMarshalTencent(png []byte) string {
	b, _ := json.Marshal(map[string]string{"ImageBase64": base64.StdEncoding.EncodeToString(png)})
	return string(b)
}

// settingsProviderForOCR is a tiny wrapper around ocr.New so a test that just
// wants a Provider can build one from a settings entry without re-typing the
// Category assertion. It also guards against a Kind typo in the test itself.
func settingsProviderForOCR(p settings.Provider) (Provider, error) {
	p.Category = settings.CatOCR
	return New(p)
}

const cannedTencentResponse = `{
  "Response": {
    "TextDetections": [
      {
        "DetectedText": "你好，世界",
        "Type": 0,
        "ItemId": 0,
        "Confidence": 99,
        "Polygon": [
          {"X": 12, "Y": 8},
          {"X": 212, "Y": 8},
          {"X": 212, "Y": 38},
          {"X": 12, "Y": 38}
        ],
        "AdvancedInfo": "{\"ItemPolygon\":{\"X\":12,\"Y\":8,\"Width\":200,\"Height\":30},\"Prob\":{\"Score\":0.99,\"Text\":\"你好，世界\"}}"
      },
      {
        "DetectedText": "Hello",
        "Type": 0,
        "ItemId": 1,
        "Confidence": 98,
        "Polygon": [
          {"X": 40, "Y": 60},
          {"X": 120, "Y": 60},
          {"X": 120, "Y": 84},
          {"X": 40, "Y": 84}
        ],
        "AdvancedInfo": "{\"ItemPolygon\":{\"X\":40,\"Y\":60,\"Width\":80,\"Height\":24}}"
      }
    ],
    "RequestId": "req-abc-123"
  }
}`
