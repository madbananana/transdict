package ocr

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestYoudaoWebOCRPostsThePictureAsADataURI(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("方法应为 POST，得到 %s", r.Method)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("解析表单失败: %v", err)
		}
		got = r.PostForm.Get("imgBase")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"lanFrom":"en","lanTo":"zh-CHS","errorCode":"0","lines":[` +
			`{"context":"hello","tranContent":"你好"},` +
			`{"context":"  world  ","tranContent":"世界"},` +
			`{"context":"   ","tranContent":""}]}`))
	}))
	defer srv.Close()

	old := youdaoWebEndpoint
	youdaoWebEndpoint = srv.URL
	t.Cleanup(func() { youdaoWebEndpoint = old })

	png := []byte{0x89, 'P', 'N', 'G', 1, 2, 3}
	blocks, err := YoudaoWebOCR(context.Background(), png)
	if err != nil {
		t.Fatalf("识别失败: %v", err)
	}
	want := "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)
	if got != want {
		t.Errorf("imgBase 应为 %q，得到 %q", want, got)
	}
	if len(blocks) != 2 || blocks[0].Text != "hello" || blocks[1].Text != "world" {
		t.Errorf("应按行去掉空白，得到 %#v", blocks)
	}
}

func TestYoudaoWebOCRReportsVendorErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"errorCode":"113","lines":[]}`))
	}))
	defer srv.Close()

	old := youdaoWebEndpoint
	youdaoWebEndpoint = srv.URL
	t.Cleanup(func() { youdaoWebEndpoint = old })

	if _, err := YoudaoWebOCR(context.Background(), []byte("png")); err == nil ||
		!strings.Contains(err.Error(), "有道 OCR") || !strings.Contains(err.Error(), "113") {
		t.Errorf("应报出服务端错误码，得到 %v", err)
	}
}

func TestYoudaoWebOCRReportsAnEmptyResult(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"errorCode":"0","lines":[]}`))
	}))
	defer srv.Close()

	old := youdaoWebEndpoint
	youdaoWebEndpoint = srv.URL
	t.Cleanup(func() { youdaoWebEndpoint = old })

	if _, err := YoudaoWebOCR(context.Background(), []byte("png")); err == nil {
		t.Error("没有识别出文字时应报错，交给上层保留原始失败")
	}
}
