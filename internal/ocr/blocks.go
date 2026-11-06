package ocr

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// sortBlocksReadingOrder reorders blocks top-to-bottom then left-to-right, the
// order a reader's eye follows. Lines that share a horizontal band are grouped
// together so a two-column caption does not interleave with the body.
//
// The band threshold is expressed as a fraction of the block height because a
// screenshot of 12 pt text behaves differently from one of 40 pt headings.
// Blocks without any coordinates keep their input order because SliceStable is
// used and both dy and dx compare equal at 0.
func sortBlocksReadingOrder(bs []Block) {
	sort.SliceStable(bs, func(i, j int) bool {
		a, b := bs[i], bs[j]
		band := maxInt(a.Height, b.Height)
		if band <= 0 {
			band = 24
		}
		dy := a.Y - b.Y
		if absInt(dy) > band/2 {
			return dy < 0
		}
		return a.X < b.X
	})
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func absInt(a int) int {
	if a < 0 {
		return -a
	}
	return a
}

// langHintKey stashes the caller's preferred source locale on the context so a
// provider that supports a language hint (Apple Vision, Google, Azure) can
// bias recognition without us widening the Provider interface.
type langHintKey struct{}

// ContextWithLang tags ctx with a BCP-47-ish source code; "" or "auto" is a
// no-op. Callers that have no language to pass — the CLI path, unit tests —
// simply don't call this.
func ContextWithLang(ctx context.Context, code string) context.Context {
	code = strings.TrimSpace(code)
	if code == "" || code == "auto" {
		return ctx
	}
	return context.WithValue(ctx, langHintKey{}, code)
}

// LangFromContext returns the language hint, or "" when none was set.
func LangFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(langHintKey{}).(string); ok {
		return v
	}
	return ""
}

// vendorError folds the code/message pieces the Chinese OCR vendors return in
// a 2xx JSON body into one Chinese sentence, so a provider file just calls it
// after decoding. The fallback text is the truncated body, matching what a
// non-JSON failure would produce at the HTTP layer.
func vendorError(prefix, code, message, body string) error {
	code = strings.TrimSpace(code)
	message = strings.TrimSpace(message)
	switch {
	case code != "" && message != "":
		return fmt.Errorf("%s %s: %s", prefix, code, message)
	case message != "":
		return fmt.Errorf("%s: %s", prefix, message)
	case code != "":
		return fmt.Errorf("%s 返回错误码 %s", prefix, code)
	default:
		return fmt.Errorf("%s 无错误详情，响应片段：%s", prefix, snippet([]byte(body)))
	}
}
