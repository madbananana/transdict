package ocr

import "testing"

func TestNormalizeTextJoinsWrappedLatinLines(t *testing.T) {
	got := NormalizeText("The quick brown fox\r\njumps over the lazy dog")
	want := "The quick brown fox jumps over the lazy dog"
	if got != want {
		t.Errorf("NormalizeText = %q, want %q", got, want)
	}
}

func TestNormalizeTextReattachesHyphenatedWords(t *testing.T) {
	got := NormalizeText("This is an exam-\nple of a hyphenated word")
	want := "This is an example of a hyphenated word"
	if got != want {
		t.Errorf("NormalizeText = %q, want %q", got, want)
	}
}

func TestNormalizeTextKeepsSentenceBreaks(t *testing.T) {
	got := NormalizeText("First sentence.\nSecond sentence.")
	want := "First sentence.\nSecond sentence."
	if got != want {
		t.Errorf("NormalizeText = %q, want %q", got, want)
	}
}

func TestNormalizeTextKeepsBlocksOnTheirOwnLine(t *testing.T) {
	got := NormalizeText("Intro line\n• first item\n• second item")
	want := "Intro line\n• first item\n• second item"
	if got != want {
		t.Errorf("NormalizeText = %q, want %q", got, want)
	}
	if got := NormalizeText("Steps:\n1. open\n2. close"); got != "Steps:\n1. open\n2. close" {
		t.Errorf("numbered list = %q", got)
	}
}

func TestNormalizeTextJoinsChineseWithoutSpaces(t *testing.T) {
	got := NormalizeText("中文 字之间 有 空格，\n而且 换行 出现了。")
	want := "中文字之间有空格，而且换行出现了。"
	if got != want {
		t.Errorf("NormalizeText = %q, want %q", got, want)
	}
}

func TestNormalizeTextKeepsSpaceBetweenLatinAndCJK(t *testing.T) {
	got := NormalizeText("使用 Swift 编写")
	if want := "使用 Swift 编写"; got != want {
		t.Errorf("NormalizeText = %q, want %q", got, want)
	}
}

func TestNormalizeTextCollapsesRunsOfSpaces(t *testing.T) {
	got := NormalizeText("  too   many\tspaces  ")
	want := "too many spaces"
	if got != want {
		t.Errorf("NormalizeText = %q, want %q", got, want)
	}
}

func TestNormalizeTextKeepsParagraphBreaks(t *testing.T) {
	got := NormalizeText("One paragraph that\nwraps.\n\nAnother one.")
	want := "One paragraph that wraps.\nAnother one."
	if got != want {
		t.Errorf("NormalizeText = %q, want %q", got, want)
	}
}

func TestNormalizeTextLeavesEmptyTextAlone(t *testing.T) {
	if got := NormalizeText("  \n\n "); got != "" {
		t.Errorf("NormalizeText = %q, want empty", got)
	}
}
