// 导入词典: EasyDict treats MDict as a list of imported .mdx files rather than
// a folder. The list lives in the provider's Extra, and the file panel is the
// one native dialog a browser cannot offer.
package gui

import (
	"errors"
	"path/filepath"
	"strings"

	"transdict/internal/dict"
	"transdict/internal/l10n"
	"transdict/internal/settings"
)

// PickDictFiles asks for .mdx files. Multi-select because dictionaries are
// normally imported a shelf at a time.
func (s *Service) PickDictFiles() ([]string, error) {
	paths, err := s.wails.Dialog.OpenFile().
		SetMessage(l10n.T("导入词典")).
		AddFilter(l10n.T("MDX 词典"), "*.mdx").
		AddFilter(l10n.T("全部文件"), "*.*").
		PromptForMultipleSelection()
	if err != nil || len(paths) == 0 {
		return nil, err
	}
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		if strings.TrimSpace(path) != "" {
			out = append(out, path)
		}
	}
	return out, nil
}

// DictFiles lists the imported dictionaries of one service.
func (s *SettingsService) DictFiles(id string) ([]string, error) {
	p, ok := providerByID(s.cfg.Get(), id)
	if !ok {
		return nil, errors.New("找不到这个服务")
	}
	return dict.ParseFiles(p.Get(dict.FilesField)), nil
}

// AddDictFiles appends imported dictionaries. The first import also fills the
// folder box with its directory, so 缺配置 never marks a service that will run;
// the file list then narrows which .mdx in that folder answer.
func (s *SettingsService) AddDictFiles(id string, paths []string) error {
	return s.editProvider(id, func(p *settings.Provider) {
		merged := append(dict.ParseFiles(p.Get(dict.FilesField)), paths...)
		p.Set(dict.FilesField, dict.JoinFiles(merged))
		if strings.TrimSpace(p.Get(dict.PathField)) == "" && len(merged) > 0 {
			p.Set(dict.PathField, filepath.Dir(merged[0]))
		}
	})
}

// RemoveDictFile drops one imported dictionary. Removing the last one leaves
// the folder scan in charge, which is the older configuration.
func (s *SettingsService) RemoveDictFile(id, path string) error {
	return s.editProvider(id, func(p *settings.Provider) {
		kept := make([]string, 0)
		for _, file := range dict.ParseFiles(p.Get(dict.FilesField)) {
			if file != path {
				kept = append(kept, file)
			}
		}
		p.Set(dict.FilesField, dict.JoinFiles(kept))
	})
}

// editProvider applies fn to the saved row with this id, whichever category it
// lives in, and reports a missing id instead of silently doing nothing.
func (s *SettingsService) editProvider(id string, fn func(*settings.Provider)) error {
	found := false
	_, err := s.cfg.Update(func(c *settings.Config) {
		for _, category := range []string{settings.CatOCR, settings.CatTranslate, settings.CatDict, settings.CatTTS} {
			list := c.List(category)
			for i := range list {
				if list[i].ID != id {
					continue
				}
				fn(&list[i])
				setList(c, category, list)
				found = true
				return
			}
		}
	})
	if err != nil {
		return err
	}
	if !found {
		return errors.New("找不到这个服务")
	}
	return nil
}
