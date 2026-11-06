package ocr

import "testing"

func line(text string, y, height int) Block {
	// Twelve pixels per rune keeps the linear approximation in wordIn exact
	// enough to assert on.
	return Block{Text: text, X: 100, Y: y, Width: len([]rune(text)) * 12, Height: height}
}

func TestWordAtPicksTheWordUnderThePointer(t *testing.T) {
	blocks := []Block{line("the quick brown fox", 0, 20)}

	for _, tc := range []struct {
		x    int
		want string
	}{
		{100 + 12, "the"},
		{100 + 60, "quick"},
		{100 + 140, "brown"},
		{100 + 200, "fox"},
		// A pointer in the gap resolves to the word on its left.
		{100 + 40, "the"},
	} {
		if got := WordAt(blocks, tc.x, 8); got != tc.want {
			t.Errorf("WordAt(x=%d) = %q, want %q", tc.x, got, tc.want)
		}
	}
}

func TestWordAtChoosesTheNearestLine(t *testing.T) {
	blocks := []Block{line("first line here", 0, 20), line("second line text", 30, 20)}

	if got := WordAt(blocks, 130, 40); got != "second" {
		t.Errorf("y=40 → %q", got)
	}
	// A pixel or two past the bottom of the upper line still belongs to it.
	if got := WordAt(blocks, 130, 22); got != "first" {
		t.Errorf("y=22 → %q", got)
	}
	// Far from every line means no answer, not a guess.
	if got := WordAt(blocks, 130, 200); got != "" {
		t.Errorf("y=200 → %q, want empty", got)
	}
}

func TestWordAtTreatsACJKRunAsOneWord(t *testing.T) {
	blocks := []Block{line("自然语言处理很有用", 0, 24)}

	got := WordAt(blocks, 100+36, 10)
	if got != "自然语言处理很有用" {
		t.Errorf("got %q, want the whole run", got)
	}
}

func TestWordAtTrimsEdgePunctuation(t *testing.T) {
	blocks := []Block{line(`"quoted" word,`, 0, 20)}

	if got := WordAt(blocks, 118, 8); got != "quoted" {
		t.Errorf("got %q, want the word under the pointer", got)
	}
	if got := WordAt(blocks, 214, 8); got != "word" {
		t.Errorf("got %q, want the word under the pointer", got)
	}
}

func TestWordAtHandlesApostrophesAndHyphens(t *testing.T) {
	blocks := []Block{line("don't stop-and-go now", 0, 20)}

	if got := WordAt(blocks, 130, 8); got != "don't" {
		t.Errorf("got %q, want the word under the pointer", got)
	}
	if got := WordAt(blocks, 178, 8); got != "stop-and-go" {
		t.Errorf("got %q, want the word under the pointer", got)
	}
}

func TestWordAtIgnoresBlankBlocks(t *testing.T) {
	blocks := []Block{{Text: "   ", X: 0, Y: 0, Width: 100, Height: 20}, line("real text", 0, 20)}

	if got := WordAt(blocks, 130, 8); got != "real" {
		t.Errorf("got %q, want the word under the pointer", got)
	}
}

func TestWordAtWithoutGeometryStillAnswers(t *testing.T) {
	// Some providers return text without positions; the pointer then resolves
	// against an assumed fixed advance rather than failing.
	blocks := []Block{{Text: "no bounds here", Y: 0, Height: 20}}

	if got := WordAt(blocks, 10, 8); got != "no" {
		t.Errorf("got %q, want the word under the pointer", got)
	}
}
