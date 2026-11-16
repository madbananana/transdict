package translate

import "testing"

func TestPromptForRefusedKey(t *testing.T) {
	cases := []struct {
		name    string
		kind    string
		baseURL string
		msg     string
		title   string
		url     string
	}{
		{"OpenAI 401", "openai", "https://api.openai.com/v1",
			"AI 翻译 HTTP 401: invalid api key", "OpenAI", "https://platform.openai.com/api-keys"},
		{"预设按主机区分", "openai", "https://api.deepseek.com/v1",
			"AI 翻译 HTTP 401: Authentication Fails", "DeepSeek", "https://platform.deepseek.com/api_keys"},
		{"Claude 403", "claude", "", "Claude HTTP 403: forbidden",
			"Anthropic", "https://console.anthropic.com/settings/keys"},
		{"Gemini 密钥无效", "gemini", "", "API key not valid",
			"Google AI", "https://aistudio.google.com/app/apikey"},
		{"额度不足", "baidu", "", "百度翻译 HTTP 429: 余额不足",
			"Baidu", "https://fanyi-api.baidu.com/"},
	}
	for _, tc := range cases {
		title, link := PromptFor(tc.kind, tc.baseURL, tc.msg)
		if title != tc.title || link != tc.url {
			t.Errorf("%s: PromptFor = %q %q, want %q %q", tc.name, title, link, tc.title, tc.url)
		}
	}
}

func TestPromptForQuietWhenNoPageFixesIt(t *testing.T) {
	cases := []struct{ name, kind, baseURL, msg string }{
		{"网络错误", "openai", "https://api.openai.com/v1", "dial tcp: connection refused"},
		{"自定义主机", "openai", "https://llm.example.com/v1", "HTTP 401: unauthorized"},
		{"免密钥服务", "bing", "", "HTTP 429: too many requests"},
		{"未知种类", "nope", "", "HTTP 401: unauthorized"},
	}
	for _, tc := range cases {
		if title, link := PromptFor(tc.kind, tc.baseURL, tc.msg); title != "" || link != "" {
			t.Errorf("%s: PromptFor = %q %q, want empty", tc.name, title, link)
		}
	}
}

// HelpPage is what the service page asks before anything has failed, so it
// answers without an error message — the same map, no refusal check.
func TestHelpPageBeforeAnyFailure(t *testing.T) {
	if title, link := HelpPage("openai", "https://api.deepseek.com/v1"); title != "DeepSeek" ||
		link != "https://platform.deepseek.com/api_keys" {
		t.Errorf("HelpPage(openai/deepseek) = %q %q", title, link)
	}
	if title, link := HelpPage("gemini", ""); title != "Google AI" ||
		link != "https://aistudio.google.com/app/apikey" {
		t.Errorf("HelpPage(gemini) = %q %q", title, link)
	}
	for _, tc := range []struct{ kind, baseURL string }{
		{"google", ""},
		{"bing", ""},
		{"openai", "https://llm.example.com/v1"},
	} {
		if title, link := HelpPage(tc.kind, tc.baseURL); title != "" || link != "" {
			t.Errorf("HelpPage(%s, %s) = %q %q, want empty", tc.kind, tc.baseURL, title, link)
		}
	}
}
