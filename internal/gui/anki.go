package gui

import (
	"context"
	"errors"
	"strings"

	"transdict/internal/anki"
	"transdict/internal/dict"
	"transdict/internal/settings"
)

// ankiClient builds the destination from the saved config. Reading it per call
// means an endpoint edited in the settings window applies to the next card
// without restarting the app.
func ankiClient(c settings.Config) anki.Client {
	fields := make([]anki.Field, 0, len(c.AnkiMappings))
	for _, m := range c.AnkiMappings {
		fields = append(fields, anki.Field{Name: m.Field, Template: m.Template})
	}
	return anki.Client{
		Endpoint: c.AnkiEndpoint,
		Deck:     c.AnkiDeck,
		Model:    c.AnkiModel,
		Fields:   fields,
	}
}

// AddToAnki is 添加到 Anki on a dictionary card: the card hands over every value
// a field template can name, and the mapping decides which go into the note.
func (s *Service) AddToAnki(note anki.Note) error {
	cfg := s.cfg.Get()
	if !cfg.AnkiEnabled {
		return errors.New("Anki Connect 未启用")
	}
	if strings.TrimSpace(cfg.AnkiDeck) == "" ||
		strings.TrimSpace(cfg.AnkiModel) == "" ||
		len(cfg.AnkiMappings) == 0 {
		return errors.New("Anki 设置不完整")
	}
	return ankiClient(cfg).Add(note)
}

// AnkiTest checks the saved destination without writing a note, so 测试连接 can
// answer before the first card is made. It works while the switch is off: the
// address is exactly what someone setting this up needs to verify.
func (s *Service) AnkiTest() (int, error) {
	return ankiClient(s.cfg.Get()).Test()
}

// AnkiFields is 从 Anki 读取字段: AnkiConnect knows how the note model spells
// its fields, so the settings page can offer them instead of trusting a typed
// name. It reads the saved endpoint and model for the same reason 测试连接 does.
func (s *Service) AnkiFields() ([]string, error) {
	cfg := s.cfg.Get()
	return anki.Fields(cfg.AnkiEndpoint, cfg.AnkiModel)
}

// AnkiPreview is 预览 on the Anki page: one word through a dictionary the user
// picked, so the templates are checked against a real answer. EasyDict's
// preview queries a service too; filling it from a canned sample hid a template
// that named a variable the dictionary never returns.
func (s *Service) AnkiPreview(providerID, word string) ([]dict.Entry, error) {
	word = strings.TrimSpace(word)
	if word == "" {
		return nil, errors.New("请输入要预览的单词")
	}
	cfg := s.cfg.Get()
	var picked settings.Provider
	for _, p := range cfg.Dict {
		if p.ID == providerID {
			picked = p
			break
		}
	}
	if picked.ID == "" {
		return nil, errors.New("请选择一个词典服务")
	}
	prov, err := dict.New(picked)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()
	return prov.Explain(ctx, word, "auto", probeTarget(cfg.TargetLang))
}
