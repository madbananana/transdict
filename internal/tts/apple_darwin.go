//go:build darwin

package tts

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"transdict/internal/settings"
)

// apple speaks through the built-in macOS `say` command. AVSpeechSynthesizer
// only routes audio to the output device and gives back no buffer, so the
// supported offline path is to render the utterance to an AIFF file with
// `say -o` and read that file back as the clip bytes.
var appleSayCommand = "say"

const (
	appleRateNormal = 190
	appleRateSlow   = 120
)

type apple struct{}

func init() {
	Register(settings.ServiceType{
		Kind: "apple", Name: "macOS 朗读",
		Note:    "系统自带，离线",
		Free:    true,
		Default: settings.Provider{Enabled: true},
	}, func(settings.Provider) (Provider, error) {
		return &apple{}, nil
	})
}

func (a *apple) Label() string { return "macOS" }

func (a *apple) Speak(ctx context.Context, text, lang string, slow bool) (*Clip, error) {
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("macOS 朗读：待朗读文本为空")
	}

	rate := appleRateNormal
	if slow {
		rate = appleRateSlow
	}
	voice := appleVoice(lang)

	out, err := os.CreateTemp(os.TempDir(), "transdict-tts-*.aiff")
	if err != nil {
		return nil, fmt.Errorf("macOS 朗读创建临时文件失败: %w", err)
	}
	path := out.Name()
	out.Close()
	defer os.Remove(path)

	// Arguments go to exec one element at a time; the user's text is the final
	// argv value and is never interpolated into a shell string.
	args := make([]string, 0, 6)
	if voice != "" {
		args = append(args, "-v", voice)
	}
	args = append(args, "-r", strconv.Itoa(rate), "-o", path, text)

	if combined, err := exec.CommandContext(ctx, appleSayCommand, args...).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("macOS 朗读合成失败: %v: %s", err, errBody(combined))
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("macOS 朗读读取音频失败: %w", err)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("macOS 朗读：未生成音频数据")
	}
	return &Clip{MIME: "audio/aiff", Data: data}, nil
}

// appleVoices maps canonical codes onto documented built-in macOS voice names.
var appleVoices = map[string]string{
	"zh":    "Ting-Ting",
	"zh-CN": "Ting-Ting",
	"zh-TW": "Mei-Jia",
	"zh-HK": "Sin-ji",
	"en":    "Alex",
	"en-US": "Alex",
	"en-GB": "Moira",
	"en-AU": "Karen",
	"ja":    "Kyoko",
	"ja-JP": "Kyoko",
	"ko":    "Yuna",
	"fr":    "Thomas",
	"de":    "Anna",
	"es":    "Carlos",
	"it":    "Alice",
	"pt":    "Luciana",
	"ru":    "Milena",
}

// appleVoice returns a built-in voice name for a language, or "" when none is
// known so `say` falls back to the system default voice.
func appleVoice(lang string) string {
	if v, ok := appleVoices[lang]; ok {
		return v
	}
	if i := strings.IndexByte(lang, '-'); i >= 0 {
		if v, ok := appleVoices[lang[:i]]; ok {
			return v
		}
	}
	return ""
}
