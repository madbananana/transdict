package gui

import (
	"strings"

	"transdict/internal/langs"
	"transdict/internal/settings"
)

// DetectText names the language of the text a query view is holding, before a
// query is made. EasyDict's components run the same recognition one second
// after the text settles so 识别为 is available while the user is still typing.
func (s *Service) DetectText(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return langs.Auto
	}
	return detectLanguage(s.cfg.Get(), text)
}

// detectLanguage names the language of text the way this app speaks it. The
// system recogniser only knows modern Chinese, so 检测古文的最小文本长度 is what
// lets a passage of 文言文 come back as 古文 instead of 简体中文.
//
// 第一语言 and 第二语言 bias the recogniser: a short query is genuinely
// ambiguous, and the language the user reads is a better guess than the
// recogniser's alone.
func detectLanguage(cfg settings.Config, text string) string {
	if langs.DetectClassical(text, cfg.ClassicalMinLength) {
		return langs.Classical
	}
	return langs.DetectWithHints(text, cfg.FirstLang, cfg.SecondLang)
}
