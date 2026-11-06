package gui

import (
	"errors"
	"net/url"
	"strings"
	"unicode/utf8"

	"transdict/internal/query"
	"transdict/internal/settings"
)

// linkLimit bounds the text a URL may carry. A link is meant for a sentence or
// a paragraph; anything longer belongs in the input window, and pasting a whole
// document into the address bar would only wait on eight services.
const linkLimit = 5000

// LinkRequest turns a transdict:// link into a query:
//
//	transdict://query?text=hello&from=en&to=zh-CN
//
// from/to are optional and fall back to the stored languages. Nothing else in
// the app is reachable from a link — it opens a translation and stops there.
func LinkRequest(raw string) (query.Request, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return query.Request{}, errors.New("无法解析链接")
	}
	if !strings.EqualFold(u.Scheme, "transdict") {
		return query.Request{}, errors.New("不是 transdict 链接")
	}
	q := u.Query()
	text := q.Get("text")
	if text == "" {
		// q is what most query links use; accepting both costs one line.
		text = q.Get("q")
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return query.Request{}, errors.New("链接里没有要翻译的文字")
	}
	if utf8.RuneCountInString(text) > linkLimit {
		text = string([]rune(text)[:linkLimit])
	}
	return query.Request{
		Context:    settings.ContextFloating,
		Mode:       "url",
		Text:       text,
		SourceLang: q.Get("from"),
		TargetLang: q.Get("to"),
	}, nil
}

// OpenLink answers a transdict:// link in the floating panel, which is the
// window someone calling the app from another program expects to see.
func (s *Service) OpenLink(raw string) error {
	req, err := LinkRequest(raw)
	if err != nil {
		return err
	}
	return s.Ask(req)
}
