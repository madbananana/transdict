//go:build !darwin

package settings

import "testing"

// A row whose kind no adapter registers cannot be switched on, so the defaults
// that a Windows install gets must not contain one.
func TestDefaultsCarryNoAppleKinds(t *testing.T) {
	cfg := Default()
	for _, category := range []Category{CatOCR, CatTranslate, CatDict, CatTTS} {
		for _, p := range cfg.List(category) {
			if p.Kind == "apple" {
				t.Errorf("%s 包含 %s：本平台的 %s 注册表里没有 apple", category, p.ID, category)
			}
		}
	}
}
