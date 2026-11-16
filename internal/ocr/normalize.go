package ocr

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// NormalizeText cleans up the line breaks and spacing a recogniser leaves
// behind, which is 开启 OCR 文本规范化. A screenshot of a wrapped paragraph comes
// back as one block per printed line, a hyphenated word is split across two
// blocks, and Chinese text often carries a space between characters the
// recogniser guessed at. Joining those back up is what makes the text read —
// and translate — like the paragraph it was.
func NormalizeText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = squeezeSpaces(line)
	}

	var out []string
	current := ""
	flush := func() {
		if current != "" {
			out = append(out, current)
			current = ""
		}
	}
	for _, line := range lines {
		switch {
		case line == "":
			flush()
		case current == "":
			current = line
		case startsBlock(line) || endsSentence(current):
			flush()
			current = line
		default:
			current = joinWrapped(current, line)
		}
	}
	flush()
	return strings.Join(out, "\n")
}

// squeezeSpaces turns tabs and runs of spaces into one space, then removes the
// ones that only a recogniser would put around Chinese characters and
// punctuation.
func squeezeSpaces(line string) string {
	var b strings.Builder
	lastSpace := false
	for _, r := range strings.TrimSpace(line) {
		if unicode.IsSpace(r) {
			if !lastSpace {
				b.WriteRune(' ')
			}
			lastSpace = true
			continue
		}
		lastSpace = false
		b.WriteRune(r)
	}
	return trimCJKSpace(b.String())
}

func trimCJKSpace(s string) string {
	runes := []rune(s)
	out := runes[:0]
	for i, r := range runes {
		if r == ' ' && i > 0 && i+1 < len(runes) {
			prev, next := runes[i-1], runes[i+1]
			if (isCJK(prev) && isCJK(next)) ||
				strings.ContainsRune("（「『【《〈“‘", prev) ||
				strings.ContainsRune("），。、；：！？）」』】》〉”’…", next) {
				continue
			}
		}
		out = append(out, r)
	}
	return string(out)
}

func isLatinLetter(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r >= utf8.RuneSelf && unicode.IsLetter(r) && !isCJK(r)
}

// endsSentence reports that the line finished a thought, so the next block
// starts a new one rather than continuing this one.
func endsSentence(s string) bool {
	s = strings.TrimRight(s, "）)」』】》〉”’\"'")
	if s == "" {
		return false
	}
	last, _ := utf8.DecodeLastRuneInString(s)
	return strings.ContainsRune(".!?。！？…；;：:", last)
}

// startsBlock recognises a line that was meant to stand on its own: a bullet
// or a numbered item.
func startsBlock(line string) bool {
	first, size := utf8.DecodeRuneInString(line)
	if strings.ContainsRune("•·▪◦‣", first) {
		return true
	}
	if (first == '-' || first == '*' || first == '#') && strings.HasPrefix(line[size:], " ") {
		return true
	}
	digits := 0
	for _, r := range line {
		if unicode.IsDigit(r) {
			digits++
			continue
		}
		return digits > 0 && (r == '.' || r == ')' || r == '、')
	}
	return false
}

// joinWrapped puts two lines of one paragraph back together: a hyphenated word
// loses its hyphen, Chinese joins without a space, and Latin text gets one.
func joinWrapped(current, next string) string {
	cr := []rune(current)
	nr := []rune(next)
	if len(cr) >= 2 && cr[len(cr)-1] == '-' && isLatinLetter(cr[len(cr)-2]) && isLatinLetter(nr[0]) {
		return string(cr[:len(cr)-1]) + next
	}
	if isCJK(cr[len(cr)-1]) || isCJK(nr[0]) {
		return current + next
	}
	return current + " " + next
}
