package gui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func serveRelease(t *testing.T, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestUpdateReportNamesBothSides(t *testing.T) {
	long := strings.Repeat("长", notesLimit+50)
	srv := serveRelease(t, `{"tag_name":"1.2.0","html_url":"https://example.test/r","body":"`+long+`"}`)

	s := &Service{version: "1.1.0"}
	report, _, err := s.fetchReport(srv.URL, false)
	if err != nil {
		t.Fatalf("updateReport: %v", err)
	}
	if report.Current != "1.1.0" || report.Version != "1.2.0" || report.URL != "https://example.test/r" {
		t.Errorf("报告 = %+v", report)
	}
	if !report.Newer() {
		t.Error("1.2.0 相对 1.1.0 应该算新版本")
	}
	// The window scrolls a long changelog; this cut is only a guard against a
	// pathological body travelling with every progress event.
	if got := len([]rune(report.Notes)); got != notesLimit+1 {
		t.Errorf("说明 %d 字, want %d（含省略号）", got, notesLimit+1)
	}
}

func TestUpdateReportStaysQuietWhenTheBuildIsCurrent(t *testing.T) {
	srv := serveRelease(t, `{"tag_name":"1.0.0","html_url":"https://example.test/r"}`)
	report, _, err := (&Service{version: "1.0.0"}).fetchReport(srv.URL, false)
	if err != nil {
		t.Fatalf("updateReport: %v", err)
	}
	if report.Version != "" {
		t.Errorf("同版本不应该报出新版本: %+v", report)
	}
	if report.Current != "1.0.0" {
		t.Errorf("当前版本 = %q, want 1.0.0 —— 手动检查要能说出这一句", report.Current)
	}
}

func TestUpdateReportRefusesAnEmptySource(t *testing.T) {
	if _, _, err := (&Service{version: "1.0.0"}).fetchReport("", false); err == nil ||
		!strings.Contains(err.Error(), "未设置更新源") {
		t.Errorf("err = %v, want 说明没有更新源", err)
	}
}

func TestUpdateReportCarriesIncludeBeta(t *testing.T) {
	srv := serveRelease(t, `[
		{"tag_name":"v1.1.0-beta.1","html_url":"https://example.test/beta","prerelease":true},
		{"tag_name":"v1.0.0","html_url":"https://example.test/stable"}
	]`)
	s := &Service{version: "1.0.0"}
	quiet, _, err := s.fetchReport(srv.URL, false)
	if err != nil {
		t.Fatalf("updateReport: %v", err)
	}
	if quiet.Version != "" {
		t.Errorf("没有打开包括 Beta 时不该报出预发布版本: %+v", quiet)
	}
	beta, _, err := s.fetchReport(srv.URL, true)
	if err != nil {
		t.Fatalf("updateReport: %v", err)
	}
	if beta.Version != "v1.1.0-beta.1" || beta.URL != "https://example.test/beta" {
		t.Errorf("打开包括 Beta 后应该报出预发布版本: %+v", beta)
	}
}

func TestUpdateSourceFallsBackToTheOfficialFeed(t *testing.T) {
	if got := updateSource(""); got == "" || !strings.Contains(got, "madbananana/transdict") {
		t.Errorf("空地址应指向官方发布源, got %q", got)
	}
	if got := updateSource("  https://example.test/r.json "); got != "https://example.test/r.json" {
		t.Errorf("自定义地址应原样保留, got %q", got)
	}
}

func TestUpdateInfoReportsTheDownloadAndTheRelease(t *testing.T) {
	s := &Service{
		version:     "1.0.0",
		notice:      &updateNotice{version: "1.1.0", url: "https://example.test/r", notes: "notes"},
		updateState: updateDownloading,
		updateDone:  55,
		updateTotal: 100,
	}
	info := s.updateInfoLocked()
	if info.Percent != 55 || info.Received != 55 || info.Total != 100 {
		t.Errorf("进度 = %+v, want 55/100", info)
	}
	if info.Current != "1.0.0" || info.Version != "1.1.0" || info.State != updateDownloading {
		t.Errorf("版本或状态 = %+v", info)
	}

	// A server that under-reports its length must not draw a bar past the track.
	s.updateDone = 120
	if got := s.updateInfoLocked().Percent; got != 100 {
		t.Errorf("超出总长的百分比 = %d, want 100", got)
	}
}

func TestUpdateInfoIsIdleWithoutANotice(t *testing.T) {
	info := (&Service{version: "1.0.0"}).updateInfoLocked()
	if info.State != updateIdle || info.Version != "" || info.Percent != 0 {
		t.Errorf("没有待安装版本时应为空闲状态: %+v", info)
	}
	if info.Current != "1.0.0" {
		t.Errorf("当前版本 = %q, want 1.0.0", info.Current)
	}
}
