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

func doubaoFor(url string) *doubao {
	return newDoubao(settings.Provider{BaseURL: url, APIKey: "key"})
}

// The dedicated translation endpoint takes the language pair as request
// options, not as an instruction in a chat message.
func TestDoubaoRequestShapeAndStream(t *testing.T) {
	var got map[string]any
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &got); err != nil {
			t.Errorf("请求体不是 JSON: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "event: response.output_text.delta\n"+
			`data: {"type":"response.output_text.delta","delta":"你"}`+"\n\n"+
			"event: response.output_text.delta\n"+
			`data: {"type":"response.output_text.delta","delta":"好"}`+"\n\n"+
			"data: [DONE]\n\n")
	}))
	defer srv.Close()

	var snapshots []string
	out, _, err := doubaoFor(srv.URL).TranslateStream(context.Background(), "hello", "en", "zh-CN",
		func(text string) { snapshots = append(snapshots, text) })
	if err != nil {
		t.Fatalf("TranslateStream: %v", err)
	}
	if out != "你好" {
		t.Errorf("译文 = %q", out)
	}
	if auth != "Bearer key" {
		t.Errorf("Authorization = %q", auth)
	}
	if got["model"] != doubaoDefaultModel {
		t.Errorf("model = %v", got["model"])
	}
	if got["stream"] != true {
		t.Errorf("stream = %v", got["stream"])
	}
	input, _ := got["input"].([]any)
	first, _ := input[0].(map[string]any)
	content, _ := first["content"].([]any)
	block, _ := content[0].(map[string]any)
	if block["type"] != "input_text" || block["text"] != "hello" {
		t.Errorf("content = %v", block)
	}
	options, _ := block["translation_options"].(map[string]any)
	if options["source_language"] != "en" || options["target_language"] != "zh" {
		t.Errorf("translation_options = %v", options)
	}
	if len(snapshots) < 2 || snapshots[0] != "你" || snapshots[len(snapshots)-1] != "你好" {
		t.Errorf("流式快照 = %v", snapshots)
	}
}

// A pair missing from the model's own vocabulary is refused before any HTTP
// request, matching EasyDict's unsupportedLanguage error.
func TestDoubaoRejectsUnsupportedPair(t *testing.T) {
	hit := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
	}))
	defer srv.Close()

	_, _, err := doubaoFor(srv.URL).Translate(context.Background(), "hello", "en", "xx")
	if err == nil || !strings.Contains(err.Error(), "不支持该语言对") {
		t.Fatalf("err = %v", err)
	}
	if hit {
		t.Error("不支持的语言对不应发出请求")
	}
}

// An inline error event must surface instead of turning into an empty answer.
func TestDoubaoSurfacesStreamError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, `data: {"error":{"message":"quota exceeded"}}`+"\n\n")
	}))
	defer srv.Close()

	_, _, err := doubaoFor(srv.URL).Translate(context.Background(), "hello", "en", "zh-CN")
	if err == nil || !strings.Contains(err.Error(), "quota exceeded") {
		t.Fatalf("err = %v", err)
	}
}
