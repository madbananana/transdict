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

type ollamaRequest struct {
	Model   string `json:"model"`
	Stream  *bool  `json:"stream"`
	Options struct {
		Temperature float64 `json:"temperature"`
	} `json:"options"`
	Messages []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"messages"`
}

func TestOllamaTranslate(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		response string
		want     string
		wantErr  string
	}{
		{
			name:     "普通字符串 content",
			response: `{"model":"qwen2.5:7b","message":{"role":"assistant","content":"你好，世界"},"done":true,"done_reason":"stop"}`,
			want:     "你好，世界",
		},
		{
			name:     "content 分片数组",
			response: `{"message":{"content":[{"text":"第一行\n"},{"text":"第二行"}]},"done":true,"done_reason":"stop"}`,
			want:     "第一行\n第二行",
		},
		{
			name:     "达到长度上限",
			response: `{"message":{"content":"半句"},"done":true,"done_reason":"length"}`,
			want:     "半句",
			wantErr:  "截断",
		},
		{
			name:     "空译文",
			response: `{"message":{"content":"   "},"done":true,"done_reason":"stop"}`,
			wantErr:  "空译文",
		},
		{
			name:     "模型不存在",
			status:   http.StatusNotFound,
			response: `{"error":"model \"llama3\" not found, try pulling it first"}`,
			wantErr:  "ollama pull",
		},
		{
			name:     "服务未启动",
			status:   http.StatusServiceUnavailable,
			response: `<html>connection refused</html>`,
			wantErr:  "HTTP 503",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got ollamaRequest
			var gotPath, gotAuth string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				gotAuth = r.Header.Get("Authorization")
				raw, _ := io.ReadAll(r.Body)
				if err := json.Unmarshal(raw, &got); err != nil {
					t.Errorf("请求体解析失败: %v (%s)", err, raw)
				}
				status := tc.status
				if status == 0 {
					status = http.StatusOK
				}
				w.WriteHeader(status)
				_, _ = io.WriteString(w, tc.response)
			}))
			defer srv.Close()

			p, err := New(settings.Provider{Kind: "ollama", BaseURL: srv.URL, Model: "qwen2.5:7b"})
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			out, _, err := p.Translate(context.Background(), "hello\nworld", "auto", "zh-CN")

			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("意外失败: %v", err)
				}
				if out != tc.want {
					t.Errorf("译文 = %q, 期望 %q", out, tc.want)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("错误 = %v, 期望包含 %q", err, tc.wantErr)
			}

			if gotPath != "/api/chat" {
				t.Errorf("路径 = %q, 期望 /api/chat", gotPath)
			}
			// Local inference has no credential, so nothing must leak a key.
			if gotAuth != "" {
				t.Errorf("Authorization = %q, 本地服务不应带密钥", gotAuth)
			}
			if got.Model != "qwen2.5:7b" {
				t.Errorf("model = %q", got.Model)
			}
			if got.Stream == nil || *got.Stream {
				t.Errorf("stream = %v, 必须显式关闭才能拿到单个 JSON 响应", got.Stream)
			}
			if got.Options.Temperature <= 0 || got.Options.Temperature > 1 {
				t.Errorf("options.temperature = %v, 期望 0~1 之间的低温", got.Options.Temperature)
			}
			if len(got.Messages) != 2 || got.Messages[0].Role != "system" || got.Messages[1].Role != "user" {
				t.Fatalf("messages = %+v, 期望 system+user 两条", got.Messages)
			}
			if !strings.Contains(got.Messages[0].Content, LanguageName("zh-CN")) {
				t.Errorf("system = %q, 期望包含 %q", got.Messages[0].Content, LanguageName("zh-CN"))
			}
			if got.Messages[1].Content != "hello\nworld" {
				t.Errorf("user = %q", got.Messages[1].Content)
			}
		})
	}
}

func TestOllamaWithPrompt(t *testing.T) {
	var systems []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var got ollamaRequest
		_ = json.Unmarshal(raw, &got)
		if len(got.Messages) > 0 {
			systems = append(systems, got.Messages[0].Content)
		}
		_, _ = io.WriteString(w, `{"message":{"content":"好"},"done":true,"done_reason":"stop"}`)
	}))
	defer srv.Close()

	saved := "用书面语"
	orig, err := New(settings.Provider{Kind: "ollama", BaseURL: srv.URL, Prompt: saved})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	adHoc := "总结成要点"
	if _, _, err := orig.(Promptable).WithPrompt(adHoc).Translate(context.Background(), "x", "auto", "en"); err != nil {
		t.Fatalf("WithPrompt: %v", err)
	}
	if _, _, err := orig.Translate(context.Background(), "x", "auto", "en"); err != nil {
		t.Fatalf("original: %v", err)
	}
	if len(systems) != 2 {
		t.Fatalf("系统提示词 %d 条", len(systems))
	}
	if !strings.Contains(systems[0], adHoc) || strings.Contains(systems[0], saved) {
		t.Errorf("临时调用提示词不对: %q", systems[0])
	}
	if !strings.Contains(systems[1], saved) || strings.Contains(systems[1], adHoc) {
		t.Errorf("原 provider 被污染: %q", systems[1])
	}
	if orig.(*ollama).Prompt != saved {
		t.Errorf("原 Prompt 被改写: %q", orig.(*ollama).Prompt)
	}
}

func TestOllamaDefaults(t *testing.T) {
	o := newOllama("", "", "")
	if o.BaseURL != ollamaDefaultBaseURL {
		t.Errorf("默认 baseUrl = %q", o.BaseURL)
	}
	if o.Model != ollamaDefaultModel {
		t.Errorf("默认 model = %q", o.Model)
	}
	m, ok := Meta("ollama")
	if !ok {
		t.Fatal("ollama 未注册")
	}
	if !m.Free {
		t.Error("ollama 本地推理应标记 Free")
	}
	for _, f := range m.Fields {
		if f.Key == settings.FieldAPIKey {
			t.Error("ollama 不该要求 API Key 字段")
		}
	}
	if m.Default.BaseURL != ollamaDefaultBaseURL {
		t.Errorf("注册默认地址 = %q", m.Default.BaseURL)
	}
	// A keyless local service must still count as ready.
	bare := settings.Provider{Kind: "ollama", Enabled: true}
	if !bare.IsReady(m) {
		t.Error("空的 ollama 配置应视为就绪")
	}
	if !strings.HasPrefix(o.Label(), "Ollama · ") {
		t.Errorf("Label = %q", o.Label())
	}
}
