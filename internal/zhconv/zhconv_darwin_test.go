//go:build darwin

package zhconv

import "testing"

func TestTraditionalRewritesSimplified(t *testing.T) {
	// 头发/干了 are the classic one-to-many traps; ICU resolves them by phrase,
	// which a character table cannot.
	cases := []struct{ in, want string }{
		{"敏捷的棕色狐狸跳过了懒惰的狗。", "敏捷的棕色狐狸跳過了懶惰的狗。"},
		{"头发干了以后再说。", "頭髮乾了以後再說。"},
		{"The quick brown fox", "The quick brown fox"},
		{"已經是大寫的字", "已經是大寫的字"},
		{"", ""},
	}
	for _, c := range cases {
		if got := Traditional(c.in); got != c.want {
			t.Errorf("Traditional(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
