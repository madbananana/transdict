package tts

import (
	"crypto/md5"
	"encoding/hex"
	"strings"
)

// userAgentChrome is the browser-ish UA the free web TTS endpoints expect; a
// bare Go UA gets the request rejected by several of them.
const userAgentChrome = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) " +
	"AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36"

// errBody renders a response body for the tail of a Chinese error string,
// truncated so a surprise HTML block page does not swamp the panel.
func errBody(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 200 {
		return s[:200] + "…"
	}
	return s
}

// clampRunes caps s to at most n runes (never splitting a multi-byte char).
func clampRunes(s string, n int) string {
	if n <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

func md5Hex(s string) string {
	sum := md5.Sum([]byte(s))
	return hex.EncodeToString(sum[:])
}
