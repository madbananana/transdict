package gui

import (
	"strings"
	"testing"

	"transdict/internal/settings"
)

func TestLinkRequestReadsTextAndLanguages(t *testing.T) {
	req, err := LinkRequest("transdict://query?text=%E4%BD%A0%E5%A5%BD&from=zh-CN&to=en")
	if err != nil {
		t.Fatalf("LinkRequest 失败: %v", err)
	}
	if req.Text != "你好" || req.SourceLang != "zh-CN" || req.TargetLang != "en" {
		t.Errorf("请求 = %+v", req)
	}
	// The floating panel is where a link from another program should land, and
	// the mode is what tells the panel the text did not come from its own box.
	if req.Context != settings.ContextFloating || req.Mode != "url" {
		t.Errorf("context=%q mode=%q, want floating/url", req.Context, req.Mode)
	}
}

func TestLinkRequestAcceptsTheShortQueryKey(t *testing.T) {
	req, err := LinkRequest("transdict://query?q=hello+world")
	if err != nil {
		t.Fatalf("LinkRequest 失败: %v", err)
	}
	if req.Text != "hello world" {
		t.Errorf("text = %q", req.Text)
	}
	if req.SourceLang != "" || req.TargetLang != "" {
		t.Errorf("未指定语言时不应编造: %+v", req)
	}
}

func TestLinkRequestRejectsLinksItCannotAnswer(t *testing.T) {
	cases := []string{
		"https://example.com/query?text=hi",
		"transdict://query",
		"transdict://query?text=%20%20",
		"transdict://query?text=%zz",
	}
	for _, raw := range cases {
		if _, err := LinkRequest(raw); err == nil {
			t.Errorf("LinkRequest(%q) 应报错", raw)
		}
	}
}

func TestLinkRequestClampsALongText(t *testing.T) {
	raw := "transdict://query?text=" + strings.Repeat("a", linkLimit+50)
	req, err := LinkRequest(raw)
	if err != nil {
		t.Fatalf("LinkRequest 失败: %v", err)
	}
	if len([]rune(req.Text)) != linkLimit {
		t.Errorf("长度 = %d, want %d", len([]rune(req.Text)), linkLimit)
	}
}
