package query

import (
	"context"
	"strings"
	"time"

	"transdict/internal/dict"
	"transdict/internal/ocr"
	"transdict/internal/settings"
	"transdict/internal/translate"
)

// RetryOCR runs one recognition service and rebuilds the rest of the response
// from the text it produced, so a retry never leaves a stale 译文 on screen.
func (e *Engine) RetryOCR(ctx context.Context, base *Response, providerID string, img []byte) *Response {
	cfg := e.cfg.Get()
	p, ok := find(cfg, providerID)
	if !ok {
		return note(base, "服务不存在")
	}
	res := cloneResponse(base)
	if res == nil {
		res = &Response{ID: newID(), Context: settings.ContextScreenshot, Mode: "screenshot"}
	}
	outcome := e.recognize(ctx, p, img, cfg.SourceLang)
	res.OCR = mergeOutcome(res.OCR, outcome)
	res.SourceText = PickText(res.OCR)
	res.ImageURL = DataURL(img)
	if res.SourceText == "" {
		return note(res, "未识别到文字")
	}

	req := Request{Context: res.Context, Mode: res.Mode, Text: res.SourceText,
		SourceLang: cfg.SourceLang, TargetLang: cfg.TargetLang}
	req.TargetLang = firstNonEmpty(res.TargetLang, req.TargetLang)
	res.Translate = e.translateStage(ctx, cfg, req, res, time.Now(), nil)
	res.Dict = e.dictStage(ctx, cfg, req, res)
	return res
}

// Retry re-runs one dictionary or translation service on the text already shown
// and replaces only that row of the response.
func (e *Engine) Retry(ctx context.Context, base *Response, providerID string) *Response {
	cfg := e.cfg.Get()
	res := cloneResponse(base)
	if res == nil || strings.TrimSpace(res.SourceText) == "" {
		return note(res, "没有可翻译的文本")
	}
	p, ok := find(cfg, providerID)
	if !ok {
		return note(res, "服务不存在")
	}
	req := Request{Context: res.Context, Mode: res.Mode, Text: res.SourceText,
		SourceLang: cfg.SourceLang, TargetLang: firstNonEmpty(res.TargetLang, cfg.TargetLang)}

	switch p.Category {
	case settings.CatDict:
		res.Dict = mergeDict(res.Dict, e.explain(ctx, p, res.SourceText, req.SourceLang, req.TargetLang))
	case settings.CatOCR:
		return note(res, "识别服务请使用重试识别")
	default:
		res.Translate = mergeTranslate(res.Translate, e.render(ctx, p, res.SourceText, req))
	}
	return res
}

// find locates a configured provider by id across every category.
func find(cfg settings.Config, id string) (settings.Provider, bool) {
	for _, category := range []string{settings.CatOCR, settings.CatTranslate, settings.CatDict, settings.CatTTS} {
		for _, p := range cfg.List(category) {
			if p.ID == id {
				return p, true
			}
		}
	}
	return settings.Provider{}, false
}

// Find is the exported form for the settings UI.
func Find(cfg settings.Config, id string) (settings.Provider, bool) { return find(cfg, id) }

func cloneResponse(base *Response) *Response {
	if base == nil {
		return nil
	}
	out := *base
	out.OCR = append([]ocr.Outcome(nil), base.OCR...)
	out.Dict = append([]dict.Result(nil), base.Dict...)
	out.Translate = append([]translate.Result(nil), base.Translate...)
	return &out
}

func mergeOutcome(list []ocr.Outcome, want ocr.Outcome) []ocr.Outcome {
	for i := range list {
		if list[i].Provider == want.Provider {
			list[i] = want
			return list
		}
	}
	return append(list, want)
}

func mergeDict(list []dict.Result, want dict.Result) []dict.Result {
	for i := range list {
		if list[i].Provider == want.Provider {
			list[i] = want
			return list
		}
	}
	return append(list, want)
}

func mergeTranslate(list []translate.Result, want translate.Result) []translate.Result {
	for i := range list {
		if list[i].Provider == want.Provider {
			if want.Text == "" && want.Error == "" {
				want.Error = "返回空译文"
			}
			list[i] = want
			return list
		}
	}
	return append(list, want)
}

func note(res *Response, msg string) *Response {
	if res == nil {
		res = &Response{}
	}
	res.Error = msg
	return res
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
