package query

import (
	"context"
	"strings"
	"time"

	"transdict/internal/langs"
	"transdict/internal/settings"
	"transdict/internal/translate"
)

const (
	// detectSampleRunes bounds what the online recognisers are shown: a
	// sentence is enough to name a language, and Google takes the text in a GET
	// URL, which a whole paragraph would overflow.
	detectSampleRunes = 200
	// detectTimeout keeps 语种识别 optimization from holding up the query for
	// long; the system answer is already in hand when it expires.
	detectTimeout = 3 * time.Second
)

// optimizeDetection asks the service named by 语种识别 to name the language
// first. Easydict offers Google and Baidu here because the system recogniser is
// unsure about short phrases. A mode that is off, a service that is not
// configured, and a failed request all leave the system answer in place.
func (e *Engine) optimizeDetection(ctx context.Context, cfg settings.Config, text string) (string, bool) {
	kind := cfg.DetectOptimize
	if kind != settings.DetectOptimizeGoogle && kind != settings.DetectOptimizeBaidu {
		return "", false
	}
	sample := []rune(strings.TrimSpace(text))
	if len(sample) == 0 {
		return "", false
	}
	if len(sample) > detectSampleRunes {
		sample = sample[:detectSampleRunes]
	}
	for _, p := range cfg.List(settings.CatTranslate) {
		if string(p.Kind) != kind || !translate.Ready(p) {
			continue
		}
		impl, err := translate.New(p)
		if err != nil {
			return "", false
		}
		// Both services report the detected source code on every translation,
		// so one request both names the language and warms the answer.
		callCtx, cancel := context.WithTimeout(ctx, detectTimeout)
		_, detected, err := impl.Translate(callCtx, string(sample), langs.Auto, "en")
		cancel()
		detected = strings.TrimSpace(detected)
		if err != nil || detected == "" {
			return "", false
		}
		return detected, true
	}
	return "", false
}
