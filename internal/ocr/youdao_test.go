package ocr

import (
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"transdict/internal/settings"
)

func TestYoudaoSignInputAndDigest(t *testing.T) {
	// Short input (<=20 runes) must be hashed verbatim so a test can pin the
	// whole digest independently of the truncation rule.
	got := youdaoSignInput("AK", "hello", "10000000", "1700000000", "SEC")
	want := "AKhello10000000" + "1700000000" + "SEC"
	if got != want {
		t.Fatalf("signInput = %q, want %q", got, want)
	}
	sum := md5.Sum([]byte(want))
	if got := youdaoSignMD5("AK", "hello", "10000000", "1700000000", "SEC"); got != hex.EncodeToString(sum[:]) {
		t.Fatalf("MD5 mismatch: got %s want %s", got, hex.EncodeToString(sum[:]))
	}

	// Long input follows the vendor's "first-10 + rune-count + last-10" rule.
	long := strings.Repeat("a", 10) + "MIDDLE" + strings.Repeat("z", 10) // 26 runes
	in := youdaoSignInput("AK", long, "10000000", "1700000000", "SEC")
	wantIn := "AK" + "aaaaaaaaaa" + strconv.Itoa(26) + "zzzzzzzzzz" + "10000000" + "1700000000" + "SEC"
	if in != wantIn {
		t.Fatalf("truncated signInput = %q\nwant %q", in, wantIn)
	}

	// Non-ASCII base64 never contains multibyte runes, but the doc uses rune
	// counts; assert youdaoTruncate agrees with utf8 length, not byte length.
	uni := "汉字测试" + strings.Repeat("x", 20) // 24 runes, way more bytes
	trunc := youdaoTruncate(uni)
	if runeCount(trunc) != 10+len(strconv.Itoa(24))+10 {
		t.Fatalf("truncated length wrong: %q", trunc)
	}
	if !strings.HasPrefix(trunc, "汉字测试xxxxx") || !strings.HasSuffix(trunc, "xxxxxxxxx") {
		t.Fatalf("truncation shape wrong: %q", trunc)
	}
}

func runeCount(s string) int { return len([]rune(s)) }

func TestYoudaoRecognizeMapsBlocks(t *testing.T) {
	prevNow, prevSalt := youdaoNow, youdaoSaltFn
	youdaoNow = func() time.Time { return time.Unix(1700000000, 0).UTC() }
	youdaoSaltFn = func() string { return "10000000" }
	t.Cleanup(func() { youdaoNow, youdaoSaltFn = prevNow, prevSalt })

	var form url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		form = r.PostForm
		_, _ = w.Write([]byte(cannedYoudaoResponse))
	}))
	defer srv.Close()

	impl, err := settingsProviderForOCR(settings.Provider{
		Kind: "youdaoocr", AppID: "APPID", Secret: "SECRET", BaseURL: srv.URL,
		Extra: map[string]string{"docType": "jpg"},
	})
	if err != nil {
		t.Fatal(err)
	}
	blocks, err := impl.Recognize(context.Background(), []byte("fake-png"))
	if err != nil {
		t.Fatal(err)
	}

	if v := form.Get("appKey"); v != "APPID" {
		t.Fatalf("appKey = %q", v)
	}
	if v := form.Get("salt"); v != "10000000" {
		t.Fatalf("salt = %q", v)
	}
	if v := form.Get("curtime"); v != "1700000000" {
		t.Fatalf("curtime = %q", v)
	}
	if v := form.Get("docType"); v != "jpg" {
		t.Fatalf("docType = %q", v)
	}
	q := form.Get("q")
	if q != base64.StdEncoding.EncodeToString([]byte("fake-png")) {
		t.Fatalf("q = %q", q)
	}
	wantVK := youdaoSignMD5("APPID", q, "10000000", "1700000000", "SECRET")
	if v := form.Get("vk"); v != wantVK {
		t.Fatalf("vk = %q, want %q", v, wantVK)
	}

	want := []Block{
		{Text: "第一行", X: 10, Y: 10, Width: 100, Height: 30, Score: 0.92},
		{Text: "第二行", X: 10, Y: 50, Width: 100, Height: 30, Score: 0.93},
	}
	if !reflect.DeepEqual(blocks, want) {
		t.Fatalf("blocks = %+v\nwant %+v", blocks, want)
	}
}

func TestYoudaoPropagatesErrorCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"errorCode":"105","reason":"参数错误"}`))
	}))
	defer srv.Close()
	impl, err := settingsProviderForOCR(settings.Provider{
		Kind: "youdaoocr", AppID: "A", Secret: "S", BaseURL: srv.URL,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = impl.Recognize(context.Background(), []byte("x"))
	if err == nil {
		t.Fatal("expected error")
	}
	for _, want := range []string{"有道 OCR", "105", "参数错误"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("err %q missing %q", err, want)
		}
	}
}

const cannedYoudaoResponse = `{
  "errorCode": "0",
  "imgWidth": "400",
  "imgHeight": "200",
  "Text": [
    {
      "cont": [
        {
          "textContent": "第一行",
          "coords": [10,10, 110,10, 110,40, 10,40],
          "prob": 92
        }
      ]
    },
    {
      "cont": [
        {
          "textContent": "第二行",
          "coords": [10,50, 110,50, 110,80, 10,80],
          "prob": 93
        }
      ]
    }
  ]
}`
