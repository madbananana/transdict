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

// expectedPreset mirrors the vendor table: a test-only copy so an accidental
// edit of a default has to be acknowledged here as well.
var expectedPreset = map[string]struct {
	baseURL string
	model   string
}{
	"deepseek":    {"https://api.deepseek.com/v1", "deepseek-chat"},
	"moonshot":    {"https://api.moonshot.cn/v1", "moonshot-v1-8k"},
	"zhipu":       {"https://open.bigmodel.cn/api/paas/v4", "glm-4-flash"},
	"groq":        {"https://api.groq.com/openai/v1", "llama-3.3-70b-versatile"},
	"minimax":     {"https://api.minimax.chat/v1", "MiniMax-Text-01"},
	"siliconflow": {"https://api.siliconflow.cn/v1", "deepseek-ai/DeepSeek-V3"},
	"xai":         {"https://api.x.ai/v1", "grok-3"},
	"mistral":     {"https://api.mistral.ai/v1", "mistral-large-latest"},
}

func TestPresetsRegisteredWithDefaults(t *testing.T) {
	for kind, want := range expectedPreset {
		m, ok := Meta(kind)
		if !ok {
			t.Fatalf("%s 未注册", kind)
		}
		if m.Category != settings.CatTranslate {
			t.Errorf("%s category = %q", kind, m.Category)
		}
		if m.Default.BaseURL != want.baseURL || m.Default.Model != want.model {
			t.Errorf("%s 默认值 = %+v, 期望 %s / %s", kind, m.Default, want.baseURL, want.model)
		}
		// Honesty check: the settings list must say these ride the OpenAI
		// protocol rather than a vendor SDK.
		if !strings.Contains(m.Note, "OpenAI 兼容") && !strings.Contains(m.Name, "OpenAI 兼容") {
			t.Errorf("%s 未在 Name/Note 里说明是 OpenAI 兼容接口: name=%q note=%q", kind, m.Name, m.Note)
		}
		if !m.Multi {
			t.Errorf("%s 应允许添加多个实例", kind)
		}

		// Empty settings fall back to the vendor's own host and model.
		p, err := New(settings.Provider{Kind: kind, APIKey: "k"})
		if err != nil {
			t.Fatalf("%s New: %v", kind, err)
		}
		o := p.(*openAI)
		if o.BaseURL != want.baseURL {
			t.Errorf("%s BaseURL = %q, 期望 %q", kind, o.BaseURL, want.baseURL)
		}
		if o.Model != want.model {
			t.Errorf("%s Model = %q, 期望 %q", kind, o.Model, want.model)
		}
	}

	if _, ok := Meta("custom"); !ok {
		t.Fatal("custom 未注册")
	}
	cm, _ := Meta("custom")
	if !cm.Multi {
		t.Error("custom 应允许添加多个实例")
	}
	if cm.Default.BaseURL != "" {
		t.Errorf("custom 不应内置默认主机: %q", cm.Default.BaseURL)
	}
	if _, err := New(settings.Provider{Kind: "custom", BaseURL: "https://llm.internal/v1"}); err != nil {
		t.Errorf("custom 填了地址却报错: %v", err)
	}
	if _, err := New(settings.Provider{Kind: "custom"}); err == nil ||
		!strings.Contains(err.Error(), "接口地址") {
		t.Errorf("custom 空地址应提示接口地址, 实际: %v", err)
	}
}

// TestPresetsShareOpenAIWireFormat proves every preset hits the same
// chat/completions contract, which is the whole reason they can reuse one
// implementation.
func TestPresetsShareOpenAIWireFormat(t *testing.T) {
	for kind := range expectedPreset {
		t.Run(kind, func(t *testing.T) {
			var path, auth, ctype string
			var body map[string]any
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				path, auth, ctype = r.URL.Path, r.Header.Get("Authorization"), r.Header.Get("Content-Type")
				raw, _ := io.ReadAll(r.Body)
				_ = json.Unmarshal(raw, &body)
				_, _ = io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"译文"}}]}`)
			}))
			defer srv.Close()

			// The user's own endpoint always wins over the vendor default.
			p, err := New(settings.Provider{Kind: kind, BaseURL: srv.URL, APIKey: "key-" + kind, Model: "m"})
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			out, _, err := p.Translate(context.Background(), "text", "auto", "zh-CN")
			if err != nil {
				t.Fatalf("Translate: %v", err)
			}
			if out != "译文" {
				t.Errorf("out = %q", out)
			}
			if path != "/chat/completions" {
				t.Errorf("%s path = %q", kind, path)
			}
			if auth != "Bearer key-"+kind {
				t.Errorf("%s Authorization = %q", kind, auth)
			}
			if ctype != "application/json" {
				t.Errorf("%s Content-Type = %q", kind, ctype)
			}
			if body["model"] != "m" {
				t.Errorf("%s model = %v", kind, body["model"])
			}
			msgs, ok := body["messages"].([]any)
			if !ok || len(msgs) != 2 {
				t.Fatalf("%s messages = %v", kind, body["messages"])
			}
			sys := msgs[0].(map[string]any)
			if sys["role"] != "system" || !strings.Contains(sys["content"].(string), LanguageName("zh-CN")) {
				t.Errorf("%s system 提示词 = %v", kind, sys)
			}
			user := msgs[1].(map[string]any)
			if user["role"] != "user" || user["content"] != "text" {
				t.Errorf("%s user 消息 = %v", kind, user)
			}
		})
	}
}

func TestPresetPromptFieldAndReadiness(t *testing.T) {
	for kind := range expectedPreset {
		m, _ := Meta(kind)
		var sawKey, sawPrompt bool
		for _, f := range m.Fields {
			switch f.Key {
			case settings.FieldAPIKey:
				sawKey = true
				if !f.Required || !f.Secret {
					t.Errorf("%s API Key 字段应为必填且隐藏: %+v", kind, f)
				}
			case settings.FieldPrompt:
				sawPrompt = true
			}
		}
		if !sawKey || !sawPrompt {
			t.Errorf("%s 字段不齐: %+v", kind, m.Fields)
		}
		if (settings.Provider{Kind: kind, Enabled: true}).IsReady(m) {
			t.Errorf("%s 无密钥不应就绪", kind)
		}
		filled := settings.Provider{Kind: kind, Enabled: true, APIKey: "k"}
		if !filled.IsReady(m) {
			t.Errorf("%s 有密钥应就绪", kind)
		}
	}
}
