package dict

import (
	"strconv"
	"strings"
	"unicode/utf8"
)

// MDict records are HTML. The UI wants readable text, so this file turns markup
// into plain lines with a small hand-written scanner: no regexp, no external
// dependency, and nothing panics on truncated or malformed markup.
//
// stripHTMLTags drops the tags, keeps the text, turns a few layout tags into
// line breaks and decodes the entities dictionaries actually use.
func stripHTMLTags(in string) string {
	var out strings.Builder
	out.Grow(len(in))

	for i := 0; i < len(in); {
		c := in[i]
		switch {
		case c == '<':
			end := strings.IndexByte(in[i:], '>')
			if end < 0 {
				// Unterminated tag: the rest of the record is markup noise.
				return finishLines(out.String())
			}
			tag := in[i+1 : i+end]
			i += end + 1

			if strings.HasPrefix(tag, "!") {
				continue // comment, doctype or CDATA marker
			}
			name, isClose := htmlTagName(tag)
			switch name {
			case "":
				continue
			case "script", "style", "head", "title":
				i = skipUntilClose(in, i, name)
				if isClose {
					continue
				}
			case "br":
				writeLineBreak(&out)
				continue
			}
			if htmlIsBlock(name) {
				writeLineBreak(&out)
			}
			continue

		case c == '&':
			text, width := decodeEntity(in[i:])
			if width == 0 {
				out.WriteByte(c)
				i++
				continue
			}
			out.WriteString(text)
			i += width
			continue

		default:
			r, size := utf8.DecodeRuneInString(in[i:])
			if r == utf8.RuneError && size <= 1 {
				i++ // invalid byte, drop it
				continue
			}
			out.WriteRune(r)
			i += size
		}
	}
	return finishLines(out.String())
}

// writeLineBreak ends a line without stacking blanks: an opening and a closing
// block tag back to back would otherwise leave a gap between two rows.
func writeLineBreak(out *strings.Builder) {
	created := out.String()
	if created == "" || strings.HasSuffix(created, "\n") {
		return
	}
	out.WriteByte('\n')
}

// skipUntilClose moves past the matching </name> for a tag whose content is not
// text, falling back to the end of the record when the file is truncated.
func skipUntilClose(in string, from int, name string) int {
	needle := "</" + name
	for i := from; i+1 < len(in); i++ {
		if in[i] != '<' {
			continue
		}
		if strings.EqualFold(in[i:min(i+len(needle), len(in))], needle) {
			end := strings.IndexByte(in[i:], '>')
			if end < 0 {
				return len(in)
			}
			return i + end + 1
		}
	}
	return len(in)
}

// htmlTagName returns the lower-cased element name of a tag body and whether it
// is a closing tag.
func htmlTagName(tag string) (string, bool) {
	isClose := strings.HasPrefix(tag, "/")
	if isClose {
		tag = tag[1:]
	}
	end := len(tag)
	for i := 0; i < len(tag); i++ {
		c := tag[i]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '/' || c == '=' {
			end = i
			break
		}
	}
	return strings.ToLower(tag[:end]), isClose
}

var htmlBlockTags = map[string]bool{
	"p": true, "div": true, "li": true, "ul": true, "ol": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	"tr": true, "table": true, "td": true, "th": true, "dl": true, "dt": true,
	"dd": true, "blockquote": true, "section": true, "article": true, "hr": true,
	"body": true, "html": true, "form": true, "center": true, "pre": true,
}

func htmlIsBlock(name string) bool { return htmlBlockTags[name] }

// decodeEntity resolves one entity at the head of s and reports how many bytes
// it consumed. Unknown entities are left alone.
func decodeEntity(s string) (string, int) {
	if len(s) < 2 || s[0] != '&' {
		return "", 0
	}
	end := strings.IndexByte(s, ';')
	if end < 0 || end > 12 {
		return "", 0
	}
	name := s[1:end]
	if name == "" {
		return "", 0
	}
	if name[0] == '#' {
		text, ok := decodeNumericEntity(name[1:])
		if !ok {
			return "", 0
		}
		return text, end + 1
	}
	if text, ok := htmlEntities[strings.ToLower(name)]; ok {
		return text, end + 1
	}
	return "", 0
}

func decodeNumericEntity(body string) (string, bool) {
	var (
		value int64
		err   error
	)
	switch {
	case strings.HasPrefix(body, "x"), strings.HasPrefix(body, "X"):
		value, err = strconv.ParseInt(body[1:], 16, 32)
	default:
		value, err = strconv.ParseInt(body, 10, 32)
	}
	if err != nil || value <= 0 || value > utf8.MaxRune {
		return "", false
	}
	return string(rune(value)), true
}

var htmlEntities = map[string]string{
	"lt": "<", "gt": ">", "amp": "&", "quot": "\"", "apos": "'",
	"nbsp": " ", "thinsp": " ", "ensp": " ", "emsp": " ", "middot": "·",
	"copy": "©", "reg": "®", "trade": "™", "deg": "°", "hellip": "…",
	"mdash": "—", "ndash": "–", "lsquo": "‘", "rsquo": "’",
	"ldquo": "“", "rdquo": "”", "times": "×", "divide": "÷",
	"prime": "′", "Prime": "″", "bull": "•", "midd": "·", "dagger": "†",
	"larr": "←", "rarr": "→", "uarr": "↑", "darr": "↓", "euro": "€",
	"pound": "£", "yen": "¥", "cent": "¢", "sect": "§", "para": "¶",
	"alpha": "α", "beta": "β", "gamma": "γ", "delta": "δ", "pi": "π",
	"mu": "μ", "omega": "ω", "sigma": "σ", "lambda": "λ", "degree": "°",
}

// finishLines trims each line, folds runs of spaces, and squeezes blank lines
// so an MDict record reads like a dictionary entry rather than a web page.
func finishLines(in string) string {
	lines := strings.Split(in, "\n")
	out := make([]string, 0, len(lines))
	blank := true
	for _, line := range lines {
		line = collapseSpaces(line)
		if line == "" {
			if !blank && len(out) > 0 {
				out = append(out, "")
				blank = true
			}
			continue
		}
		out = append(out, line)
		blank = false
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	for len(out) > 0 && out[0] == "" {
		out = out[1:]
	}
	return strings.Join(out, "\n")
}

func collapseSpaces(line string) string {
	fields := strings.Fields(line)
	return strings.Join(fields, " ")
}
