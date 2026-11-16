package langs

import "unicode"

// Classical is the tag the recogniser reports for 文言文. It is a genre rather
// than a service language, so it never appears in the pickers: a translation
// target of 古文 would be a request no provider understands.
const Classical = "zh-classical"

// classicalRatio is how much of a passage has to be function words before it
// reads as classical Chinese. Modern writing leans on 的/了/是, while the
// particles below barely survive into it.
const classicalRatio = 0.1

var classicalMarkers = map[rune]bool{
	'之': true, '乎': true, '者': true, '也': true, '矣': true, '焉': true, '哉': true,
	'兮': true, '曰': true, '吾': true, '余': true, '汝': true, '爾': true, '尔': true,
	'蓋': true, '盖': true, '夫': true, '遂': true, '奚': true, '耶': true, '邪': true,
	'歟': true, '欤': true, '乃': true, '於': true, '與': true, '与': true, '則': true,
	'则': true, '莫': true, '弗': true,
}

// DetectClassical reports whether text reads as 文言文. minLen is 检测古文的最小
// 文本长度: a handful of characters cannot show a genre, so anything shorter is
// never tested, and minLen <= 0 switches the test off entirely. Only Han
// characters count, so a Latin sentence can never reach the threshold.
func DetectClassical(text string, minLen int) bool {
	if minLen <= 0 {
		return false
	}
	total, marked := 0, 0
	for _, r := range text {
		if !unicode.Is(unicode.Han, r) {
			continue
		}
		total++
		if classicalMarkers[r] {
			marked++
		}
	}
	if total < minLen {
		return false
	}
	return float64(marked)/float64(total) >= classicalRatio
}
