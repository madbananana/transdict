package translate

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"transdict/internal/settings"
)

// geminiRequest mirrors the documented generateContent body so the test checks
// the wire shape rather than our own struct tags.
type geminiRequest struct {
	SystemInstruction struct {
		Parts []struct {
			Text string `json:"text"`
		} `json:"parts"`
	} `json:"systemInstruction"`
	Contents []struct {
		Role  string `json:"role"`
		Parts []struct {
			Text string `json:"text"`
		} `json:"parts"`
	} `json:"contents"`
	GenerationConfig map[string]any `json:"generationConfig"`
}

func TestGeminiTranslate(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		response string
		want     string
		wantErr  string
	}{
		{
			name:     "parts 数组拼接",
			response: `{"candidates":[{"content":{"role":"model","parts":[{"text":"你好，"},{"text":"世界"}]},"finishReason":"STOP"}]}`,
			want:     "你好，世界",
		},
		{
			name:     "单个 part",
			response: `{"candidates":[{"content":{"parts":[{"text":"こんにちは"}]},"finishReason":"STOP"}]}`,
			want:     "こんにちは",
		},
		{
			name:     "文本被长度截断",
			response: `{"candidates":[{"content":{"parts":[{"text":"半句"}]},"finishReason":"MAX_TOKENS"}]}`,
			want:     "半句",
			wantErr:  "截断",
		},
		{
			name:     "安全策略拦截",
			response: `{"promptFeedback":{"blockReason":"SAFETY"}}`,
			wantErr:  "拒绝了这段文本",
		},
		{
			name:     "候选被拦截",
			response: `{"candidates":[{"finishReason":"SAFETY","content":{"parts":[{"text":"半句"}]},"blockReason":"SAFETY"}]}`,
			wantErr:  "安全策略",
		},
		{
			name:     "没有候选",
			response: `{"candidates":[]}`,
			wantErr:  "空结果",
		},
		{
			name:     "空文本",
			response: `{"candidates":[{"content":{"parts":[]},"finishReason":"STOP"}]}`,
			wantErr:  "未给出译文",
		},
		{
			name:     "密钥无效",
			status:   http.StatusBadRequest,
			response: `{"error":{"code":400,"message":"API key not valid","status":"INVALID_ARGUMENT"}}`,
			wantErr:  "API key not valid",
		},
		{
			name:     "非 JSON 响应",
			status:   http.StatusBadGateway,
			response: `<html>proxy error</html>`,
			wantErr:  "HTTP 502",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got geminiRequest
			var gotPath, gotKey string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				gotKey = r.URL.Query().Get("key")
				raw, _ := io.ReadAll(r.Body)
				if err := json.Unmarshal(raw, &got); err != nil {
					t.Errorf("请求体不是合法 JSON: %v (%s)", err, raw)
				}
				status := tc.status
				if status == 0 {
					status = http.StatusOK
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				_, _ = io.WriteString(w, tc.response)
			}))
			defer srv.Close()

			p, err := New(settings.Provider{
				Kind: "gemini", BaseURL: srv.URL, APIKey: "secret-key", Model: "gemini-2.0-flash",
			})
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			out, detected, err := p.Translate(context.Background(), "hello\nworld", "auto", "zh-CN")

			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("Translate 意外失败: %v", err)
				}
				if out != tc.want {
					t.Errorf("译文 = %q, 期望 %q", out, tc.want)
				}
			} else {
				if err == nil {
					t.Fatalf("期望错误包含 %q, 实际得到 %q", tc.wantErr, out)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("错误 = %q, 期望包含 %q", err, tc.wantErr)
				}
			}
			if detected != "" {
				t.Errorf("detect = %q, Gemini 不返回检测语言", detected)
			}

			// Wire shape: model in the path, key in the query, text in parts.
			if want := "/v1beta/models/gemini-2.0-flash:generateContent"; gotPath != want {
				t.Errorf("路径 = %q, 期望 %q", gotPath, want)
			}
			if gotKey != "secret-key" {
				t.Errorf("query key = %q, 期望 secret-key", gotKey)
			}
			if len(got.Contents) != 1 || got.Contents[0].Role != "user" {
				t.Fatalf("contents = %+v, 期望一条 user 消息", got.Contents)
			}
			if len(got.Contents[0].Parts) != 1 || got.Contents[0].Parts[0].Text != "hello\nworld" {
				t.Errorf("contents parts = %+v, 期望原文逐字传入", got.Contents[0].Parts)
			}
			sys := got.SystemInstruction.Parts
			if len(sys) != 1 || !strings.Contains(sys[0].Text, LanguageName("zh-CN")) {
				t.Errorf("systemInstruction = %+v, 期望包含目标语言名 %q", sys, LanguageName("zh-CN"))
			}
			if got.GenerationConfig == nil {
				t.Error("缺少 generationConfig")
			} else if _, ok := got.GenerationConfig["temperature"]; !ok {
				t.Error("generationConfig 缺少 temperature")
			}
		})
	}
}

func TestGeminiLanguageNameInPrompt(t *testing.T) {
	var sysText string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var got geminiRequest
		_ = json.Unmarshal(raw, &got)
		if len(got.SystemInstruction.Parts) > 0 {
			sysText = got.SystemInstruction.Parts[0].Text
		}
		_, _ = io.WriteString(w, `{"candidates":[{"content":{"parts":[{"text":"ok"}]},"finishReason":"STOP"}]}`)
	}))
	defer srv.Close()

	g := newGemini(srv.URL, "k", "", "")
	for _, lang := range []string{"ja", "fr", "zh-TW"} {
		if _, _, err := g.Translate(context.Background(), "hi", "auto", lang); err != nil {
			t.Fatalf("translate %s: %v", lang, err)
		}
		if want := LanguageName(lang); !strings.Contains(sysText, want) {
			t.Errorf("语言 %s 的提示词 %q 未包含 %q", lang, sysText, want)
		}
	}
}

func TestGeminiWithPrompt(t *testing.T) {
	var systemPrompts []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var got geminiRequest
		_ = json.Unmarshal(raw, &got)
		if len(got.SystemInstruction.Parts) > 0 {
			systemPrompts = append(systemPrompts, got.SystemInstruction.Parts[0].Text)
		}
		_, _ = io.WriteString(w, `{"candidates":[{"content":{"parts":[{"text":"好"}]},"finishReason":"STOP"}]}`)
	}))
	defer srv.Close()

	saved := "语气正式"
	orig, err := New(settings.Provider{
		Kind: "gemini", BaseURL: srv.URL, APIKey: "k", Prompt: saved,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	pr, ok := orig.(Promptable)
	if !ok {
		t.Fatal("gemini 必须实现 Promptable")
	}

	if _, _, err := pr.WithPrompt("改成口语化").Translate(context.Background(), "x", "auto", "en"); err != nil {
		t.Fatalf("WithPrompt translate: %v", err)
	}
	if _, _, err := orig.Translate(context.Background(), "x", "auto", "en"); err != nil {
		t.Fatalf("original translate: %v", err)
	}
	if len(systemPrompts) != 2 {
		t.Fatalf("收到 %d 个提示词, 期望 2", len(systemPrompts))
	}
	if !strings.Contains(systemPrompts[0], "改成口语化") {
		t.Errorf("临时指令未进入系统提示词: %q", systemPrompts[0])
	}
	if strings.Contains(systemPrompts[0], saved) {
		t.Errorf("已保存的附加指令不应与临时指令叠加: %q", systemPrompts[0])
	}
	if !strings.Contains(systemPrompts[1], saved) || strings.Contains(systemPrompts[1], "改成口语化") {
		t.Errorf("WithPrompt 污染了原 provider: %q", systemPrompts[1])
	}
	if got := orig.(*gemini).Prompt; got != saved {
		t.Errorf("原 provider 的 Prompt 被改成 %q", got)
	}
}

func TestGeminiDefaults(t *testing.T) {
	g := newGemini("", "", "", "")
	if g.BaseURL != geminiDefaultBaseURL {
		t.Errorf("默认 baseUrl = %q, 期望 %q", g.BaseURL, geminiDefaultBaseURL)
	}
	if g.Model != geminiDefaultModel {
		t.Errorf("默认 model = %q, 期望 %q", g.Model, geminiDefaultModel)
	}
	if !strings.Contains(g.endpoint(), geminiDefaultBaseURL+"/v1beta/models/") {
		t.Errorf("endpoint = %q", g.endpoint())
	}

	// A trailing slash must not produce a doubled separator.
	g = newGemini("https://proxy.example/v1/", "k", "m", "")
	if g.BaseURL != "https://proxy.example/v1" {
		t.Errorf("baseUrl 未去掉尾部斜杠: %q", g.BaseURL)
	}
	if !strings.HasPrefix(g.endpoint(), "https://proxy.example/v1/v1beta/models/m:generateContent") {
		t.Errorf("自定义地址 endpoint = %q", g.endpoint())
	}
	if !strings.HasPrefix(g.Label(), "Gemini · ") {
		t.Errorf("Label = %q", g.Label())
	}
	m, ok := Meta("gemini")
	if !ok {
		t.Fatal("gemini 未注册")
	}
	if m.Default.BaseURL != geminiDefaultBaseURL || m.Default.Model != geminiDefaultModel {
		t.Errorf("注册默认值 = %+v", m.Default)
	}
}
