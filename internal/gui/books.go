package gui

import (
	"regexp"
	"strings"

	"transdict/internal/selection"
)

// Apple Books wraps a copied highlight in curly quotes and appends a copyright
// notice. Translating that notice wastes the services on a paragraph nobody
// selected, so the notice is dropped before the query starts.
const booksAppBundle = "com.apple.iBooksX"

// booksExcerptPatterns follow Easydict's two notices: the English one, and the
// Chinese one whose wording changed over time — only the part shared by the
// simplified and traditional forms is required.
var booksExcerptPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?s)^“(.+?)”\s+Excerpt From.+?This material may be protected by copyright\.\s*$`),
	regexp.MustCompile(`(?s)^“(.+?)”\s+摘.+?版[权權]保[护護]。\s*$`),
}

// stripBooksExcerpt returns the quoted excerpt, or the text unchanged when it
// does not carry a Books notice.
func stripBooksExcerpt(text string) string {
	for _, re := range booksExcerptPatterns {
		if m := re.FindStringSubmatch(text); m != nil {
			return strings.TrimSpace(m[1])
		}
	}
	return text
}

// frontmostBundleID is a variable so the cleanup can be tested without a
// window server.
var frontmostBundleID = selection.FrontmostBundleID

// booksExcerptText applies 移除 Books 摘录版权信息 to a selection. The notice is
// only removed when the text really came from Books: the same wording pasted
// from elsewhere is the user's own text, not something to rewrite.
func (s *Service) booksExcerptText(text string) string {
	if text == "" || !s.cfg.Get().RemoveBooksExcerpt {
		return text
	}
	if frontmostBundleID() != booksAppBundle {
		return text
	}
	return stripBooksExcerpt(text)
}
