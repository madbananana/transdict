//go:build !darwin

package translate

import (
	"context"
	"errors"

	"transdict/internal/settings"
)

// 苹果翻译 is registered everywhere so a synced config keeps its row; only
// macOS can actually run the system Translation framework.
func init() {
	Register(settings.ServiceType{
		Kind: "apple", Name: "苹果翻译",
		Note: "系统离线翻译，仅 macOS 可用",
		Free: true,
	}, func(p settings.Provider) (Provider, error) {
		return apple{}, nil
	})
}

type apple struct{}

func (apple) Label() string { return "苹果" }

func (apple) Translate(ctx context.Context, text, sourceLang, targetLang string) (string, string, error) {
	return "", "", errors.New("苹果翻译仅在 macOS 上可用")
}
