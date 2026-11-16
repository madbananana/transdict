//go:build darwin

package translate

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"transdict/internal/settings"
)

func fakeAppleHelper(t *testing.T, body string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "apple-translate")
	script := "#!/bin/sh\ncat >/dev/null\ncat <<'EOF'\n" + body + "\nEOF\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TRANSDUCT_APPLE_TRANSLATE", path)
}

func TestAppleTranslateReadsHelperJSON(t *testing.T) {
	fakeAppleHelper(t, `{"text":"你好"}`)
	p, err := New(settings.Provider{Kind: "apple"})
	if err != nil {
		t.Fatal(err)
	}
	out, _, err := p.Translate(context.Background(), "hello", "en", "zh-CN")
	if err != nil || out != "你好" {
		t.Errorf("Translate = %q, %v", out, err)
	}
}

func TestAppleTranslateSurfacesHelperError(t *testing.T) {
	fakeAppleHelper(t, `{"error":"苹果翻译尚未下载 en → zh-CN 的语言包"}`)
	p, _ := New(settings.Provider{Kind: "apple"})
	if _, _, err := p.Translate(context.Background(), "hello", "en", "zh-CN"); err == nil ||
		!strings.Contains(err.Error(), "尚未下载") {
		t.Errorf("err = %v", err)
	}
}

func TestAppleTranslateRejectsEmptyAnswer(t *testing.T) {
	fakeAppleHelper(t, `{"text":"  "}`)
	p, _ := New(settings.Provider{Kind: "apple"})
	if _, _, err := p.Translate(context.Background(), "hello", "en", "zh-CN"); err == nil {
		t.Error("空译文应当报错")
	}
}
