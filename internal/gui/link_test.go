package gui

import (
	"strings"
	"testing"

	"transdict/internal/settings"
)

// 关于's 贡献者 button belongs to the same repository page, so the URL is
// derived rather than typed twice in the settings page. 反馈 is the issue list
// of the same repository, for the same reason.
func TestRepositoryLinksAreDerived(t *testing.T) {
	if ContributorsURL != ProjectURL+"/graphs/contributors" {
		t.Errorf("ContributorsURL = %q", ContributorsURL)
	}
	if FeedbackURL != ProjectURL+"/issues" {
		t.Errorf("FeedbackURL = %q", FeedbackURL)
	}
}

// Quick links are handed to the system, so the schemes are the only door: a
// file:// or javascript: URL must be refused before `open` ever sees it.
func TestOpenQuickLinkRefusesOtherSchemes(t *testing.T) {
	s := &Service{}
	for _, raw := range []string{"file:///etc/passwd", "javascript:alert(1)", "transdict://good", ""} {
		if err := s.OpenQuickLink(raw); err == nil {
			t.Errorf("OpenQuickLink(%q) 没有拒绝", raw)
		}
	}
}

func TestLinkRequestReadsTextAndLanguages(t *testing.T) {
	req, err := LinkRequest("transdict://query?text=%E4%BD%A0%E5%A5%BD&from=zh-CN&to=en")
	if err != nil {
		t.Fatalf("LinkRequest 失败: %v", err)
	}
	if req.Text != "你好" || req.SourceLang != "zh-CN" || req.TargetLang != "en" {
		t.Errorf("请求 = %+v", req)
	}
	// A link from another program lands in the translation window, and the mode
	// is what tells the window the text did not come from its own box.
	if req.Context != settings.ContextMain || req.Mode != "url" {
		t.Errorf("context=%q mode=%q, want main/url", req.Context, req.Mode)
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

// EasyDict also answers easydict://good and easydict://good%2Fgirl, which is
// what scripts send when they do not know about the query key.
func TestLinkRequestAcceptsTheBareWordForm(t *testing.T) {
	cases := map[string]string{
		"transdict://good":               "good",
		"transdict://good/girl":          "good/girl",
		"transdict://good%2Fgirl":        "good/girl",
		"transdict://%E4%BD%A0%E5%A5%BD": "你好",
		"transdict://Good":               "Good",
		"transdict://good?text=other":    "good",
	}
	for raw, want := range cases {
		req, err := LinkRequest(raw)
		if err != nil {
			t.Errorf("LinkRequest(%q) 失败: %v", raw, err)
			continue
		}
		if req.Text != want || req.Mode != "url" {
			t.Errorf("LinkRequest(%q) = %q/%q, want %q/url", raw, req.Text, req.Mode, want)
		}
	}
}

// A sentence with a bare & (R&D, Q&A) is what EasyDict encodes before routing;
// otherwise the query would stop at the ampersand.
func TestLinkRequestKeepsAmpersandsInText(t *testing.T) {
	cases := []struct {
		raw  string
		text string
		from string
	}{
		{"transdict://query?text=Agent Mode & Auto Context", "Agent Mode & Auto Context", ""},
		{"transdict://query?text=R&D&from=en", "R&D", "en"},
		{"transdict://query?from=en&text=Q&A&to=zh-CN", "Q&A", "en"},
		{"transdict://query?text=R%26D", "R&D", ""},
		{"transdict://query?text=hi#frag", "hi", ""},
	}
	for _, c := range cases {
		req, err := LinkRequest(c.raw)
		if err != nil {
			t.Errorf("LinkRequest(%q) 失败: %v", c.raw, err)
			continue
		}
		if req.Text != c.text || req.SourceLang != c.from {
			t.Errorf("LinkRequest(%q) = %q/%q, want %q/%q", c.raw, req.Text, req.SourceLang, c.text, c.from)
		}
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
