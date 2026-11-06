package update

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewerOrdersVersions(t *testing.T) {
	cases := []struct {
		current, latest string
		want            bool
	}{
		{"1.0.0", "1.0.1", true},
		{"1.9.0", "1.10.0", true}, // segments are numbers, not text
		{"1.2.3", "1.2.3", false},
		{"v1.2.3", "1.2.4", true}, // a tag prefix is spelling
		{"1.2", "1.2.0", false},   // a missing segment is zero
		{"1.2.0", "1.2", false},
		{"1.3.0", "1.2.9", false},
		{"2.0.0", "1.99.99", false},
		{"1.2.3-rc1", "1.2.3", true}, // the published build comes after its rc
		{"1.2.3", "1.2.3-rc1", false},
		{"1.2.3-rc1", "1.2.3-rc2", true},
		{"1.2.3+build9", "1.2.3", false}, // 构建号是元数据，不代表更新
		{"dev", "1.0.0", true},           // a build nobody tagged hears about every release
	}
	for _, c := range cases {
		if got := Newer(c.current, c.latest); got != c.want {
			t.Errorf("Newer(%q, %q) = %v, want %v", c.current, c.latest, got, c.want)
		}
	}
}

// serve answers every request with one body, which is enough: the package reads
// a document, it does not follow links.
func serve(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestFetchReadsAGitHubRelease(t *testing.T) {
	srv := serve(t, http.StatusOK, `{"tag_name":"v1.4.0","html_url":"https://example.test/releases/v1.4.0","body":"修好了截图"}`)
	rel, err := Fetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if rel.Version != "v1.4.0" || rel.URL != "https://example.test/releases/v1.4.0" || rel.Notes != "修好了截图" {
		t.Errorf("读到的发布 = %+v", rel)
	}
}

func TestFetchPrefersTheFlatShape(t *testing.T) {
	srv := serve(t, http.StatusOK, `{"version":"1.4.0","url":"https://example.test/d.pkg","notes":"flat"}`)
	rel, err := Fetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if rel.Version != "1.4.0" || rel.URL != "https://example.test/d.pkg" || rel.Notes != "flat" {
		t.Errorf("读到的发布 = %+v", rel)
	}
}

func TestFetchRefusesWhatIsNotARelease(t *testing.T) {
	cases := []struct {
		name, body string
		want       string
	}{
		{"没有版本号", `{}`, "发布文件没有版本号"},
		{"不是 JSON", `<html>登录</html>`, "更新源返回的不是发布文件"},
	}
	for _, c := range cases {
		srv := serve(t, http.StatusOK, c.body)
		if _, err := Fetch(context.Background(), srv.URL); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want 包含 %q", c.name, err, c.want)
		}
	}
	if _, err := Fetch(context.Background(), ""); err == nil || !strings.Contains(err.Error(), "未设置更新源") {
		t.Errorf("空地址: err = %v", err)
	}
}

func TestFetchReportsAnUnreachableSource(t *testing.T) {
	srv := serve(t, http.StatusNotFound, `{"tag_name":"1.4.0"}`)
	if _, err := Fetch(context.Background(), srv.URL); err == nil || !strings.Contains(err.Error(), "404") {
		t.Errorf("err = %v, want 带上状态码", err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Fetch(cancelled, "https://example.test/releases.json"); err == nil {
		t.Error("取消的请求不应该成功")
	}
}

func TestCheckStaysQuietWhenTheBuildIsCurrent(t *testing.T) {
	srv := serve(t, http.StatusOK, `{"tag_name":"1.4.0","html_url":"https://example.test/r"}`)
	current, err := Check(context.Background(), srv.URL, "1.4.0")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if current != nil {
		t.Errorf("同版本不应该报出新发布: %+v", current)
	}
	ahead, err := Check(context.Background(), srv.URL, "1.5.0")
	if err != nil || ahead != nil {
		t.Errorf("更新的本地版本不该报出新发布: %+v %v", ahead, err)
	}
	fresh, err := Check(context.Background(), srv.URL, "1.3.0")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if fresh == nil || fresh.Version != "1.4.0" {
		t.Errorf("落后的版本应该报出 1.4.0, got %+v", fresh)
	}
}
