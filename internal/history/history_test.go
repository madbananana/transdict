package history

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	return &Store{path: filepath.Join(t.TempDir(), "history.json")}
}

func entry(id, source string, at time.Time) Entry {
	return Entry{ID: id, Source: source, CreatedAt: at.Unix()}
}

func TestPutIgnoresBlankSource(t *testing.T) {
	s := newTestStore(t)
	if err := s.Put(entry("1", "   ", time.Now())); err != nil {
		t.Fatal(err)
	}
	if got := s.List(Filter{}, 0); len(got) != 0 {
		t.Errorf("空原文不该入库，得到 %d 条", len(got))
	}
}

// A retry reuses the same id: the row has to update in place, and starring it
// first must not be undone by translating it again.
func TestPutReplacesByIDAndKeepsFavorite(t *testing.T) {
	s := newTestStore(t)
	now := time.Now()
	if err := s.Put(entry("a", "第一次", now)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Star("a"); err != nil {
		t.Fatal(err)
	}
	again := entry("a", "第二次", now.Add(time.Minute))
	again.Target = "again"
	if err := s.Put(again); err != nil {
		t.Fatal(err)
	}

	got := s.List(Filter{}, 0)
	if len(got) != 1 {
		t.Fatalf("重试应覆盖原记录，得到 %d 条", len(got))
	}
	if got[0].Source != "第二次" || got[0].Target != "again" {
		t.Errorf("记录未更新: %+v", got[0])
	}
	if !got[0].Favorite {
		t.Error("重试不应取消收藏")
	}
}

func TestListOrdersNewestFirstAndFilters(t *testing.T) {
	s := newTestStore(t)
	base := time.Now()
	for i, src := range []string{"apple pie", "Banana", "cherry"} {
		e := entry(fmt.Sprint(i), src, base.Add(time.Duration(i)*time.Hour))
		if i == 0 {
			e.Target = "苹果"
			e.Services = []string{"Google"}
		}
		if err := s.Put(e); err != nil {
			t.Fatal(err)
		}
	}

	if got := s.List(Filter{}, 0); len(got) != 3 || got[0].Source != "cherry" {
		t.Errorf("默认应倒序，得到 %v", got)
	}
	// Needle is case-folded and reaches the translation and the service names,
	// not only the source text.
	for _, needle := range []string{"BANANA", "苹果", "google"} {
		if got := s.List(Filter{Needle: needle}, 0); len(got) != 1 {
			t.Errorf("搜索 %q 命中 %d 条, 期望 1", needle, len(got))
		}
	}
	if got := s.List(Filter{Favorites: true}, 0); len(got) != 0 {
		t.Errorf("未收藏时只看收藏应为空，得到 %d", len(got))
	}
	if got := s.List(Filter{}, 2); len(got) != 2 {
		t.Errorf("limit 未生效，得到 %d", len(got))
	}
}

func TestStarAndDelete(t *testing.T) {
	s := newTestStore(t)
	if err := s.Put(entry("a", "x", time.Now())); err != nil {
		t.Fatal(err)
	}
	on, err := s.Star("a")
	if err != nil || !on {
		t.Fatalf("Star = %v, %v", on, err)
	}
	if on, err := s.Star("a"); err != nil || on {
		t.Errorf("再次 Star 应取消收藏，得到 %v, %v", on, err)
	}
	if _, err := s.Star("missing"); err == nil {
		t.Error("收藏不存在的记录应当报错")
	}
	if err := s.Delete("a"); err != nil {
		t.Fatal(err)
	}
	if got := s.List(Filter{}, 0); len(got) != 0 {
		t.Errorf("删除后仍有 %d 条", len(got))
	}
}

func TestClearKeepsFavorites(t *testing.T) {
	s := newTestStore(t)
	old := time.Now().AddDate(0, 0, -40)
	a, b, c := entry("a", "旧的", old), entry("b", "也旧的", old), entry("c", "新的", time.Now())
	c.Favorite = true
	for _, e := range []Entry{a, b, c} {
		if err := s.Put(e); err != nil {
			t.Fatal(err)
		}
	}

	// days <= 0 is what the settings button advertises: everything except the
	// rows the user chose to keep.
	dropped, err := s.Clear(0)
	if err != nil {
		t.Fatal(err)
	}
	if dropped != 2 {
		t.Errorf("Clear(0) 删除 %d 条, 期望 2", dropped)
	}
	if got := s.List(Filter{}, 0); len(got) != 1 || got[0].ID != "c" {
		t.Errorf("Clear(0) 后剩下 %v", got)
	}
}

func TestClearByAgeKeepsRecentAndFavorites(t *testing.T) {
	s := newTestStore(t)
	now := time.Now()
	old := entry("old", "很久以前", now.AddDate(0, 0, -30))
	recent := entry("recent", "昨天", now.Add(-24*time.Hour))
	starred := entry("starred", "很久以前但收藏了", now.AddDate(0, 0, -90))
	starred.Favorite = true
	for _, e := range []Entry{old, recent, starred} {
		if err := s.Put(e); err != nil {
			t.Fatal(err)
		}
	}
	if dropped, err := s.Clear(7); err != nil || dropped != 1 {
		t.Fatalf("Clear(7) = %d, %v; 期望只删掉过期且未收藏的一条", dropped, err)
	}
	if got := s.List(Filter{}, 0); len(got) != 2 {
		t.Errorf("清理后剩 %d 条: %v", len(got), got)
	}
}

// The file is the user's data: writing it atomically and reading it back
// unchanged is the whole contract.
func TestStoreRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")
	s := &Store{path: path}
	e := entry("a", "原文", time.Now())
	e.Target = "译文"
	e.Services = []string{"Google", "有道"}
	e.Context = "main"
	if err := s.Put(e); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path + ".tmp"); err == nil {
		t.Error("临时文件没有被重命名，写入不是原子的")
	}

	reopened := &Store{path: path}
	if err := reopened.load(); err != nil {
		t.Fatalf("重新读取失败: %v", err)
	}
	got := reopened.List(Filter{}, 0)
	if len(got) != 1 || got[0].Services[1] != "有道" || got[0].Target != "译文" {
		t.Fatalf("读回的内容不一致: %+v", got)
	}
}

func TestLoadToleratesNoFile(t *testing.T) {
	s := &Store{path: filepath.Join(t.TempDir(), "missing.json")}
	if err := s.load(); err != nil {
		t.Errorf("首次启动没有历史文件，应视为空而非错误: %v", err)
	}
}

func TestCapDropsOldest(t *testing.T) {
	s := newTestStore(t)
	s.entries = make([]Entry, MaxEntries)
	base := time.Now().Add(-time.Duration(MaxEntries) * time.Minute)
	for i := range s.entries {
		s.entries[i] = entry(fmt.Sprint(i), fmt.Sprintf("第 %d 条", i), base.Add(time.Duration(i)*time.Minute))
	}
	if err := s.Put(entry("new", "最新", time.Now())); err != nil {
		t.Fatal(err)
	}
	if len(s.entries) != MaxEntries {
		t.Fatalf("超出上限后仍有 %d 条", len(s.entries))
	}
	if s.entries[0].ID != "1" {
		t.Errorf("应删掉最旧的一条，当前首条是 %q", s.entries[0].ID)
	}
	if got := s.List(Filter{Needle: "最新"}, 0); len(got) != 1 {
		t.Error("新记录必须保留在窗口内")
	}
}

func TestExportCoversEveryMatchingRow(t *testing.T) {
	s := newTestStore(t)
	a := entry("a", "第一行\n第二行", time.Date(2026, 3, 4, 5, 6, 7, 0, time.Local))
	a.Target = "译文"
	a.TargetLang = "zh-CN"
	a.Services = []string{"Google", "有道"}
	b := entry("b", "apple", time.Now())
	b.Favorite = true
	for _, e := range []Entry{a, b} {
		if err := s.Put(e); err != nil {
			t.Fatal(err)
		}
	}

	md := s.Export(Filter{}, FormatMarkdown)
	for _, want := range []string{"# Transdict 记录", "2026-03-04 05:06:07 · zh-CN",
		"> 第一行\n> 第二行", "译文", "服务：Google、有道"} {
		if !strings.Contains(md, want) {
			t.Errorf("导出缺少 %q:\n%s", want, md)
		}
	}
	if only := s.Export(Filter{Favorites: true}, FormatMarkdown); !strings.Contains(only, "apple") || strings.Contains(only, "第一行") {
		t.Errorf("只看收藏的导出不正确:\n%s", only)
	}
	if searched := s.Export(Filter{Needle: "apple"}, FormatMarkdown); !strings.Contains(searched, "apple") || strings.Contains(searched, "译文") {
		t.Errorf("按关键词导出不正确:\n%s", searched)
	}
}

func serviceLog(t *testing.T) *Store {
	t.Helper()
	s := newTestStore(t)
	now := time.Now()
	a := entry("a", "hello, world\n你好", now)
	a.Target = `引号 " 与逗号`
	a.TargetLang = "zh-CN"
	a.Services = []string{"Google", "有道"}
	b := entry("b", "bonjour", now.Add(time.Minute))
	b.Services = []string{"有道"}
	b.Favorite = true
	for _, e := range []Entry{a, b} {
		if err := s.Put(e); err != nil {
			t.Fatal(err)
		}
	}
	return s
}

// The dropdown filters by the exact name a row was recorded with, so a service
// that only appears inside the searched text must not slip through.
func TestFilterByService(t *testing.T) {
	s := serviceLog(t)
	if got := s.List(Filter{Service: "有道"}, 0); len(got) != 2 {
		t.Errorf("有道命中 %d 条, 期望 2", len(got))
	}
	if got := s.List(Filter{Service: "Google"}, 0); len(got) != 1 || got[0].ID != "a" {
		t.Errorf("Google 过滤结果错误: %v", got)
	}
	// "goo" is inside a service name, so it searches but is not one.
	if got := s.List(Filter{Service: "goo"}, 0); len(got) != 0 {
		t.Errorf("精确过滤不该放过前缀，得到 %d 条", len(got))
	}
	if got := s.List(Filter{Service: "Google", Favorites: true}, 0); len(got) != 0 {
		t.Errorf("两个条件应叠加，得到 %d 条", len(got))
	}
}

func TestServicesCountsMostUsedFirst(t *testing.T) {
	got := serviceLog(t).Services()
	if len(got) != 2 {
		t.Fatalf("得到 %v", got)
	}
	if got[0].Name != "有道" || got[0].Entries != 2 {
		t.Errorf("首位应是出现两次的有道，得到 %+v", got[0])
	}
	if got[1].Name != "Google" || got[1].Entries != 1 {
		t.Errorf("次位应是 Google，得到 %+v", got[1])
	}
	// The store hands back what was recorded; the interface language is applied
	// by the caller, so Label stays empty here.
	if got[0].Label != "" {
		t.Errorf("Label 应由调用方填写，得到 %q", got[0].Label)
	}
}

// A translation log is full of commas, quotes and line breaks. Left raw they
// re-shuffle the rows when a spreadsheet opens the file, so the export has to
// survive a real CSV reader.
func TestExportCSVIsReadableByASpreadsheet(t *testing.T) {
	s := serviceLog(t)
	csvText := s.Export(Filter{}, FormatCSV)
	// Excel guesses the codepage from the bytes when nothing marks the file, and
	// a log of Chinese translations arrives as mojibake.
	if !strings.HasPrefix(csvText, "\ufeff") {
		t.Error("缺少 BOM")
	}
	r := csv.NewReader(strings.NewReader(strings.TrimPrefix(csvText, "\ufeff")))
	r.FieldsPerRecord = 5
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("CSV 解析失败: %v\n%s", err, csvText)
	}
	// Header plus both rows: the cell with a newline inside stays one record.
	if len(records) != 3 {
		t.Fatalf("得到 %d 条记录, 期望 3:\n%q", len(records), csvText)
	}
	want := []string{"时间", "原文", "译文", "目标语言", "服务"}
	for i, head := range want {
		if records[0][i] != head {
			t.Errorf("表头第 %d 列是 %q, 期望 %q", i, records[0][i], head)
		}
	}
	// Newest first, so records[1] is the bonjour row.
	if records[1][1] != "bonjour" || records[1][4] != "有道" {
		t.Errorf("第二行内容错误: %v", records[1])
	}
	if records[2][1] != "hello, world\n你好" {
		t.Errorf("含逗号与换行的原文往返错误: %q", records[2][1])
	}
	if records[2][2] != `引号 " 与逗号` || records[2][3] != "zh-CN" || records[2][4] != "Google、有道" {
		t.Errorf("第一行内容错误: %v", records[2])
	}
}

func TestExportJSONKeepsEveryField(t *testing.T) {
	data := serviceLog(t).Export(Filter{Service: "Google"}, FormatJSON)
	var back []Entry
	if err := json.Unmarshal([]byte(data), &back); err != nil {
		t.Fatalf("导出的 JSON 读不回来: %v\n%s", err, data)
	}
	if len(back) != 1 || back[0].ID != "a" || len(back[0].Services) != 2 {
		t.Errorf("往返后内容不一致: %+v", back)
	}
	empty := &Store{path: filepath.Join(t.TempDir(), "h.json")}
	if got := empty.Export(Filter{}, FormatJSON); strings.TrimSpace(got) != "[]" {
		t.Errorf("空记录的 JSON 导出应是空数组，得到 %q", got)
	}
}
