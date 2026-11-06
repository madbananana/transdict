package translate

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"transdict/internal/netp"
	"transdict/internal/settings"
)

// The CLI providers reuse a subscription the user already pays for by driving
// a local agent binary instead of an HTTP key. Everything travels as an argv
// slice plus a stdin reader: no shell is ever spawned, so a quoted sentence or
// a backtick in the source text cannot become a command.
type cliProvider struct {
	Name   string
	Binary string
	Model  string
	Prompt string

	// args builds the argv tail; input builds the stdin payload, and a nil
	// input means the CLI takes the whole prompt as an argument instead.
	args  func(model, instruction, text string) []string
	input func(model, instruction, text string) string
	// run is swappable so a test can assert argv without a real binary.
	run func(ctx context.Context, path string, args []string, stdin string) (string, error)
}

// cliSpec is one supported binary's calling convention.
type cliSpec struct {
	kind        string
	name        string
	note        string
	binary      string
	defaultArgs func(model, instruction, text string) []string
	defaultPipe func(model, instruction, text string) string
}

// cliTimeout is generous because a cold agent start plus one round trip on a
// throttled plan can legitimately take most of a minute.
var cliTimeout = 120 * time.Second

var cliSpecs = []cliSpec{
	{
		kind: "claude-cli", name: "Claude Code CLI", binary: "claude",
		note: "调用本机 claude（-p 打印模式），复用已有登录或 ANTHROPIC_API_KEY",
		// The instruction goes to --system-prompt so the pasted text stays
		// byte-for-byte what the user selected.
		defaultArgs: func(model, instruction, text string) []string {
			args := []string{"-p", "--output-format", "text", "--system-prompt", instruction}
			return append(args, cliModelFlag("-m", model)...)
		},
		defaultPipe: func(model, instruction, text string) string { return text },
	},
	{
		kind: "codex-cli", name: "Codex CLI", binary: "codex",
		note: "调用本机 codex exec，复用 ChatGPT 登录；- 从标准输入读取",
		defaultArgs: func(model, instruction, text string) []string {
			args := []string{"exec", "--skip-git-repo-check", "-"}
			return append(args, cliModelFlag("-m", model)...)
		},
		defaultPipe: cliJoinPrompt,
	},
	{
		kind: "copilot-cli", name: "Copilot CLI", binary: "copilot",
		note: "调用本机 copilot -p（GitHub Copilot 命令行），也可指向 gh 的 copilot 入口",
		// -p only takes the prompt as an argument, so no stdin payload.
		defaultArgs: func(model, instruction, text string) []string {
			args := []string{"-p", cliJoinPrompt(model, instruction, text), "--no-color"}
			return append(args, cliModelFlag("--model", model)...)
		},
	},
}

func init() {
	for _, spec := range cliSpecs {
		Register(settings.ServiceType{
			Kind: spec.kind, Name: spec.name, Note: spec.note, Multi: true, Markdown: true, Prompt: true,
			Fields: []settings.Field{
				{Key: "binary", Label: "可执行文件", Placeholder: spec.binary},
				{Key: settings.FieldModel, Label: "模型", Placeholder: "留空使用 CLI 默认"},
				{Key: settings.FieldPrompt, Label: "附加指令", Text: true},
			},
			Default: settings.Provider{Extra: map[string]string{"binary": spec.binary}},
		}, func(p settings.Provider) (Provider, error) {
			binary := strings.TrimSpace(p.Extra["binary"])
			if binary == "" {
				binary = spec.binary
			}
			c := &cliProvider{
				Name:   spec.name,
				Binary: binary,
				Model:  p.Model,
				Prompt: p.Prompt,
				args:   spec.defaultArgs,
				input:  spec.defaultPipe,
			}
			c.run = c.execRun
			return c, nil
		})
	}
}

func (c *cliProvider) Label() string {
	if c.Model != "" {
		return c.Name + " · " + c.Model
	}
	return c.Name
}

// WithPrompt swaps the ad-hoc 润色/总结 instruction in for the saved one; the
// original provider keeps its settings.
func (c *cliProvider) WithPrompt(prompt string) Provider {
	clone := *c
	clone.Prompt = prompt
	return &clone
}

func (c *cliProvider) instruction(targetLang string) string {
	sb := strings.Builder{}
	sb.WriteString("把用户提供的文本翻译成")
	sb.WriteString(LanguageName(targetLang))
	sb.WriteString("。严格按原文换行，一行对一行；数字、代码、专有名词与标点保持原样。" +
		"只回答译文本身，不要解释、不要加标题、不要用代码块包裹。")
	if strings.TrimSpace(c.Prompt) != "" {
		sb.WriteString("\n附加要求：" + c.Prompt)
	}
	return sb.String()
}

func (c *cliProvider) Translate(ctx context.Context, text, sourceLang, targetLang string) (string, string, error) {
	instruction := c.instruction(targetLang)

	path, err := lookupCLI(c.Binary)
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) || errors.Is(err, fs.ErrNotExist) {
			return "", "", cliMissing(c.Name, c.Binary)
		}
		return "", "", fmt.Errorf("%s 无法执行 %s：%v", c.Name, c.Binary, err)
	}

	args := c.args(c.Model, instruction, text)
	pipe := ""
	if c.input != nil {
		pipe = c.input(c.Model, instruction, text)
	}

	out, err := c.run(ctx, path, args, pipe)
	if err != nil {
		return "", "", fmt.Errorf("%s 调用失败：%v", c.Name, err)
	}
	out = strings.TrimPrefix(strings.TrimSpace(out), "\ufeff")
	out = strings.TrimSpace(out)
	if out == "" {
		return "", "", fmt.Errorf("%s 没有输出译文", c.Name)
	}
	return out, "", nil
}

// execRun is the real command runner. The context deadline both bounds the
// wait and kills the child, so a CLI stuck on login cannot pin a query.
func (c *cliProvider) execRun(ctx context.Context, path string, args []string, stdin string) (string, error) {
	window := netp.Timeout(cliTimeout)
	ctx, cancel := context.WithTimeout(ctx, window)
	defer cancel()

	cmd := exec.CommandContext(ctx, path, args...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	err := cmd.Run()
	out := buf.String()
	if err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("超过 %s 未返回：%s", window, truncate(out))
		}
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return "", fmt.Errorf("退出码 %d：%s", exit.ExitCode(), truncate(out))
		}
		return "", err
	}
	return out, nil
}

// cliMissing is deliberately actionable: the two fixes are installing the
// binary or pointing 可执行文件 at its full path.
func cliMissing(name, binary string) error {
	return fmt.Errorf("未找到 %s 所需的命令 %q。请先安装该命令行工具，或在设置的「可执行文件」里填写完整路径（例如 /usr/local/bin/%s）",
		name, binary, binary)
}

// lookupCLI resolves a command name without going through a shell. npm-style
// agent CLIs arrive as .cmd wrappers on Windows, where LookPath may not match
// every PATHEXT entry, so a failed lookup is retried with common extensions.
func lookupCLI(name string) (string, error) {
	// An explicit path in either slash form is used as given, since LookPath
	// searches PATH only and refuses names holding a separator.
	if strings.ContainsAny(name, "/\\") {
		return checkedCLIPath(name)
	}
	path, err := exec.LookPath(name)
	if err == nil {
		return path, nil
	}
	if runtime.GOOS == "windows" {
		for _, ext := range []string{".cmd", ".exe", ".bat", ".ps1"} {
			if strings.HasSuffix(strings.ToLower(name), ext) {
				continue
			}
			if p, retryErr := exec.LookPath(name + ext); retryErr == nil {
				return p, nil
			}
		}
	}
	return "", err
}

// checkedCLIPath validates a user-typed path so a typo surfaces as the same
// actionable "install or configure" hint as a PATH miss.
func checkedCLIPath(name string) (string, error) {
	if _, err := os.Stat(name); err == nil {
		return name, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	if runtime.GOOS == "windows" {
		for _, ext := range []string{".cmd", ".exe", ".bat", ".ps1"} {
			if _, statErr := os.Stat(name + ext); statErr == nil {
				return name + ext, nil
			}
		}
	}
	return "", fs.ErrNotExist
}

// cliModelFlag appends a model flag only when the user filled 模型 in.
func cliModelFlag(flag, model string) []string {
	if strings.TrimSpace(model) == "" {
		return nil
	}
	return []string{flag, model}
}

// cliJoinPrompt folds the instruction and the source text into one prompt for
// CLIs that have no separate system channel.
func cliJoinPrompt(_, instruction, text string) string {
	return instruction + "\n\n文本：\n" + text
}
