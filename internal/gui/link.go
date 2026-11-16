package gui

import (
	"errors"
	"log"
	"net/url"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"unicode/utf8"

	"transdict/internal/query"
	"transdict/internal/settings"
)

// linkLimit bounds the text a URL may carry. A link is meant for a sentence or
// a paragraph; anything longer belongs in the input window, and pasting a whole
// document into the address bar would only wait on eight services.
const linkLimit = 5000

// ProjectURL is where the source lives, which 关于 links to.
const ProjectURL = "https://github.com/madbananana/transdict"

// ContributorsURL is the GitHub contributor graph, 关于's 贡献者 entry.
const ContributorsURL = ProjectURL + "/graphs/contributors"

// FeedbackURL is the issue list, the tray's 反馈 entry: a question or a bug report
// belongs next to the code, where the next user can find the answer too.
const FeedbackURL = ProjectURL + "/issues"

// OpenProjectPage opens the repository in the default browser. It is not a
// query: the 关于 page needs the browser, not a translation result.
func (s *Service) OpenProjectPage() error {
	return s.openURL(ProjectURL)
}

// OpenFeedbackPage opens the issue tracker, mirroring EasyDict's 反馈 entry.
func (s *Service) OpenFeedbackPage() error {
	return s.openURL(FeedbackURL)
}

// OpenContributorsPage opens the contributor graph, mirroring EasyDict's 关于
// row of the same name.
func (s *Service) OpenContributorsPage() error {
	return s.openURL(ContributorsURL)
}

func (s *Service) openURL(raw string) error {
	if s.wails == nil {
		return errors.New("浏览器不可用")
	}
	return s.wails.Browser.OpenURL(raw)
}

// quickLinkPattern is what the titlebar chips may hand to the system: the
// browser's two schemes and the dictionary apps' own.
var quickLinkPattern = regexp.MustCompile(`(?i)^(https?://|dict://|eudic://)`)

// OpenQuickLink opens a 快捷链接 and reports whether the system found anything
// to open it with. Wails' Browser.OpenURL starts `open` and never waits for it,
// so an uninstalled Eudic comes back as success and the 欧 chip looks dead;
// waiting for the command turns that into an error the window can show.
func (s *Service) OpenQuickLink(raw string) error {
	raw = strings.TrimSpace(raw)
	if !quickLinkPattern.MatchString(raw) {
		return errors.New("无法打开链接")
	}
	if runtime.GOOS != "darwin" {
		return s.openURL(raw)
	}
	if s.wails == nil {
		return errors.New("浏览器不可用")
	}
	if out, err := exec.Command("open", raw).CombinedOutput(); err != nil {
		log.Printf("transdict: open %s: %v: %s", raw, err, strings.TrimSpace(string(out)))
		switch {
		case strings.HasPrefix(strings.ToLower(raw), "eudic://"):
			return errors.New("未找到欧路词典，请确认已安装")
		case strings.HasPrefix(strings.ToLower(raw), "dict://"):
			return errors.New("无法打开系统词典")
		default:
			return errors.New("无法打开浏览器")
		}
	}
	return nil
}

// LinkRequest turns a transdict:// link into a query:
//
//	transdict://query?text=hello&from=en&to=zh-CN
//	transdict://good
//	transdict://good/girl
//
// The bare forms are EasyDict's: whatever follows the scheme is the sentence,
// which is what Alfred and Raycast send without knowing the parameter names.
// from/to are optional and fall back to the stored languages.
func LinkRequest(raw string) (query.Request, error) {
	raw = strings.TrimSpace(raw)
	const scheme = "transdict://"
	if len(raw) < len(scheme) || !strings.EqualFold(raw[:len(scheme)], scheme) {
		return query.Request{}, errors.New("不是 transdict 链接")
	}
	rest := raw[len(scheme):]
	if i := strings.IndexByte(rest, '#'); i >= 0 {
		rest = rest[:i]
	}
	action, params := rest, url.Values{}
	if i := strings.IndexByte(rest, '?'); i >= 0 {
		action = rest[:i]
		q, err := url.ParseQuery(encodeBareAmpersands(rest[i+1:]))
		if err != nil {
			return query.Request{}, errors.New("无法解析链接")
		}
		params = q
	}
	text := ""
	if strings.EqualFold(action, "query") {
		text = params.Get("text")
		if text == "" {
			// q is what most query links use; accepting both costs one line.
			text = params.Get("q")
		}
	} else {
		// transdict://good and transdict://good%2Fgirl: the action is the text.
		decoded, err := url.PathUnescape(action)
		if err != nil {
			return query.Request{}, errors.New("无法解析链接")
		}
		text = decoded
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return query.Request{}, errors.New("链接里没有要翻译的文字")
	}
	if utf8.RuneCountInString(text) > linkLimit {
		text = string([]rune(text)[:linkLimit])
	}
	return query.Request{
		Context:    settings.ContextMain,
		Mode:       "url",
		Text:       text,
		SourceLang: params.Get("from"),
		TargetLang: params.Get("to"),
	}, nil
}

// linkParams are the keys this app reads; an & before one of them separates
// parameters, everything else is part of the sentence.
var linkParams = []string{"text=", "q=", "from=", "to="}

// encodeBareAmpersands quotes the & that belongs to a sentence rather than to
// the next parameter. EasyDict encodes every & before routing (issue #761),
// because translators are routinely handed "R&D" or "A & B" unescaped; this
// mirrors that without losing the from/to keys this app added.
func encodeBareAmpersands(query string) string {
	var b strings.Builder
	for i := 0; i < len(query); i++ {
		if query[i] != '&' {
			b.WriteByte(query[i])
			continue
		}
		rest := strings.ToLower(query[i+1:])
		known := false
		for _, key := range linkParams {
			if strings.HasPrefix(rest, key) {
				known = true
				break
			}
		}
		if known {
			b.WriteByte('&')
		} else {
			b.WriteString("%26")
		}
	}
	return b.String()
}

// OpenLink answers a transdict:// link in the translation window.
func (s *Service) OpenLink(raw string) error {
	req, err := LinkRequest(raw)
	if err != nil {
		return err
	}
	return s.Ask(req)
}
