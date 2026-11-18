package translate

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"transdict/internal/netp"
	"transdict/internal/settings"
)

// OpenAI translates through any OpenAI-compatible chat endpoint. Layout
// preservation matters for screenshot translation, where lines must stay
// one-to-one with the OCR output above them.
type openAI struct {
	BaseURL string
	APIKey  string
	Model   string
	Prompt  string
	// Effort is EasyDict's 思考程度, forwarded as reasoning_effort. Empty means
	// the field is left out so plain chat models keep working.
	Effort string
	// HideThink drops an inline reasoning block from the answer.
	HideThink bool

	// Stream is 启用流式传输. Empty in the stored row means on, so a service saved
	// before the switch existed keeps its streamed card.
	Stream bool

	// CustomPrompt is 使用自定义 Prompt: the saved system/user pair replaces the
	// built-in per-task instructions. An empty user prompt falls back to the
	// query text, and an empty system prompt sends no system message at all.
	CustomPrompt bool
	SystemPrompt string
	UserPrompt   string

	// The task switches decide which built-in instruction a query gets, and
	// which queries the service answers at all. They default on, matching
	// EasyDict; only an explicit 关闭 turns one off.
	TaskTranslate  bool
	TaskSentence   bool
	TaskDictionary bool

	// FirstLang is 第一语言, used by ${{firstLanguage}}. It is supplied per run
	// through WithFirstLanguage, since it lives in the app config rather than
	// on the service row.
	FirstLang string

	// override marks a call-specific instruction such as 润色, which beats the
	// saved custom prompt for that one call.
	override bool

	httpc *http.Client
}

func init() {
	Register(settings.ServiceType{
		Kind: "openai", Name: "AI 模型（OpenAI 兼容）", Note: "DeepSeek、通义、Kimi、硅基流动、Ollama 等均可",
		Multi: true, Markdown: true, Prompt: true,
		Models:      []string{"gpt-4o-mini", "gpt-4o", "o3-mini"},
		FetchModels: true,
		Fields: []settings.Field{
			{Key: settings.FieldBaseURL, Label: "API 请求地址",
				Placeholder: "完整请求 URL，例如 https://api.openai.com/v1/chat/completions"},
			{Key: settings.FieldAPIKey, Label: "API Key", Secret: true, Required: true},
			{Key: settings.FieldModel, Label: "模型", Placeholder: "gpt-4o-mini"},
			{Key: settings.FieldSupportedModels, Label: "支持的模型", Placeholder: "模型名字，多个模型使用逗号（,）分隔", Advanced: true},
			{Key: settings.FieldPrompt, Label: "附加指令", Text: true, Advanced: true},
			reasoningEffortField(),
			hideThinkField(),
			customPromptField(),
			systemPromptField(),
			userPromptField(),
			usageStatusField(),
			taskField(settings.FieldTranslation, "翻译"),
			taskField(settings.FieldSentence, "句子分析"),
			taskField(settings.FieldDictionary, "查单词"),
			streamField(),
		},
		Default: settings.Provider{BaseURL: "https://api.openai.com/v1", Model: "gpt-4o-mini"},
	}, func(p settings.Provider) (Provider, error) {
		return newOpenAI(p), nil
	})
}

// thinkBlock matches the inline reasoning block reasoning models wrap their
// scratchpad in, including one left unclosed after a truncation.
var thinkBlock = regexp.MustCompile(`(?is)<think\b[^>]*>.*?(?:</think\s*>|$)`)

// reasoningEffortField is EasyDict's 思考程度 picker, shared by the generic
// OpenAI entry and every vendor preset because they all speak the same
// chat/completions body.
func reasoningEffortField() settings.Field {
	return settings.Field{
		Key:      settings.FieldReasoningEffort,
		Label:    "思考程度",
		Advanced: true,
		Options: []settings.FieldOption{
			{Value: "", Label: "关闭思考"},
			{Value: "high", Label: "高"},
			{Value: "max", Label: "最大"},
		},
	}
}

// hideThinkField is EasyDict's 隐藏 <think> 标签内容. Reasoning endpoints often
// put their scratchpad in the same content string; leaving it in makes the card
// look like it answered twice.
func hideThinkField() settings.Field {
	return settings.Field{
		Key:      settings.FieldHideThink,
		Label:    "隐藏 <think> 标签内容",
		Advanced: true,
		Options: []settings.FieldOption{
			{Value: "", Label: "不隐藏"},
			{Value: "1", Label: "隐藏"},
		},
	}
}

func customPromptField() settings.Field {
	return settings.Field{
		Key:      settings.FieldEnableCustomPrompt,
		Label:    "使用自定义 Prompt",
		Note:     "开启后不再使用默认的翻译、句子分析和查单词 Prompt。",
		Advanced: true,
		Options: []settings.FieldOption{
			{Value: "", Label: "关闭"},
			{Value: "1", Label: "开启"},
		},
	}
}

func systemPromptField() settings.Field {
	return settings.Field{
		Key:         settings.FieldSystemPrompt,
		Label:       "系统 Prompt",
		Placeholder: "你是一个有用的翻译助手。",
		Text:        true,
		Advanced:    true,
	}
}

func userPromptField() settings.Field {
	return settings.Field{
		Key:         settings.FieldUserPrompt,
		Label:       "用户 Prompt",
		Placeholder: "将下面 ${{queryFromLanguage}} 的文本翻译成 ${{queryTargetLanguage}}: ${{queryText}}",
		Text:        true,
		Advanced:    true,
		Note:        "可用变量：${{queryText}}、${{queryFromLanguage}}、${{queryTargetLanguage}}、${{firstLanguage}}。留空时发送查询原文。",
	}
}

func usageStatusField() settings.Field {
	return settings.Field{
		Key:   settings.FieldUsageStatus,
		Label: "使用状态",
		Options: []settings.FieldOption{
			{Value: settings.UsageDefault, Label: "默认（跟随窗口选择）"},
			{Value: settings.UsageAlwaysOn, Label: "始终开启"},
			{Value: settings.UsageAlwaysOff, Label: "始终关闭"},
		},
	}
}

// taskField is one of 翻译/句子分析/查单词. 开启 is the stored "1" so a row saved
// before the switch existed reads as on, and 关闭 is an explicit "0".
func taskField(key, label string) settings.Field {
	return settings.Field{
		Key:   key,
		Label: label,
		Options: []settings.FieldOption{
			{Value: "1", Label: "开启"},
			{Value: "0", Label: "关闭"},
		},
	}
}

// streamField is EasyDict's 启用流式传输. A proxy that buffers, or a gateway that
// only answers stream=false, can keep the service by taking the one-shot path;
// empty means on, as shipped.
func streamField() settings.Field {
	return settings.Field{
		Key:      settings.FieldStream,
		Label:    "启用流式传输",
		Note:     "如果端点不支持流式响应，请关闭此选项。",
		Advanced: true,
		Options: []settings.FieldOption{
			{Value: "", Label: "开启"},
			{Value: "0", Label: "关闭"},
		},
	}
}

// StreamingEnabled answers the 启用流式传输 switch: the pipeline then takes the
// one-shot path and the card does not pretend to be waiting on a stream.
func (o *openAI) StreamingEnabled() bool { return o.Stream }

func newOpenAI(p settings.Provider) *openAI {
	baseURL := p.BaseURL
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	model := p.Model
	if model == "" {
		model = "gpt-4o-mini"
	}
	return &openAI{
		BaseURL:   strings.TrimRight(baseURL, "/"),
		APIKey:    p.APIKey,
		Model:     model,
		Prompt:    p.Prompt,
		Effort:    p.Get(settings.FieldReasoningEffort),
		HideThink: p.Get(settings.FieldHideThink) == "1",
		Stream:    p.Get(settings.FieldStream) != "0",

		CustomPrompt: p.Get(settings.FieldEnableCustomPrompt) == "1",
		SystemPrompt: p.Get(settings.FieldSystemPrompt),
		UserPrompt:   p.Get(settings.FieldUserPrompt),

		TaskTranslate:  p.Get(settings.FieldTranslation) != "0",
		TaskSentence:   p.Get(settings.FieldSentence) != "0",
		TaskDictionary: p.Get(settings.FieldDictionary) != "0",

		httpc: netp.Client(120 * time.Second),
	}
}

func (o *openAI) Label() string { return "AI 模型 · " + o.Model }

// WithPrompt replaces the instruction for one call. The copy keeps the
// credentials but not the user's saved 附加指令 or custom prompt, since an
// ad-hoc prompt such as 润色 would otherwise be diluted by it.
func (o *openAI) WithPrompt(prompt string) Provider {
	clone := *o
	clone.Prompt = prompt
	clone.override = true
	return &clone
}

// WithFirstLanguage supplies ${{firstLanguage}} for one call.
func (o *openAI) WithFirstLanguage(lang string) Provider {
	clone := *o
	clone.FirstLang = strings.TrimSpace(lang)
	return &clone
}

// SupportsQuery reports whether a task switch covers this query. When every
// switch is off the service stays out of the query, rather than answering a
// kind of question the user turned off.
func (o *openAI) SupportsQuery(text, sourceLang, _ string) bool {
	return o.queryType(text, sourceLang) != ""
}

// queryType mirrors EasyDict: a headword asks 查单词, an English sentence asks
// 句子分析, everything else is 翻译. A disabled type is skipped rather than
// failing the service.
func (o *openAI) queryType(text, sourceLang string) string {
	if o.TaskDictionary && dictionaryQuery(text, sourceLang) {
		return "dictionary"
	}
	if o.TaskSentence && sentenceQuery(text, sourceLang) {
		return "sentence"
	}
	if o.TaskTranslate {
		return "translation"
	}
	return ""
}

// English source is what EasyDict's sentence analysis is written for. An
// unknown source is treated as English so 自动检测 still reaches it.
func englishSource(sourceLang string) bool {
	return sourceLang == "" || sourceLang == "auto" || strings.HasPrefix(sourceLang, "en")
}

// dictionaryQuery is EasyDict's 查单词 gate: one Latin word, or a short Chinese
// word/phrase. Internal punctuation means the text is a sentence.
func dictionaryQuery(text, sourceLang string) bool {
	text = strings.TrimSpace(text)
	if text == "" || strings.ContainsAny(text, "，,；;：:。！？!?…\n\r\t") {
		return false
	}
	if hanOnly(text) {
		return utf8.RuneCountInString(text) <= 8
	}
	if !englishSource(sourceLang) {
		return false
	}
	return len(strings.Fields(text)) == 1 && utf8.RuneCountInString(text) <= 24
}

// sentenceQuery wants more than one word, or terminal punctuation: a lone word
// is a lookup, not a sentence to analyse.
func sentenceQuery(text, sourceLang string) bool {
	text = strings.TrimSpace(text)
	if text == "" || !englishSource(sourceLang) {
		return false
	}
	return len(strings.Fields(text)) > 1 || strings.ContainsAny(text, ".!?")
}

// hanOnly reports text made of Han characters and spaces, which is how a bare
// 中文 query is told apart from a Latin word.
func hanOnly(text string) bool {
	han := false
	for _, r := range text {
		switch {
		case unicode.Is(unicode.Han, r):
			han = true
		case unicode.IsSpace(r):
		default:
			return false
		}
	}
	return han
}

func (o *openAI) instruction(targetLang string) string {
	sb := strings.Builder{}
	sb.WriteString("你是专业译者。把用户提供的文本翻译成")
	sb.WriteString(LanguageName(targetLang))
	sb.WriteString("，逐行对应原文的换行，保留数字、代码、专有名词与标点格式。" +
		"只输出译文，不要解释、不要加引号或代码块。")
	o.appendExtra(&sb)
	return sb.String()
}

// dictionaryInstruction is the built-in 查单词 prompt EasyDict swaps in for a
// headword. It is our own wording, not the reference app's.
func (o *openAI) dictionaryInstruction(targetLang string) string {
	sb := strings.Builder{}
	sb.WriteString("你是词典编纂者。用")
	sb.WriteString(LanguageName(targetLang))
	sb.WriteString("解释用户给出的词语：先给读音或音标和词性，再给释义、常见搭配与一两个例句。" +
		"按 Markdown 组织，只输出词条内容，不要寒暄。")
	o.appendExtra(&sb)
	return sb.String()
}

// sentenceInstruction is the built-in 句子分析 prompt.
func (o *openAI) sentenceInstruction(targetLang string) string {
	sb := strings.Builder{}
	sb.WriteString("你是语言老师。分析用户给出的句子：先给出")
	sb.WriteString(LanguageName(targetLang))
	sb.WriteString("译文，再讲清句子结构、关键词和难点。按 Markdown 组织，只输出分析内容，不要寒暄。")
	o.appendExtra(&sb)
	return sb.String()
}

func (o *openAI) appendExtra(sb *strings.Builder) {
	if strings.TrimSpace(o.Prompt) != "" {
		sb.WriteString("\n补充要求：" + o.Prompt)
	}
}

// messages is the chat body for one query: the custom prompt pair when 使用
// 自定义 Prompt is on, otherwise the instruction for the query's task.
func (o *openAI) messages(text, sourceLang, targetLang string) []map[string]string {
	if o.CustomPrompt && !o.override {
		user := strings.TrimSpace(o.expand(o.UserPrompt, text, sourceLang, targetLang))
		if user == "" {
			user = text
		}
		var out []map[string]string
		if system := strings.TrimSpace(o.expand(o.SystemPrompt, text, sourceLang, targetLang)); system != "" {
			out = append(out, map[string]string{"role": "system", "content": system})
		}
		return append(out, map[string]string{"role": "user", "content": user})
	}
	instruction := o.instruction(targetLang)
	switch o.queryType(text, sourceLang) {
	case "dictionary":
		instruction = o.dictionaryInstruction(targetLang)
	case "sentence":
		instruction = o.sentenceInstruction(targetLang)
	}
	return []map[string]string{
		{"role": "system", "content": instruction},
		{"role": "user", "content": text},
	}
}

// expand fills EasyDict's custom-prompt variables.
func (o *openAI) expand(prompt, text, sourceLang, targetLang string) string {
	if prompt == "" {
		return ""
	}
	return strings.NewReplacer(
		"${{queryText}}", text,
		"${{queryFromLanguage}}", queryLanguageName(sourceLang),
		"${{queryTargetLanguage}}", LanguageName(targetLang),
		"${{firstLanguage}}", queryLanguageName(o.FirstLang),
	).Replace(prompt)
}

// queryLanguageName names the source; 自动检测 is EasyDict's word for an
// unnamed source, so a prompt reads naturally instead of saying "auto".
func queryLanguageName(code string) string {
	if code == "" || code == "auto" {
		return "自动检测"
	}
	return LanguageName(code)
}

func (o *openAI) Translate(ctx context.Context, text, sourceLang, targetLang string) (string, string, error) {
	resp, err := o.post(ctx, text, sourceLang, targetLang, false)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return "", "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("AI 翻译 HTTP %d: %s", resp.StatusCode, truncate(string(raw)))
	}
	if err := bodyProblem(resp.Header.Get("Content-Type"), raw); err != nil {
		return "", "", err
	}
	return o.parseReply(raw)
}

// bodyProblem names the two misconfigurations EasyDict explains instead of
// echoing the page: an HTML login or error page, and any body that is not the
// JSON the endpoint promised. Raw bytes say nothing useful about the address.
func bodyProblem(contentType string, raw []byte) error {
	if strings.Contains(strings.ToLower(contentType), "text/html") ||
		strings.HasPrefix(strings.TrimSpace(string(raw)), "<") {
		return errors.New("端点返回了 HTML 页面。请检查端点 URL 是否正确，是否包含完整的 API 路径。")
	}
	body := strings.TrimSpace(string(raw))
	if body != "" && !strings.HasPrefix(body, "{") && !strings.HasPrefix(body, "[") {
		return errors.New("响应格式不符合预期。该端点可能不完全兼容 OpenAI 接口，请检查端点 URL 和 API 文档。")
	}
	return nil
}

// ListModels reads the endpoint's own catalog for 从云端获取模型. The OpenAI
// shape is the one compatible gateways copy, and a proxy answers for itself
// because the saved endpoint is the one asked.
func (o *openAI) ListModels(ctx context.Context) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, o.modelsURL(), nil)
	if err != nil {
		return nil, err
	}
	if o.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+o.APIKey)
	}
	resp, err := o.httpc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("获取模型列表 HTTP %d: %s", resp.StatusCode, truncate(string(raw)))
	}
	var body struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, errors.New("模型列表格式不符合预期")
	}
	models := make([]string, 0, len(body.Data))
	for _, m := range body.Data {
		if id := strings.TrimSpace(m.ID); id != "" {
			models = append(models, id)
		}
	}
	return models, nil
}

// TranslateStream asks the same endpoint for a streamed answer. An endpoint
// that ignores `stream` and replies with one JSON body is accepted as-is, so
// the feature never turns a working service into an error.
func (o *openAI) TranslateStream(ctx context.Context, text, sourceLang, targetLang string,
	onText func(string)) (string, string, error) {
	resp, err := o.post(ctx, text, sourceLang, targetLang, true)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	if !strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		if err != nil {
			return "", "", err
		}
		if resp.StatusCode != http.StatusOK {
			return "", "", fmt.Errorf("AI 翻译 HTTP %d: %s", resp.StatusCode, truncate(string(raw)))
		}
		if err := bodyProblem(resp.Header.Get("Content-Type"), raw); err != nil {
			return "", "", err
		}
		out, detected, err := o.parseReply(raw)
		if out != "" && onText != nil {
			onText(out)
		}
		return out, detected, err
	}
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
		return "", "", fmt.Errorf("AI 翻译 HTTP %d: %s", resp.StatusCode, truncate(string(raw)))
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64<<10), 1<<20)
	var answer strings.Builder
	last := ""
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content json.RawMessage `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		// A keep-alive or a vendor comment is not a failed answer.
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		if chunk.Error.Message != "" {
			return "", "", fmt.Errorf("AI 翻译: %s", chunk.Error.Message)
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		piece, err := contentText(chunk.Choices[0].Delta.Content)
		if err != nil || piece == "" {
			continue
		}
		answer.WriteString(piece)
		if visible := o.visible(answer.String()); visible != last {
			last = visible
			if onText != nil {
				onText(visible)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return "", "", err
	}
	out := o.visible(answer.String())
	if onText != nil {
		onText(out)
	}
	if strings.TrimSpace(out) == "" {
		return "", "", fmt.Errorf("AI 翻译返回空结果")
	}
	return strings.TrimSpace(out), "", nil
}

// chatURL accepts the two forms people paste: a base such as .../v1, and the
// complete .../chat/completions address EasyDict's OpenAI field documents.
// Appending blindly would request .../chat/completions/chat/completions.
func (o *openAI) chatURL() string {
	base := strings.TrimRight(o.BaseURL, "/")
	if strings.HasSuffix(base, "/chat/completions") {
		return base
	}
	return base + "/chat/completions"
}

// modelsURL is the catalog beside the same address, whichever form was saved.
func (o *openAI) modelsURL() string {
	base := strings.TrimSuffix(strings.TrimRight(o.BaseURL, "/"), "/chat/completions")
	return base + "/models"
}

func (o *openAI) post(ctx context.Context, text, sourceLang, targetLang string, stream bool) (*http.Response, error) {
	payloadIn := map[string]any{
		"model":       o.Model,
		"messages":    o.messages(text, sourceLang, targetLang),
		"temperature": 0.2,
	}
	if o.Effort != "" {
		payloadIn["reasoning_effort"] = o.Effort
	}
	if stream {
		payloadIn["stream"] = true
	}
	body, _ := json.Marshal(payloadIn)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.chatURL(),
		bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+o.APIKey)
	return o.httpc.Do(req)
}

// visible applies 隐藏 <think> 标签内容 to a partial answer as well, so the tag
// never flashes in the card before the final trim.
func (o *openAI) visible(s string) string {
	if !o.HideThink {
		return s
	}
	return thinkBlock.ReplaceAllString(s, "")
}

func (o *openAI) parseReply(raw []byte) (string, string, error) {
	var payload struct {
		Choices []struct {
			Message struct {
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", "", fmt.Errorf("AI 翻译响应解析失败: %s", truncate(string(raw)))
	}
	if len(payload.Choices) == 0 {
		if payload.Error.Message != "" {
			return "", "", fmt.Errorf("AI 翻译: %s", payload.Error.Message)
		}
		return "", "", fmt.Errorf("AI 翻译返回空结果: %s", truncate(string(raw)))
	}

	out, err := contentText(payload.Choices[0].Message.Content)
	if err != nil {
		return "", "", err
	}
	return strings.TrimSpace(o.visible(out)), "", nil
}

func contentText(raw json.RawMessage) (string, error) {
	if len(raw) == 0 {
		return "", nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s, nil
	}
	var parts []struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &parts); err != nil {
		return "", fmt.Errorf("AI 翻译 content 格式未知: %s", truncate(string(raw)))
	}
	var sb strings.Builder
	for _, p := range parts {
		sb.WriteString(p.Text)
	}
	return sb.String(), nil
}
