package gui

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"transdict/internal/dict"
	"transdict/internal/l10n"
	"transdict/internal/query"
	"transdict/internal/settings"
)

// notebookWriteFailed is EasyDict's vocabulary_notebook.write_failed_message.
// Logging alone hid a wrong folder from the user, who then kept querying into
// nothing.
const notebookWriteFailed = "生词本写入失败，请重新选择目录。"

// notebookFile is the 生词本 itself: one JSON object per line, appended as the
// user queries. The name and the row shape are EasyDict's, because the file is
// written for an external review program that reads `words.jsonl` and expects
// its fields.
const notebookFile = "words.jsonl"

// notebookEntry is EasyDict's VocabularyEntry JSON. translatedText is omitted
// for sentence queries without a translation; dictionaryEntry is omitted when
// the query carried no word result.
type notebookEntry struct {
	Text            string           `json:"text"`
	SourceLanguage  string           `json:"sourceLanguage"`
	TargetLanguage  string           `json:"targetLanguage"`
	TranslatedText  string           `json:"translatedText,omitempty"`
	Timestamp       string           `json:"timestamp"`
	DictionaryEntry *dictionaryEntry `json:"dictionaryEntry,omitempty"`
}

// notebookTime is EasyDict's `yyyy-MM-dd'T'HH:mm:ssXXX`: local time with the
// numeric offset, not an epoch number.
const notebookTime = "2006-01-02T15:04:05-07:00"

// saveNotebook appends a finished answer to the 生词本. A retry answers under
// the same response id, so the id is claimed once: retrying one service must
// not turn one query into two notebook lines.
func (s *Service) saveNotebook(res *query.Response) {
	cfg := s.cfg.Get()
	entry, ok := notebookRecord(cfg, res)
	if !ok || !s.claimNotebook(res.ID) {
		return
	}
	line, err := json.Marshal(entry)
	if err != nil {
		log.Printf("transdict: 写入生词本失败: %v", err)
		s.Toast(l10n.T(notebookWriteFailed))
		return
	}
	path := filepath.Join(cfg.VocabularyDir, notebookFile)
	// The folder picker only returns folders that exist, but the field also
	// accepts a typed path, and failing every query because a directory is
	// missing is worse than creating it once.
	if err := os.MkdirAll(cfg.VocabularyDir, 0o755); err != nil {
		log.Printf("transdict: 写入生词本失败: %v", err)
		s.Toast(l10n.T(notebookWriteFailed))
		return
	}
	if err := appendNotebookLine(path, line); err != nil {
		log.Printf("transdict: 写入生词本失败: %v", err)
		s.Toast(l10n.T(notebookWriteFailed))
	}
}

// appendNotebookLine writes one JSONL row. A file that another tool left
// without a trailing newline gets one first, so the new record keeps its own
// line instead of running into the previous one.
func appendNotebookLine(path string, line []byte) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	buf := append(append([]byte(nil), line...), '\n')
	if st, err := f.Stat(); err == nil && st.Size() > 0 {
		last := make([]byte, 1)
		if _, err := f.ReadAt(last, st.Size()-1); err == nil && last[0] != '\n' {
			buf = append([]byte{'\n'}, buf...)
		}
	}
	_, err = f.Write(buf)
	return err
}

// notebookRecord is the row a finished answer contributes, or ok=false when
// there is nothing to save: the switch is off, no directory was chosen, the
// response is a progress update, the query itself failed, or no service
// answered with any content. EasyDict only records answers that carry a word
// result, a translation or a dictionary block.
func notebookRecord(cfg settings.Config, res *query.Response) (notebookEntry, bool) {
	dir := strings.TrimSpace(cfg.VocabularyDir)
	if !cfg.VocabularyNotebook || dir == "" || res == nil || res.Partial || res.Error != "" {
		return notebookEntry{}, false
	}
	text := strings.TrimSpace(res.SourceText)
	if text == "" {
		return notebookEntry{}, false
	}
	translated := query.FirstTranslation(res)
	dictEntry := dictionaryEntryFrom(res.Dict)
	if translated == "" && dictEntry == nil && !hasDictContent(res.Dict) {
		return notebookEntry{}, false
	}
	source := res.SourceLang
	if source == "" || source == "auto" {
		source = res.Detected
	}
	if source == "auto" {
		source = ""
	}
	return notebookEntry{
		Text:            text,
		SourceLanguage:  source,
		TargetLanguage:  res.TargetLang,
		TranslatedText:  translated,
		Timestamp:       time.Now().Format(notebookTime),
		DictionaryEntry: dictEntry,
	}, true
}

// hasDictContent counts a dictionary answer that the JSON schema cannot carry
// — Apple Dictionary's HTML block has no phonetics or parts — as content, so
// the query is still recorded.
func hasDictContent(results []dict.Result) bool {
	for _, r := range results {
		if r.Error == "" && len(r.Entries) > 0 {
			return true
		}
	}
	return false
}

func (s *Service) claimNotebook(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.notebook == nil {
		s.notebook = make(map[string]bool)
	}
	if s.notebook[id] {
		return false
	}
	s.notebook[id] = true
	return true
}
