package gui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"transdict/internal/dict"
	"transdict/internal/query"
	"transdict/internal/settings"
	"transdict/internal/translate"
)

// 生词本 is only written when it is on, a directory was chosen, and the
// response is a finished answer with text of its own.
func TestNotebookRecordNeedsSwitchDirectoryAndAnswer(t *testing.T) {
	answer := &query.Response{ID: "q1", SourceText: "hello", Context: settings.ContextClipboard,
		TargetLang: "zh-CN", Mode: "selection",
		Translate: []translate.Result{{Label: "Google", Text: "你好"}}}
	on := settings.Config{VocabularyNotebook: true, VocabularyDir: "/tmp/words"}
	if _, ok := notebookRecord(on, answer); !ok {
		t.Fatal("开启且选了目录时应该记录")
	}
	if _, ok := notebookRecord(settings.Config{VocabularyDir: "/tmp/words"}, answer); ok {
		t.Error("生词本关闭时不应记录")
	}
	if _, ok := notebookRecord(settings.Config{VocabularyNotebook: true, VocabularyDir: "  "}, answer); ok {
		t.Error("没有目录时不应记录")
	}
	if _, ok := notebookRecord(on, &query.Response{ID: "q2", Partial: true, SourceText: "hel"}); ok {
		t.Error("进度更新不应记录")
	}
	if _, ok := notebookRecord(on, &query.Response{ID: "q3", SourceText: "  "}); ok {
		t.Error("没有文本时不应记录")
	}
	if _, ok := notebookRecord(on, &query.Response{ID: "q4", SourceText: "hi", Error: "查询失败"}); ok {
		t.Error("查询本身失败时不应记录")
	}
	if _, ok := notebookRecord(on, &query.Response{ID: "q5", SourceText: "hi",
		Translate: []translate.Result{{Label: "Google", Error: "超时"}}}); ok {
		t.Error("所有服务都没给出内容时不应记录")
	}
}

func TestNotebookRecordCarriesTheAnswer(t *testing.T) {
	entry, ok := notebookRecord(settings.Config{VocabularyNotebook: true, VocabularyDir: "/tmp/words"},
		&query.Response{ID: "q1", SourceText: "hello", Detected: "en", TargetLang: "zh-CN",
			Context: settings.ContextClipboard, Mode: "selection",
			Translate: []translate.Result{{Label: "Google", Text: "你好"}, {Label: "Bing", Error: "超时"}}})
	if !ok {
		t.Fatal("应该记录")
	}
	if entry.Text != "hello" || entry.TranslatedText != "你好" || entry.SourceLanguage != "en" ||
		entry.TargetLanguage != "zh-CN" {
		t.Errorf("entry = %+v", entry)
	}
	ts, err := time.Parse(time.RFC3339, entry.Timestamp)
	if err != nil {
		t.Errorf("timestamp %q 不是 ISO 8601: %v", entry.Timestamp, err)
	} else if ts.Format(notebookTime) != entry.Timestamp {
		t.Errorf("timestamp %q 应带本地偏移", entry.Timestamp)
	}
	if entry.DictionaryEntry != nil {
		t.Errorf("没有词典结果时 dictionaryEntry 应为空, got %+v", entry.DictionaryEntry)
	}
}

func TestNotebookRecordCarriesTheDictionaryEntry(t *testing.T) {
	entry, ok := notebookRecord(settings.Config{VocabularyNotebook: true, VocabularyDir: "/tmp/words"},
		&query.Response{ID: "q1", SourceText: "hello", Detected: "en", TargetLang: "zh-CN",
			Dict: []dict.Result{{Label: "有道词典", Entries: []dict.Entry{{
				Word:        "hello",
				Phonetics:   []dict.Phonetic{{Lang: "en", Symbol: "həˈləʊ"}},
				Definitions: []dict.Definition{{Pos: "int.", Meanings: []string{"你好"}}},
			}}}}})
	if !ok {
		t.Fatal("只有词典结果也应该记录")
	}
	if entry.DictionaryEntry == nil {
		t.Fatal("缺少 dictionaryEntry")
	}
	if got := entry.DictionaryEntry.Parts; len(got) != 1 || got[0].Means[0] != "你好" {
		t.Errorf("parts = %+v", got)
	}
	if got := entry.DictionaryEntry.Phonetics; len(got) != 1 || got[0].Value != "həˈləʊ" {
		t.Errorf("phonetics = %+v", got)
	}
}

func TestSaveNotebookAppendsOneLinePerResponse(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "not-yet-there")
	store := testStore(t)
	if _, err := store.Update(func(c *settings.Config) {
		c.VocabularyNotebook = true
		c.VocabularyDir = dir
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	s := &Service{cfg: store}
	res := &query.Response{ID: "q1", SourceText: "hello", TargetLang: "zh-CN",
		Translate: []translate.Result{{Label: "Google", Text: "你好"}}}
	s.saveNotebook(res)
	s.saveNotebook(res) // 同一响应的重试不应写第二行

	path := filepath.Join(dir, notebookFile)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if got := lineCount(data); got != 1 {
		t.Fatalf("生词本行数 = %d, 期望 1\n%s", got, data)
	}
	var entry map[string]any
	if err := json.Unmarshal(firstLine(data), &entry); err != nil {
		t.Fatalf("JSONL 解析失败: %v", err)
	}
	if entry["text"] != "hello" || entry["translatedText"] != "你好" ||
		entry["targetLanguage"] != "zh-CN" {
		t.Errorf("entry = %v", entry)
	}
	if _, ok := entry["timestamp"].(string); !ok {
		t.Errorf("timestamp 应是字符串: %v", entry["timestamp"])
	}
	// The row is EasyDict's VocabularyEntry schema, not this app's own fields.
	for _, bad := range []string{"time", "translation", "sourceLang", "targetLang", "services"} {
		if _, ok := entry[bad]; ok {
			t.Errorf("不应有非 EasyDict 字段 %q: %v", bad, entry)
		}
	}

	next := &query.Response{ID: "q2", SourceText: "world", TargetLang: "zh-CN",
		Translate: []translate.Result{{Label: "Google", Text: "世界"}}}
	s.saveNotebook(next)
	data, _ = os.ReadFile(path)
	if got := lineCount(data); got != 2 {
		t.Errorf("生词本行数 = %d, 期望 2", got)
	}
}

func TestAppendNotebookLineSeparatesFromMissingNewline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "words.jsonl")
	if err := os.WriteFile(path, []byte(`{"text":"old"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := appendNotebookLine(path, []byte(`{"text":"new"}`)); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := lineCount(data); got != 2 {
		t.Errorf("行数 = %d, 期望 2\n%s", got, data)
	}
}

func TestSaveNotebookDoesNothingWhenOff(t *testing.T) {
	dir := t.TempDir()
	store := testStore(t)
	if _, err := store.Update(func(c *settings.Config) { c.VocabularyDir = dir }); err != nil {
		t.Fatalf("Update: %v", err)
	}
	s := &Service{cfg: store}
	s.saveNotebook(&query.Response{ID: "q1", SourceText: "hello"})
	if _, err := os.Stat(filepath.Join(dir, notebookFile)); !os.IsNotExist(err) {
		t.Error("生词本关闭时不应创建文件")
	}
}

func lineCount(data []byte) int {
	n := 0
	for _, b := range data {
		if b == '\n' {
			n++
		}
	}
	return n
}

func firstLine(data []byte) []byte {
	for i, b := range data {
		if b == '\n' {
			return data[:i]
		}
	}
	return data
}
