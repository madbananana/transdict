package translate

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"transdict/internal/settings"
)

// fakeCLI writes a POSIX shell script that records how it was invoked and
// answers with a fixed translation, so the provider is exercised end to end
// without a real agent binary on PATH.
func fakeCLI(t *testing.T, script string) (path, argsFile, stdinFile string) {
	t.Helper()
	dir := t.TempDir()
	argsFile = filepath.Join(dir, "argv.txt")
	stdinFile = filepath.Join(dir, "stdin.txt")
	path = filepath.Join(dir, "fake-cli")

	body := "#!/bin/sh\n" +
		"printf '%s\\n' \"$*\" >'" + argsFile + "'\n" +
		"cat >'" + stdinFile + "' 2>/dev/null || true\n" +
		script
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("写脚本: %v", err)
	}
	return path, argsFile, stdinFile
}

func readOrFail(t *testing.T, p string) string {
	t.Helper()
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("读 %s: %v", p, err)
	}
	return string(raw)
}

func TestCLITranslateWithFakeBinary(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake CLI 是 /bin/sh 脚本，Windows 上不运行")
	}
	for _, kind := range []string{"claude-cli", "codex-cli", "copilot-cli"} {
		t.Run(kind, func(t *testing.T) {
			bin, argsFile, stdinFile := fakeCLI(t, `printf '你好，世界\n'`)
			p, err := New(settings.Provider{
				Kind:  kind,
				Extra: map[string]string{"binary": bin},
			})
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			out, detected, err := p.Translate(context.Background(), "hello world", "auto", "zh-CN")
			if err != nil {
				t.Fatalf("Translate: %v (argv=%q stdin=%q)", err, readOrFail(t, argsFile), readOrFail(t, stdinFile))
			}
			if out != "你好，世界" {
				t.Errorf("out = %q", out)
			}
			if detected != "" {
				t.Errorf("detected = %q, CLI 不做语言检测", detected)
			}

			// The target language must reach the model somewhere: either as a
			// dedicated system flag or folded into the prompt.
			combined := readOrFail(t, argsFile) + readOrFail(t, stdinFile)
			if !strings.Contains(combined, LanguageName("zh-CN")) {
				t.Errorf("提示词缺少目标语言名 %q:\nargv=%q\nstdin=%q", LanguageName("zh-CN"), combined, combined)
			}
			if !strings.Contains(combined, "hello world") {
				t.Errorf("原文未传给 CLI: %q", combined)
			}
			if !strings.HasPrefix(p.Label(), "Claude Code CLI") && !strings.HasPrefix(p.Label(), "Codex CLI") &&
				!strings.HasPrefix(p.Label(), "Copilot CLI") {
				t.Errorf("Label = %q", p.Label())
			}
		})
	}
}

// TestCLINoShellInterpretation guards the security property: text that would
// be dangerous through sh -c must arrive at the binary unchanged.
func TestCLINoShellInterpretation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("需要 /bin/sh")
	}
	tricky := "价格 $(echo PWNED) `id` && echo x; 你好"
	bin, argsFile, stdinFile := fakeCLI(t, `printf 'OK\n'`)
	p, err := New(settings.Provider{Kind: "claude-cli", Extra: map[string]string{"binary": bin}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, _, err := p.Translate(context.Background(), tricky, "auto", "en"); err != nil {
		t.Fatalf("Translate: %v", err)
	}
	got := readOrFail(t, argsFile) + readOrFail(t, stdinFile)
	if !strings.Contains(got, tricky) {
		t.Errorf("原文被改写:\n%q", got)
	}
	// Substitution would have replaced $( ) and backticks with their output.
	if strings.Contains(got, "uid=") || strings.Contains(got, "gid=") {
		t.Errorf("疑似执行了注入的命令:\n%q", got)
	}
}

func TestCLIMissingBinary(t *testing.T) {
	tests := []struct {
		name   string
		binary string
	}{
		{"PATH 中不存在", "transdict-no-such-cli-binary"},
		{"显式路径不存在", filepath.Join(t.TempDir(), "missing", "cli")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p, err := New(settings.Provider{Kind: "codex-cli", Extra: map[string]string{"binary": tc.binary}})
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			_, _, err = p.Translate(context.Background(), "hi", "auto", "en")
			if err == nil {
				t.Fatal("缺失二进制必须报错")
			}
			for _, want := range []string{"未找到", "安装", tc.binary} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("错误 %q 未包含 %q", err, want)
				}
			}
		})
	}
}

func TestCLINonZeroExit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("需要 /bin/sh")
	}
	bin, _, _ := fakeCLI(t, "echo \"not logged in\" >&2\nexit 3\n")
	p, _ := New(settings.Provider{Kind: "claude-cli", Extra: map[string]string{"binary": bin}})
	_, _, err := p.Translate(context.Background(), "hi", "auto", "en")
	if err == nil || !strings.Contains(err.Error(), "退出码 3") || !strings.Contains(err.Error(), "not logged in") {
		t.Fatalf("错误 = %v, 期望带上退出码与 stderr", err)
	}
}

func TestCLIEmptyOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("需要 /bin/sh")
	}
	bin, _, _ := fakeCLI(t, `printf '  \n'`)
	p, _ := New(settings.Provider{Kind: "copilot-cli", Extra: map[string]string{"binary": bin}})
	if _, _, err := p.Translate(context.Background(), "hi", "auto", "en"); err == nil ||
		!strings.Contains(err.Error(), "没有输出译文") {
		t.Fatalf("错误 = %v", err)
	}
}

func TestCLITimeout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("需要 /bin/sh")
	}
	restore := cliTimeout
	cliTimeout = 80 * time.Millisecond
	defer func() { cliTimeout = restore }()

	bin, _, _ := fakeCLI(t, "sleep 5\nprintf 'late\\n'\n")
	p, err := New(settings.Provider{Kind: "codex-cli", Extra: map[string]string{"binary": bin}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	start := time.Now()
	if _, _, err := p.Translate(context.Background(), "hi", "auto", "en"); err == nil ||
		!strings.Contains(err.Error(), "未返回") {
		t.Fatalf("错误 = %v, 期望超时提示", err)
	}
	if time.Since(start) > 5*time.Second {
		t.Fatal("超时后未杀掉子进程")
	}
}

// TestCLIArgvAndPrompt covers each spec's argv shape plus the Promptable copy
// semantics, using an injected runner so no process is needed.
func TestCLIArgvAndPrompt(t *testing.T) {
	const saved = "保留品牌名"
	const adHoc = "润色成广告语"

	for _, kind := range []string{"claude-cli", "codex-cli", "copilot-cli"} {
		t.Run(kind, func(t *testing.T) {
			orig, err := New(settings.Provider{
				Kind: kind, Model: "model-x", Prompt: saved,
				Extra: map[string]string{"binary": "/bin/echo"},
			})
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			c := orig.(*cliProvider)

			var seenArgs []string
			var seenStdin string
			c.run = func(_ context.Context, path string, args []string, stdin string) (string, error) {
				seenArgs = append([]string{path}, args...)
				seenStdin = stdin
				return " 译文 ", nil
			}

			if out, _, err := c.Translate(context.Background(), "src", "auto", "ja"); err != nil {
				t.Fatalf("Translate: %v", err)
			} else if out != "译文" {
				t.Errorf("out = %q, 期望去空白", out)
			}
			argv := strings.Join(seenArgs, "\n")
			// Where the instruction lands is per-CLI: claude and copilot get it
			// in argv, codex folds it into the stdin prompt.
			combined := argv + "\n" + seenStdin
			if !strings.Contains(combined, LanguageName("ja")) {
				t.Errorf("提示词缺少目标语言名: argv=%v stdin=%q", seenArgs, seenStdin)
			}
			if !strings.Contains(combined, "src") {
				t.Errorf("原文既不在 argv 也不在 stdin: argv=%v stdin=%q", seenArgs, seenStdin)
			}
			// The model flag is only appended when the user set 模型.
			if !strings.Contains(argv, "model-x") {
				t.Errorf("argv 未带模型: %v", seenArgs)
			}
			if !strings.Contains(combined, saved) {
				t.Errorf("已保存指令未生效: argv=%v stdin=%q", seenArgs, seenStdin)
			}

			// WithPrompt must target the ad-hoc instruction and leave c alone.
			pr := orig.(Promptable)
			copied := pr.WithPrompt(adHoc).(*cliProvider)
			if copied.Prompt != adHoc {
				t.Errorf("副本 Prompt = %q", copied.Prompt)
			}
			if c.Prompt != saved {
				t.Errorf("原 provider Prompt 被改成 %q", c.Prompt)
			}
			if copied.run == nil || copied.args == nil {
				t.Error("WithPrompt 副本丢失了依赖字段")
			}
			copied.run = func(_ context.Context, path string, args []string, stdin string) (string, error) {
				seenArgs = append([]string{path}, args...)
				seenStdin = stdin
				return "ok", nil
			}
			if _, _, err := copied.Translate(context.Background(), "src", "auto", "en"); err != nil {
				t.Fatalf("副本 Translate: %v", err)
			}
			combined = strings.Join(seenArgs, "\n") + seenStdin
			if !strings.Contains(combined, adHoc) {
				t.Errorf("临时指令未传给 CLI: %q", combined)
			}
			if strings.Contains(combined, saved) {
				t.Errorf("临时调用混入了已保存指令: %q", combined)
			}
			if !strings.Contains(combined, LanguageName("en")) {
				t.Errorf("副本提示词缺少目标语言: %q", combined)
			}
		})
	}
}

func TestCLIRegistryDefaults(t *testing.T) {
	want := map[string]string{
		"claude-cli":  "claude",
		"codex-cli":   "codex",
		"copilot-cli": "copilot",
	}
	for kind, binary := range want {
		m, ok := Meta(kind)
		if !ok {
			t.Fatalf("%s 未注册", kind)
		}
		if got := m.Default.Extra["binary"]; got != binary {
			t.Errorf("%s 默认二进制 = %q, 期望 %q", kind, got, binary)
		}
		var sawBinaryField bool
		for _, f := range m.Fields {
			if f.Key == "binary" && !f.Required {
				sawBinaryField = true
			}
		}
		if !sawBinaryField {
			t.Errorf("%s 应暴露可选的 binary 字段: %+v", kind, m.Fields)
		}
		// No override means the vendor default binary is used.
		c, err := New(settings.Provider{Kind: kind})
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		if got := c.(*cliProvider).Binary; got != binary {
			t.Errorf("%s Binary = %q, 期望 %q", kind, got, binary)
		}
	}
}
