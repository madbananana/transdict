package gui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"transdict/internal/dict"
	"transdict/internal/settings"
	"transdict/internal/translate"
	"transdict/internal/tts"
)

// probeTimeout is generous because some services are AI models or local CLIs
// whose first answer includes a process start. A timeout still belongs here:
// without one, 验证 on a dead endpoint would leave the button spinning forever.
const probeTimeout = 30 * time.Second

// TestProvider is the 验证 button in a service's detail pane. It runs one tiny
// query through the saved configuration, so a wrong key or endpoint is
// diagnosed here rather than in a full translation. The reply is a short
// summary; a failure returns the provider's own reason as the error.
func (s *SettingsService) TestProvider(id string) (string, error) {
	cfg := s.cfg.Get()
	p, ok := providerByID(cfg, id)
	if !ok {
		return "", errors.New("找不到这个服务")
	}
	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()

	switch p.Category {
	case settings.CatTranslate:
		prov, err := translate.New(p)
		if err != nil {
			return "", err
		}
		out, _, err := prov.Translate(ctx, "hello", "en", probeTarget(cfg.TargetLang))
		if err != nil {
			return "", err
		}
		return clipProbe(out), nil

	case settings.CatDict:
		prov, err := dict.New(p)
		if err != nil {
			return "", err
		}
		entries, err := prov.Explain(ctx, "hello", "en", probeTarget(cfg.TargetLang))
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("hello：%d 条释义", len(entries)), nil

	case settings.CatTTS:
		prov, err := tts.New(p)
		if err != nil {
			return "", err
		}
		clip, err := prov.Speak(ctx, "hello", "en", false)
		if err != nil {
			return "", err
		}
		if clip == nil || clip.Source() == "" {
			return "", errors.New("朗读服务没有返回音频")
		}
		return "朗读服务可用", nil

	case settings.CatOCR:
		// Recognition needs pixels; a blank image would only prove the endpoint
		// rejects blank images. OCR failures are diagnosed from a real capture.
		return "", errors.New("OCR 服务需要截图才能验证")
	}
	return "", errors.New("未知的服务分类：" + p.Category)
}

func providerByID(cfg settings.Config, id string) (settings.Provider, bool) {
	for _, list := range [][]settings.Provider{cfg.Translate, cfg.Dict, cfg.TTS, cfg.OCR} {
		for _, p := range list {
			if p.ID == id {
				return p, true
			}
		}
	}
	return settings.Provider{}, false
}

// probeTarget keeps the probe away from English→English, which several
// providers treat as "no translation needed" and answer with the input.
func probeTarget(target string) string {
	if target == "" || target == "en" {
		return "zh-CN"
	}
	return target
}

func clipProbe(text string) string {
	text = strings.Join(strings.Fields(text), " ")
	r := []rune(text)
	switch {
	case len(r) == 0:
		return "服务返回了空结果"
	case len(r) > 60:
		return string(r[:60]) + "…"
	}
	return text
}
