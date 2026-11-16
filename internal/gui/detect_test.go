package gui

import (
	"testing"

	"transdict/internal/langs"
	"transdict/internal/settings"
)

const classicalPoem = "学而时习之，不亦说乎？有朋自远方来，不亦乐乎？人不知而不愠，不亦君子乎？"

// 检测古文的最小文本长度 is what turns a passage of 文言文 into 古文 instead of
// 简体中文; zero must switch the test off and let the system recogniser answer.
func TestDetectLanguageReportsClassicalChinese(t *testing.T) {
	cfg := settings.Config{ClassicalMinLength: 20}
	if got := detectLanguage(cfg, classicalPoem); got != langs.Classical {
		t.Errorf("古文被识别为 %q, want %q", got, langs.Classical)
	}
	cfg.ClassicalMinLength = 0
	if got := detectLanguage(cfg, classicalPoem); got == langs.Classical {
		t.Error("最小长度为 0 时仍报告古文")
	}
}

// DetectText is the pass behind 识别为 before a query exists: an empty box has
// no language, and a typed word is named by the same recogniser a query uses.
func TestDetectTextNamesTypedText(t *testing.T) {
	s := &Service{cfg: testStore(t)}
	if got := s.DetectText("  "); got != langs.Auto {
		t.Errorf("空文本识别为 %q, want %q", got, langs.Auto)
	}
	if got := s.DetectText(" hello world "); got != "en" {
		t.Errorf("英文识别为 %q, want en", got)
	}
}

// A 古文 passage is not the removed language, so 划词查询图标 must still appear
// when the user excluded 简体中文.
func TestQueryIconKeepsClassicalChineseWhenChineseIsExcluded(t *testing.T) {
	cfg := settings.Config{
		AutoShowQueryIcon:    true,
		QueryIconMinLength:   10,
		QueryIconExcludeLang: "zh-CN",
		ClassicalMinLength:   20,
	}
	if !queryIconWanted(cfg, classicalPoem) {
		t.Error("排除简体中文后，古文选区仍应显示查询图标")
	}
	cfg.ClassicalMinLength = 0
	if queryIconWanted(cfg, classicalPoem) {
		t.Error("关闭古文检测后，古文选区应按简体中文被排除")
	}
}
