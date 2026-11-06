package ocr

import (
	"strings"
	"unicode"
)

// trimEdge is the punctuation a word run can pick up at its edges: typographic
// quotes, the periods of an abbreviation, list bullets.
const trimEdge = "-_'’.\"“”()[]{}<>:;,!?/\\|`~*#"

// WordAt picks the word under the pointer among recognized blocks, in the same
// pixel space as the cropped image. 鼠标取词 works on anything the accessibility
// API cannot see — a PDF viewer, a canvas, a video — because it reads pixels.
//
// Glyph positions inside a block are approximated linearly across its width.
// That is imprecise by a character or two, and it stays correct in practice
// because a pointer sits inside a word, not on its boundary.
func WordAt(blocks []Block, x, y int) string {
	block := blockAt(blocks, x, y)
	if block == nil {
		return ""
	}
	return wordIn(*block, x-block.X)
}

// blockAt prefers the block containing the point, then the nearest block on the
// same line: a tight crop can put the pointer a pixel below the glyphs.
func blockAt(blocks []Block, x, y int) *Block {
	var best *Block
	bestGap := 1 << 30
	for i := range blocks {
		b := blocks[i]
		if strings.TrimSpace(b.Text) == "" {
			continue
		}
		gap := 0
		switch {
		case y < b.Y:
			gap = b.Y - y
		case y > b.Y+b.Height:
			gap = y - (b.Y + b.Height)
		}
		// Half a line of slack either way keeps a two-line crop from answering
		// with the wrong line.
		if gap > maxInt(b.Height, 12)/2+2 {
			continue
		}
		if gap < bestGap {
			bestGap, best = gap, &blocks[i]
		}
	}
	return best
}

func wordIn(block Block, localX int) string {
	runes := []rune(block.Text)
	if len(runes) == 0 {
		return ""
	}
	width := block.Width
	if width <= 0 {
		// Providers that report no geometry still order their blocks, so a
		// fixed advance keeps the estimate monotonic.
		width = len(runes) * 12
	}

	index := int(float64(localX) * float64(len(runes)) / float64(width))
	if index < 0 {
		index = 0
	}
	if index >= len(runes) {
		index = len(runes) - 1
	}
	// A pointer in the gap between two words belongs to the one on its left.
	for index > 0 && unicode.IsSpace(runes[index]) {
		index--
	}
	if unicode.IsSpace(runes[index]) {
		return ""
	}

	// Chinese, Japanese and Korean have no spaces, so a run of those scripts is
	// the unit a reader means by "this word".
	cluster := isCJK
	if !isCJK(runes[index]) {
		cluster = isLatinPart
	}
	start, end := index, index
	for start > 0 && cluster(runes[start-1]) {
		start--
	}
	for end+1 < len(runes) && cluster(runes[end+1]) {
		end++
	}
	return strings.Trim(string(runes[start:end+1]), trimEdge)
}

func isCJK(r rune) bool {
	return unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul)
}

func isLatinPart(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '\'' || r == '-' || r == '_' ||
		unicode.In(r, unicode.Mn, unicode.Mc)
}
