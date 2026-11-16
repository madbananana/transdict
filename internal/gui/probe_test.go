package gui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"transdict/internal/settings"
)

// chatServer answers like an OpenAI-compatible endpoint. The probe has to go
// through the real provider code, so the wire format matters here.
func chatServer(t *testing.T, content string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"` + content + `"}}]}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func probeService(t *testing.T, p settings.Provider) *SettingsService {
	t.Helper()
	store := testStore(t)
	if _, err := store.Update(func(c *settings.Config) {
		c.Translate = []settings.Provider{p}
	}); err != nil {
		t.Fatalf("写入服务: %v", err)
	}
	return NewSettings(store, "test")
}

// 验证 runs a real query: a reachable endpoint answers with the translation the
// UI shows beside the button.
func TestTestProviderReturnsTheTranslation(t *testing.T) {
	srv := chatServer(t, "你好")
	svc := probeService(t, settings.Provider{
		ID: "tr-probe", Category: settings.CatTranslate, Kind: "openai",
		Label: "探针", Enabled: true, BaseURL: srv.URL, APIKey: "k", Model: "m",
	})

	got, err := svc.TestProvider("tr-probe")
	if err != nil {
		t.Fatalf("TestProvider: %v", err)
	}
	if got != "你好" {
		t.Errorf("验证结果 = %q, want 你好", got)
	}
}

// An endpoint that answers with nothing is a failure the user has to see; the
// button must not report success for an empty string.
func TestTestProviderRejectsAnEmptyAnswer(t *testing.T) {
	srv := chatServer(t, "")
	svc := probeService(t, settings.Provider{
		ID: "tr-probe", Category: settings.CatTranslate, Kind: "openai",
		Label: "探针", Enabled: true, BaseURL: srv.URL, APIKey: "k", Model: "m",
	})

	got, err := svc.TestProvider("tr-probe")
	if err != nil {
		t.Fatalf("TestProvider: %v", err)
	}
	if got != "服务返回了空结果" {
		t.Errorf("验证结果 = %q, want 空结果提示", got)
	}
}

func TestTestProviderCarriesTheProvidersError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad key", http.StatusUnauthorized)
	}))
	t.Cleanup(srv.Close)
	svc := probeService(t, settings.Provider{
		ID: "tr-probe", Category: settings.CatTranslate, Kind: "openai",
		Label: "探针", Enabled: true, BaseURL: srv.URL, APIKey: "k", Model: "m",
	})

	if _, err := svc.TestProvider("tr-probe"); err == nil {
		t.Fatal("401 被当成了验证成功")
	}
}

func TestTestProviderRefusesUnknownAndOCR(t *testing.T) {
	store := testStore(t)
	svc := NewSettings(store, "test")

	if _, err := svc.TestProvider("nope"); err == nil || !strings.Contains(err.Error(), "找不到") {
		t.Errorf("未知 id 的错误 = %v, want 找不到这个服务", err)
	}

	id := store.Get().OCR[0].ID
	if _, err := svc.TestProvider(id); err == nil || !strings.Contains(err.Error(), "截图") {
		t.Errorf("OCR 验证的错误 = %v, want 需要截图的提示", err)
	}
}

func TestProbeTargetAvoidsEnglishIntoEnglish(t *testing.T) {
	if got := probeTarget("en"); got != "zh-CN" {
		t.Errorf("probeTarget(en) = %q, want zh-CN", got)
	}
	if got := probeTarget("ja"); got != "ja" {
		t.Errorf("probeTarget(ja) = %q, want ja", got)
	}
}

func TestClipProbeCollapsesAndShortens(t *testing.T) {
	if got := clipProbe("  hello\n world  "); got != "hello world" {
		t.Errorf("clipProbe = %q, want hello world", got)
	}
	long := strings.Repeat("好", 80)
	if got := clipProbe(long); len([]rune(got)) != 61 {
		t.Errorf("截断后的长度 = %d, want 61（60 字加省略号）", len([]rune(got)))
	}
}
