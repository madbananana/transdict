//go:build darwin

package translate

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"transdict/internal/settings"
)

// 苹果翻译 runs macOS's on-device Translation framework. The framework is
// Swift-only, so the work is done by the apple-translate helper that the
// packaging task compiles into Contents/MacOS beside this binary.
type apple struct {
	helper string
}

type appleReply struct {
	Text  string `json:"text"`
	Error string `json:"error"`
}

func init() {
	Register(settings.ServiceType{
		Kind: "apple", Name: "苹果翻译",
		Note: "系统离线翻译，需在系统设置中下载语言包",
		Free: true,
	}, func(p settings.Provider) (Provider, error) {
		return &apple{}, nil
	})
}

func (a *apple) Label() string { return "苹果" }

func (a *apple) Translate(ctx context.Context, text, sourceLang, targetLang string) (string, string, error) {
	path, err := appleHelper()
	if err != nil {
		return "", "", fmt.Errorf("苹果翻译不可用：%v", err)
	}
	req, err := json.Marshal(map[string]string{
		"text": text, "source": sourceLang, "target": targetLang,
	})
	if err != nil {
		return "", "", err
	}

	cmd := exec.CommandContext(ctx, path)
	cmd.Stdin = bytes.NewReader(req)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil && len(out) == 0 {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return "", "", errors.New("苹果翻译失败：" + msg)
		}
		return "", "", fmt.Errorf("苹果翻译失败：%v", err)
	}

	var reply appleReply
	if err := json.Unmarshal(out, &reply); err != nil {
		return "", "", fmt.Errorf("苹果翻译返回了无法解析的结果")
	}
	if reply.Error != "" {
		return "", "", errors.New(reply.Error)
	}
	reply.Text = strings.TrimSpace(reply.Text)
	if reply.Text == "" {
		return "", "", errors.New("苹果翻译没有输出译文")
	}
	return reply.Text, "", nil
}

// appleHelper finds the compiled helper: an explicit override first (tests and
// development), then the bundle next to this executable, then PATH.
func appleHelper() (string, error) {
	if p := strings.TrimSpace(os.Getenv("TRANSDUCT_APPLE_TRANSLATE")); p != "" {
		return p, nil
	}
	if exe, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(exe), "apple-translate")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return exec.LookPath("apple-translate")
}
