package langs

import "testing"

// 检测古文的最小文本长度 is a gate, not a hint: a short line is never tested, and
// zero turns the whole test off.
func TestDetectClassicalRespectsTheMinimumLength(t *testing.T) {
	poem := "学而时习之，不亦说乎？有朋自远方来，不亦乐乎？人不知而不愠，不亦君子乎？"
	if !DetectClassical(poem, 20) {
		t.Error("古文诗句未被识别")
	}
	if DetectClassical("三人行，必有我师焉。", 20) {
		t.Error("短于最小长度仍被判定为古文")
	}
	if !DetectClassical("三人行，必有我师焉。", 5) {
		t.Error("达到最小长度时未被识别")
	}
	if DetectClassical(poem, 0) {
		t.Error("最小长度 0 应当关闭检测")
	}
}

// Modern writing must not be mistaken for classical Chinese, which would send a
// plain Chinese note through the wrong language chip.
func TestDetectClassicalLeavesModernChineseAlone(t *testing.T) {
	for _, text := range []string{
		"今天我们需要认真讨论这个问题的解决方案",
		"这个功能的设计目标是让用户可以更快地找到答案",
		"请把会议纪要发给所有参与者并确认时间",
		"Hello OCR World, this is plain English text",
	} {
		if DetectClassical(text, 20) {
			t.Errorf("现代文本 %q 被误判为古文", text)
		}
	}
}

// Punctuation and Latin letters do not count towards the length, or a short
// classical line padded with English would slip under the gate.
func TestDetectClassicalCountsOnlyHanCharacters(t *testing.T) {
	if DetectClassical("之乎者也之乎者也 abcdefghijklmnop", 20) {
		t.Error("拉丁字母不应计入古文判定长度")
	}
	if !DetectClassical("之乎者也之乎者也之乎者也之乎者也之乎者也", 20) {
		t.Error("够长的古文未被识别")
	}
}
