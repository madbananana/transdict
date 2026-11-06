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

type claudeRequest struct {
	Model       string `json:"model"`
	MaxTokens   int    `json:"max_tokens"`
	System      string `json:"system"`
	Temperature any    `json:"temperature"`
	Messages    []struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	} `json:"messages"`
}

func TestClaudeTranslate(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		response string
		want     string
		wantErr  string
	}{
		{
			name:     "content 文本块数组",
			response: `{"type":"message","content":[{"type":"text","text":"第一段\n"},{"type":"text","text":"第二段"}],"stop_reason":"end_turn"}`,
			want:     "第一段\n第二段",
		},
		{
			name:     "代理返回纯字符串 content",
			response: `{"type":"message","content":"字符串译文","stop_reason":"end_turn"}`,
			want:     "字符串译文",
		},
		{
			name:     "达到 max_tokens",
			response: `{"type":"message","content":[{"type":"text","text":"半句"}],"stop_reason":"max_tokens"}`,
			want:     "半句",
			wantErr:  "截断",
		},
		{
			name:     "模型拒绝",
			response: `{"type":"message","content":[],"stop_reason":"refusal"}`,
			wantErr:  "拒绝",
		},
		{
			name:     "没有正文",
			response: `{"type":"message","content":[],"stop_reason":"end_turn"}`,
			wantErr:  "未给出译文",
		},
		{
			name:     "密钥错误",
			status:   http.StatusUnauthorized,
			response: `{"type":"error","error":{"type":"authentication_error","message":"invalid x-api-key"}}`,
			wantErr:  "invalid x-api-key",
		},
		{
			name:     "过载",
			status:   http.StatusTooManyRequests,
			response: `{"type":"error","error":{"type":"rate_limit_error","message":"Too many requests"}}`,
			wantErr:  "HTTP 429",
		},
		{
			name:     "非 JSON",
			status:   http.StatusBadGateway,
			response: `<html>gateway</html>`,
			wantErr:  "HTTP 502",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got claudeRequest
			var gotPath, gotKey, gotVersion, gotCT string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				gotKey = r.Header.Get("x-api-key")
				gotVersion = r.Header.Get("anthropic-version")
				gotCT = r.Header.Get("Content-Type")
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

			p, err := New(settings.Provider{
				Kind: "claude", BaseURL: srv.URL, APIKey: "sk-ant-test", Model: "claude-sonnet-4-5",
			})
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
			} else {
				if err == nil {
					t.Fatalf("期望错误包含 %q, 实际得到 %q", tc.wantErr, out)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("错误 = %q, 期望包含 %q", err, tc.wantErr)
				}
			}

			if gotPath != "/v1/messages" {
				t.Errorf("路径 = %q, 期望 /v1/messages", gotPath)
			}
			if gotKey != "sk-ant-test" {
				t.Errorf("x-api-key = %q", gotKey)
			}
			if gotVersion != "2023-06-01" {
				t.Errorf("anthropic-version = %q", gotVersion)
			}
			if gotCT != "application/json" {
				t.Errorf("Content-Type = %q", gotCT)
			}
			if got.Model != "claude-sonnet-4-5" {
				t.Errorf("model = %q", got.Model)
			}
			// max_tokens is mandatory upstream, so a zero request would 400.
			if got.MaxTokens <= 0 {
				t.Errorf("max_tokens = %d, 必须为正", got.MaxTokens)
			}
			if !strings.Contains(got.System, LanguageName("zh-CN")) {
				t.Errorf("system = %q, 期望包含 %q", got.System, LanguageName("zh-CN"))
			}
			if len(got.Messages) != 1 || got.Messages[0].Role != "user" {
				t.Fatalf("messages = %+v, 期望单条 user 消息", got.Messages)
			}
			var userText string
			if err := json.Unmarshal(got.Messages[0].Content, &userText); err != nil {
				t.Fatalf("user content 应为字符串: %v", err)
			}
			if userText != "hello\nworld" {
				t.Errorf("user 文本 = %q, 期望逐字原文", userText)
			}
		})
	}
}

func TestClaudeWithPrompt(t *testing.T) {
	var systems []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var got claudeRequest
		_ = json.Unmarshal(raw, &got)
		systems = append(systems, got.System)
		_, _ = io.WriteString(w, `{"type":"message","content":[{"type":"text","text":"好"}],"stop_reason":"end_turn"}`)
	}))
	defer srv.Close()

	saved := "保留术语原文"
	orig, err := New(settings.Provider{Kind: "claude", BaseURL: srv.URL, APIKey: "k", Prompt: saved})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	adHoc := "缩写成一句话"
	if _, _, err := orig.(Promptable).WithPrompt(adHoc).Translate(context.Background(), "x", "auto", "en"); err != nil {
		t.Fatalf("WithPrompt: %v", err)
	}
	if _, _, err := orig.Translate(context.Background(), "x", "auto", "en"); err != nil {
		t.Fatalf("original: %v", err)
	}
	if len(systems) != 2 {
		t.Fatalf("系统提示词 %d 条, 期望 2", len(systems))
	}
	if !strings.Contains(systems[0], adHoc) {
		t.Errorf("临时指令未写入 system: %q", systems[0])
	}
	if strings.Contains(systems[0], saved) {
		t.Errorf("临时调用不应叠加已保存指令: %q", systems[0])
	}
	if !strings.Contains(systems[1], saved) || strings.Contains(systems[1], adHoc) {
		t.Errorf("原 provider 被污染: %q", systems[1])
	}
	if orig.(*claude).Prompt != saved {
		t.Errorf("原 Prompt 字段被改写: %q", orig.(*claude).Prompt)
	}
}

func TestClaudeDefaults(t *testing.T) {
	c := newClaude("", "", "", "")
	if c.BaseURL != claudeDefaultBaseURL {
		t.Errorf("默认 baseUrl = %q", c.BaseURL)
	}
	if c.Model != claudeDefaultModel {
		t.Errorf("默认 model = %q", c.Model)
	}
	if !strings.Contains(c.systemPrompt("ja"), LanguageName("ja")) {
		t.Errorf("提示词缺少语言名: %q", c.systemPrompt("ja"))
	}
	if !strings.HasPrefix(c.Label(), "Claude · ") {
		t.Errorf("Label = %q", c.Label())
	}
	m, ok := Meta("claude")
	if !ok {
		t.Fatal("claude 未注册")
	}
	if m.Default.BaseURL != claudeDefaultBaseURL || m.Default.Model != claudeDefaultModel {
		t.Errorf("注册默认值 = %+v", m.Default)
	}
}
