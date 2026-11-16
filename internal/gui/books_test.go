package gui

import (
	"testing"

	"transdict/internal/settings"
)

const (
	booksExcerptEN = "“The quick brown fox\njumps over the lazy dog.”\nExcerpt From\nThe Fox Book\nThis material may be protected by copyright.\n"
	booksExcerptZH = "“今天天气很好，我们去公园吧。” 摘录来自《某书》 此内容可能受版权保护。"
	booksExcerptTW = "“今天天氣很好，我們去公園吧。” 摘 某書 此內容可能受版權保護。"
)

func TestStripBooksExcerptKeepsTheQuotedText(t *testing.T) {
	cases := map[string]string{
		booksExcerptEN: "The quick brown fox\njumps over the lazy dog.",
		booksExcerptZH: "今天天气很好，我们去公园吧。",
		booksExcerptTW: "今天天氣很好，我們去公園吧。",
		"普通文本":         "普通文本",
	}
	for in, want := range cases {
		if got := stripBooksExcerpt(in); got != want {
			t.Errorf("stripBooksExcerpt(%q) = %q, want %q", in, got, want)
		}
	}
}

// The notice is only rewritten for a selection that came from Books; the same
// wording pasted into a text field is the user's own text.
func TestBooksExcerptOnlyForBooksSelections(t *testing.T) {
	store := testStore(t)
	s := &Service{cfg: store}
	old := frontmostBundleID
	defer func() { frontmostBundleID = old }()

	frontmostBundleID = func() string { return booksAppBundle }
	if got := s.booksExcerptText(booksExcerptZH); got != "今天天气很好，我们去公园吧。" {
		t.Errorf("Books selection = %q, want the quoted text", got)
	}

	frontmostBundleID = func() string { return "com.apple.Safari" }
	if got := s.booksExcerptText(booksExcerptZH); got != booksExcerptZH {
		t.Errorf("non-Books selection = %q, want it untouched", got)
	}

	frontmostBundleID = func() string { return booksAppBundle }
	if _, err := store.Update(func(c *settings.Config) { c.RemoveBooksExcerpt = false }); err != nil {
		t.Fatalf("设置: %v", err)
	}
	if got := s.booksExcerptText(booksExcerptZH); got != booksExcerptZH {
		t.Errorf("关闭开关后 = %q, want it untouched", got)
	}
}
