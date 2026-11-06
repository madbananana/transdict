package translate

import "testing"

// The panel lays an answer out as prose only where the service writes prose. A
// model registered without the flag shows the user asterisks and code fences; a
// one-sentence translator carrying it loses the plain quoting the card is styled
// for, so the boundary between the two is worth pinning down.
func TestOnlyModelsAskForMarkdown(t *testing.T) {
	models := []string{
		"openai", "claude", "gemini", "ollama", "claude-cli", "codex-cli", "copilot-cli",
		"deepseek", "moonshot", "zhipu", "groq", "minimax", "siliconflow", "xai", "mistral", "custom",
	}
	for _, kind := range models {
		meta, ok := Meta(kind)
		if !ok {
			t.Fatalf("%s 没有注册", kind)
		}
		if !meta.Markdown {
			t.Errorf("%s 是大模型，却没有声明按 Markdown 排版", kind)
		}
	}
	for _, kind := range []string{"google", "deepl", "bing", "caiyun", "youdao", "baidu", "tencent", "alibaba", "volc", "niutrans"} {
		if meta, ok := Meta(kind); ok && meta.Markdown {
			t.Errorf("%s 返回一句译文，不该按 Markdown 排版", kind)
		}
	}
}

// 网页版 exists so a card that looks wrong can be checked against the service: a
// link that cannot carry the query opens the front page and pretends to be an answer.
func TestEveryGoogleStylePageCarriesTheQuery(t *testing.T) {
	meta, ok := Meta("google")
	if !ok {
		t.Fatal("google 没有注册")
	}
	got := meta.SiteURL("你好 world", "auto", "zh-CN")
	want := "https://translate.google.com/?sl=auto&tl=zh-CN&text=%E4%BD%A0%E5%A5%BD+world&op=translate"
	if got != want {
		t.Errorf("google 网页版 = %q, want %q", got, want)
	}
}
