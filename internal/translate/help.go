package translate

import (
	"net/url"
	"strings"
)

// helpPage is the 参见 👉 row EasyDict puts under a refused answer: the site
// where the key is created or the quota is topped up. A network fault has no
// such page — retrying is the fix — so it carries none.
type helpPage struct {
	Title string
	URL   string
}

// kindHelp covers services whose key lives on a page of its own.
var kindHelp = map[string]helpPage{
	"alibaba":  {"Aliyun", "https://usercenter.console.aliyun.com/#/manage/ak"},
	"baidu":    {"Baidu", "https://fanyi-api.baidu.com/"},
	"caiyun":   {"Caiyun", "https://platform.caiyunapp.com/"},
	"claude":   {"Anthropic", "https://console.anthropic.com/settings/keys"},
	"deepl":    {"DeepL", "https://www.deepl.com/your-account/keys"},
	"doubao":   {"Volcengine", "https://console.volcengine.com/ark"},
	"gemini":   {"Google AI", "https://aistudio.google.com/app/apikey"},
	"niutrans": {"Niutrans", "https://niutrans.com/"},
	"tencent":  {"Tencent Cloud", "https://console.cloud.tencent.com/tmt"},
	"volc":     {"Volcengine", "https://console.volcengine.com/ark"},
}

// hostHelp covers the OpenAI-compatible presets. They all share the "openai"
// kind, so the endpoint host is what tells DeepSeek from OpenAI; a custom host
// has no page we can name and gets no link.
var hostHelp = map[string]helpPage{
	"api.openai.com":     {"OpenAI", "https://platform.openai.com/api-keys"},
	"api.deepseek.com":   {"DeepSeek", "https://platform.deepseek.com/api_keys"},
	"api.moonshot.cn":    {"Kimi", "https://platform.moonshot.cn/console/api-keys"},
	"open.bigmodel.cn":   {"Zhipu", "https://open.bigmodel.cn/usercenter/apikeys"},
	"api.groq.com":       {"Groq", "https://console.groq.com/keys"},
	"api.minimax.chat":   {"MiniMax", "https://platform.minimaxi.com/user-center/basic-information/interface-key"},
	"api.siliconflow.cn": {"SiliconFlow", "https://cloud.siliconflow.cn/account/ak"},
	"api.x.ai":           {"xAI", "https://console.x.ai/"},
	"api.mistral.ai":     {"Mistral", "https://console.mistral.ai/api-keys"},
}

// PromptFor returns the 参见 👉 link for an answer that failed because the key
// was refused or the quota ran out. Any other error — a timeout, a refused
// connection, a bad response — leaves it empty: no page fixes those.
func PromptFor(kind, baseURL, msg string) (title, link string) {
	if !keyRefused(msg) {
		return "", ""
	}
	return HelpPage(kind, baseURL)
}

// HelpPage names the site for a kind before anything has failed: the service
// page shows it beside 缺少密钥, so an empty key comes with a way to get one.
func HelpPage(kind, baseURL string) (title, link string) {
	page, ok := kindHelp[kind]
	if kind == "openai" {
		page, ok = hostHelp[hostOf(baseURL)]
	}
	if !ok {
		return "", ""
	}
	return page.Title, page.URL
}

func hostOf(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Hostname())
}

// keyRefused reads the status the providers put in their errors. The services
// word a dead key differently ("invalid x-api-key", "API key not valid",
// "insufficient quota"), but every one of them names the HTTP status, and the
// Chinese clouds answer 额度/余额 in the message.
func keyRefused(msg string) bool {
	m := strings.ToLower(msg)
	for _, hint := range []string{
		"http 401", "http 403", "http 429",
		"unauthorized", "invalid api key", "invalid x-api-key",
		"api key not valid", "apikey is invalid",
		"insufficient quota", "quota", "额度", "余额不足",
		"permission denied", "authorization was disabled",
	} {
		if strings.Contains(m, hint) {
			return true
		}
	}
	return false
}
