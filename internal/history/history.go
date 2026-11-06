// Package history stores past queries. A single JSON file with a bounded
// window keeps the dependency list empty and the store readable by the user,
// which matters more here than the query power a SQLite index would add: the
// list is searched in memory and even 5 years of daily use stays small.
package history

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"transdict/internal/appdir"
	"transdict/internal/l10n"
)

// MaxEntries bounds the file. Easydict caps its table too; older rows are
// dropped oldest-first so recency always survives.
const MaxEntries = 5000

// Export formats. Anything else falls back to Markdown, which is the shape a
// translation log is most useful in.
const (
	FormatMarkdown = "markdown"
	FormatJSON     = "json"
	FormatCSV      = "csv"
)

// Filter narrows 历史. Needle matches loosely — source, answer, or any service
// name — while Service is the exact one a dropdown offered.
type Filter struct {
	Needle    string
	Favorites bool
	Service   string
}

type Entry struct {
	ID         string   `json:"id"`
	Kind       string   `json:"kind"`
	Context    string   `json:"context"`
	Source     string   `json:"source"`
	Detected   string   `json:"detected,omitempty"`
	Target     string   `json:"target,omitempty"`
	TargetLang string   `json:"targetLang,omitempty"`
	Services   []string `json:"services,omitempty"`
	Image      string   `json:"image,omitempty"`
	Favorite   bool     `json:"favorite"`
	CreatedAt  int64    `json:"createdAt"`
	ElapsedMs  int64    `json:"elapsedMs,omitempty"`
}

func (e Entry) Time() time.Time { return time.Unix(e.CreatedAt, 0) }

// Store is safe for concurrent use; every mutation persists synchronously so a
// crash cannot lose the query the user just saw.
type Store struct {
	mu      sync.Mutex
	path    string
	entries []Entry
}

func NewStore() (*Store, error) {
	dir, err := appdir.Root()
	if err != nil {
		return nil, err
	}
	s := &Store{path: filepath.Join(dir, "history.json")}
	return s, s.load()
}

func (s *Store) load() error {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &s.entries)
}

func (s *Store) saveLocked() error {
	data, err := json.MarshalIndent(s.entries, "", " ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// Put adds an entry, replacing any earlier one with the same id so a retry
// updates the row instead of duplicating it.
func (s *Store) Put(e Entry) error {
	if strings.TrimSpace(e.Source) == "" {
		return nil
	}
	if e.CreatedAt == 0 {
		e.CreatedAt = time.Now().Unix()
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	idx := -1
	for i := range s.entries {
		if s.entries[i].ID == e.ID && e.ID != "" {
			idx = i
			break
		}
	}
	if idx >= 0 {
		e.Favorite = s.entries[idx].Favorite || e.Favorite
		s.entries[idx] = e
	} else {
		s.entries = append(s.entries, e)
		if len(s.entries) > MaxEntries {
			drop := len(s.entries) - MaxEntries
			s.entries = append([]Entry(nil), s.entries[drop:]...)
		}
	}
	return s.saveLocked()
}

// List returns the newest first.
func (s *Store) List(f Filter, limit int) []Entry {
	s.mu.Lock()
	defer s.mu.Unlock()

	needle := strings.ToLower(strings.TrimSpace(f.Needle))
	service := strings.TrimSpace(f.Service)
	out := make([]Entry, 0, len(s.entries))
	for _, e := range s.entries {
		if f.Favorites && !e.Favorite {
			continue
		}
		if service != "" && !carries(e, service) {
			continue
		}
		if needle != "" && !matches(e, needle) {
			continue
		}
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// Services lists every name the log records, each with how many rows it produced,
// so the dropdown can offer a filter that actually matches something. A service
// that has since been deleted keeps its rows, and its name still filters them.
func (s *Store) Services() []ServiceCount {
	s.mu.Lock()
	defer s.mu.Unlock()

	counts := map[string]int{}
	for _, e := range s.entries {
		for _, name := range e.Services {
			counts[name]++
		}
	}
	out := make([]ServiceCount, 0, len(counts))
	for name, n := range counts {
		out = append(out, ServiceCount{Name: name, Entries: n})
	}
	// Most used first; the name breaks ties so the list is stable across calls.
	sort.Slice(out, func(i, j int) bool {
		if out[i].Entries != out[j].Entries {
			return out[i].Entries > out[j].Entries
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// ServiceCount is one entry of Services(). Name is what the rows hold and what
// a filter has to match; Label is the same words in the interface language, for
// display. Keeping both means an English UI can still filter a log recorded in
// Chinese.
type ServiceCount struct {
	Name    string `json:"name"`
	Label   string `json:"label"`
	Entries int    `json:"entries"`
}

func carries(e Entry, service string) bool {
	for _, name := range e.Services {
		if name == service {
			return true
		}
	}
	return false
}

func matches(e Entry, needle string) bool {
	if strings.Contains(strings.ToLower(e.Source), needle) ||
		strings.Contains(strings.ToLower(e.Target), needle) {
		return true
	}
	for _, svc := range e.Services {
		if strings.Contains(strings.ToLower(svc), needle) {
			return true
		}
	}
	return false
}

// Star flips the favorite flag and returns the new value.
func (s *Store) Star(id string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.entries {
		if s.entries[i].ID != id {
			continue
		}
		s.entries[i].Favorite = !s.entries[i].Favorite
		return s.entries[i].Favorite, s.saveLocked()
	}
	return false, errors.New("记录不存在")
}

func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.entries[:0]
	for _, e := range s.entries {
		if e.ID != id {
			out = append(out, e)
		}
	}
	s.entries = out
	return s.saveLocked()
}

// Clear drops rows older than days; days <= 0 clears everything but favorites,
// which is what the settings button advertises.
func (s *Store) Clear(days int) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	before := len(s.entries)
	if days <= 0 {
		out := s.entries[:0]
		for _, e := range s.entries {
			if e.Favorite {
				out = append(out, e)
			}
		}
		s.entries = out
	} else {
		cutoff := time.Now().AddDate(0, 0, -days).Unix()
		out := s.entries[:0]
		for _, e := range s.entries {
			if e.CreatedAt >= cutoff || e.Favorite {
				out = append(out, e)
			}
		}
		s.entries = out
	}
	return before - len(s.entries), s.saveLocked()
}

// Export renders the filtered log. Markdown reads like a notes page, JSON keeps
// every field a row carries, and CSV is the one a spreadsheet opens without
// asking. The static words are the interface's; the recorded text is left as it
// was answered.
func (s *Store) Export(f Filter, format string) string {
	entries := s.List(f, 0)
	switch format {
	case FormatJSON:
		return exportJSON(entries)
	case FormatCSV:
		return exportCSV(entries)
	default:
		return exportMarkdown(entries)
	}
}

// exportCSV quotes the way RFC 4180 asks: a cell with a comma, quote or newline
// in it becomes a quoted cell with every quote doubled. A translation log is
// full of all three.
func exportCSV(entries []Entry) string {
	rows := [][]string{
		{l10n.T("时间"), l10n.T("原文"), l10n.T("译文"), l10n.T("目标语言"), l10n.T("服务")},
	}
	for _, e := range entries {
		services := append([]string(nil), e.Services...)
		for i, name := range services {
			services[i] = l10n.T(name)
		}
		rows = append(rows, []string{
			e.Time().Format("2006-01-02 15:04:05"), e.Source, e.Target, e.TargetLang,
			strings.Join(services, "、"),
		})
	}
	// Excel guesses the codepage from the bytes when there is no mark in front,
	// and a log of Chinese translations arrives as mojibake.
	var sb strings.Builder
	sb.WriteRune('\uFEFF')
	for _, row := range rows {
		for i, cell := range row {
			if i > 0 {
				sb.WriteByte(',')
			}
			sb.WriteString(csvCell(cell))
		}
		sb.WriteByte('\n')
	}
	return sb.String()
}

func csvCell(s string) string {
	if !strings.ContainsAny(s, ",\"\n\r") {
		return s
	}
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

func exportJSON(entries []Entry) string {
	if len(entries) == 0 {
		return "[]\n"
	}
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		// A row of strings and numbers cannot fail to marshal; the empty array is
		// here so a broken export still opens.
		return "[]\n"
	}
	return string(data) + "\n"
}

func exportMarkdown(entries []Entry) string {
	var sb strings.Builder
	sb.WriteString("# " + l10n.T("Transdict 记录") + "\n\n")
	for _, e := range entries {
		sb.WriteString("## ")
		sb.WriteString(e.Time().Format("2006-01-02 15:04:05"))
		if e.TargetLang != "" {
			sb.WriteString(" · " + e.TargetLang)
		}
		sb.WriteString("\n\n")
		sb.WriteString("> " + strings.ReplaceAll(e.Source, "\n", "\n> ") + "\n\n")
		if e.Target != "" {
			sb.WriteString(e.Target + "\n\n")
		}
		if len(e.Services) > 0 {
			names := append([]string(nil), e.Services...)
			for i, name := range names {
				names[i] = l10n.T(name)
			}
			sb.WriteString(l10n.T("服务") + "：" + strings.Join(names, "、") + "\n\n")
		}
	}
	return sb.String()
}
