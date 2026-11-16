package dict

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"transdict/internal/settings"
)

// mdict serves offline MDict dictionaries. Easydict imports individual .mdx
// files, and so does this service: 导入词典 stores the files in FilesField and
// those answer in the order they were imported. A folder still works on its
// own — the older quick way — and is scanned only while no file is imported.
type mdict struct {
	dir   string
	files []string
}

// PathField is the Extra key the settings UI writes the folder into.
const PathField = "path"

// FilesField is the Extra key holding one absolute .mdx path per line. It is
// exported for the settings window, which edits the list.
const FilesField = "files"

func init() {
	Register(settings.ServiceType{
		Kind: "mdict", Name: "MDict 离线词典", Note: "本地 .mdx 词典文件或文件夹，仅单词，离线可用",
		Free: true, Multi: true,
		Fields: []settings.Field{
			{Key: PathField, Label: "词典文件夹", Placeholder: "~/Dicts", Required: true, Folder: true},
		},
	}, func(p settings.Provider) (Provider, error) {
		files := ParseFiles(p.Get(FilesField))
		if len(files) > 0 {
			usable := make([]string, 0, len(files))
			for _, file := range files {
				if info, err := os.Stat(file); err == nil && !info.IsDir() {
					usable = append(usable, file)
				}
			}
			if len(usable) == 0 {
				return nil, fmt.Errorf("MDict：导入的词典文件都不存在")
			}
			return &mdict{files: usable}, nil
		}
		dir := expandHome(strings.TrimSpace(p.Get(PathField)))
		if dir == "" {
			return nil, fmt.Errorf("MDict：请先选择词典文件夹或导入词典")
		}
		info, err := os.Stat(dir)
		if err != nil || !info.IsDir() {
			return nil, fmt.Errorf("MDict：词典文件夹不存在：%s", dir)
		}
		return &mdict{dir: dir}, nil
	})
}

// ParseFiles turns the stored newline-separated list into clean paths. Blank
// lines and duplicates are dropped so an import cannot add the same file twice.
func ParseFiles(raw string) []string {
	seen := map[string]bool{}
	var out []string
	for _, line := range strings.Split(raw, "\n") {
		path := strings.TrimSpace(line)
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		out = append(out, path)
	}
	return out
}

// JoinFiles is the inverse of ParseFiles, kept beside it so both sides of the
// Extra value agree on the separator.
func JoinFiles(paths []string) string {
	return strings.Join(ParseFiles(strings.Join(paths, "\n")), "\n")
}

func (m *mdict) Label() string { return "MDict 离线词典" }

func (m *mdict) Explain(ctx context.Context, text, sourceLang, targetLang string) ([]Entry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	word := strings.TrimSpace(text)
	if word == "" {
		return nil, fmt.Errorf("MDict：查询内容为空")
	}

	files, err := m.dictionaries()
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		if m.dir != "" {
			return nil, fmt.Errorf("MDict：文件夹内没有找到 .mdx 词典：%s", m.dir)
		}
		return nil, fmt.Errorf("MDict：尚未导入任何词典")
	}

	var (
		out      []Entry
		misses   int
		failures []string
	)
	for _, file := range files {
		dict, err := cachedMDX(file)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s：%s", pathBase(file), err))
			continue
		}
		html, matched, err := dict.lookup(word)
		if err != nil {
			if mdxIsNotFound(err) {
				misses++
			} else {
				failures = append(failures, fmt.Sprintf("%s：%s", pathBase(file), err))
			}
			continue
		}
		body := stripHTMLTags(html)
		if strings.TrimSpace(body) == "" {
			misses++
			continue
		}
		out = append(out, Entry{Word: matched, Text: mdictTitle(dict, file) + "\n\n" + body})
	}

	if len(out) == 0 {
		if len(failures) > 0 && misses == 0 {
			return nil, fmt.Errorf("MDict：%s", strings.Join(failures, "；"))
		}
		return nil, NotFound(fmt.Sprintf("MDict 本地词典未收录该词：%s", clip(word, 40)))
	}
	return out, nil
}

// mdictTitle keeps the dictionary name out of the body when the header has none.
func mdictTitle(dict *mdxFile, file string) string {
	title := strings.TrimSpace(dict.title)
	if title == "" {
		title = strings.TrimSpace(mdxSidecarTitle(file))
	}
	if title == "" {
		return strings.TrimSuffix(pathBase(file), filepath.Ext(file))
	}
	return "《" + title + "》"
}

// dictionaries lists the files to query: the imported list when there is one,
// otherwise every .mdx in the folder. Both are sorted for a stable answer order.
func (m *mdict) dictionaries() ([]string, error) {
	if len(m.files) > 0 {
		out := append([]string(nil), m.files...)
		sort.Strings(out)
		return out, nil
	}
	dirEntries, err := os.ReadDir(m.dir)
	if err != nil {
		return nil, fmt.Errorf("MDict：无法读取词典文件夹：%s", err)
	}
	var out []string
	for _, e := range dirEntries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		switch strings.ToLower(filepath.Ext(name)) {
		case ".mdx", ".md":
			out = append(out, filepath.Join(m.dir, name))
		}
	}
	sort.Strings(out)
	return out, nil
}

// mdxCached is the parsed index per file, keyed by path plus modification time
// so an edited or replaced dictionary is picked up without a restart.
var (
	mdictCacheMu = sync.RWMutex{}
	mdictCache   = map[string]mdxCached{}
)

type mdxCached struct {
	file *mdxFile
	err  error
}

func cachedMDX(path string) (*mdxFile, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	key := path + "\x00" + info.ModTime().Format(time.RFC3339Nano)

	mdictCacheMu.RLock()
	cached, ok := mdictCache[key]
	mdictCacheMu.RUnlock()
	if ok {
		return cached.file, cached.err
	}

	mdictCacheMu.Lock()
	defer mdictCacheMu.Unlock()
	if cached, ok := mdictCache[key]; ok {
		return cached.file, cached.err
	}
	parsed, parseErr := openMDX(path)
	mdictCache[key] = mdxCached{file: parsed, err: parseErr}
	return parsed, parseErr
}

// mdxSidecarTitle reads the metadata file some builders ship next to a
// dictionary. There is no single standard, so several known spellings are
// tried and the header's own Title stays the primary source.
func mdxSidecarTitle(path string) string {
	base := strings.TrimSuffix(path, filepath.Ext(path))
	candidates := []string{
		base + ".mddictionary",
		base + ".mdct",
		path + ".meta",
		base + ".meta",
		base + ".name",
	}
	for _, candidate := range candidates {
		data, err := os.ReadFile(candidate)
		if err != nil {
			continue
		}
		if title := mdxTitleFromMetadata(data); title != "" {
			return title
		}
	}
	return ""
}

// mdxTitleFromMetadata pulls a title out of either an XML or a JSON sidecar.
func mdxTitleFromMetadata(data []byte) string {
	text := string(data)
	if attrs := mdxXMLAttrs(text); attrs["Title"] != "" {
		return attrs["Title"]
	}
	if title := mdxTagContent(text, "title"); title != "" {
		return title
	}
	if title := mdxTagContent(text, "name"); title != "" {
		return title
	}

	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err == nil {
		for _, key := range []string{"title", "name", "dictName", "词典"} {
			if value, ok := payload[key].(string); ok && strings.TrimSpace(value) != "" {
				return strings.TrimSpace(value)
			}
		}
	}
	return ""
}

// mdxTagContent returns the text between <tag> and </tag>, once.
func mdxTagContent(text, tag string) string {
	open := "<" + tag
	index := strings.Index(strings.ToLower(text), strings.ToLower(open))
	if index < 0 {
		return ""
	}
	rest := text[index:]
	close := strings.Index(rest, ">")
	if close < 0 {
		return ""
	}
	end := strings.Index(rest[close+1:], "</"+tag)
	if end < 0 {
		return ""
	}
	return strings.TrimSpace(mdxUnescapeXML(rest[close+1 : close+1+end]))
}

// expandHome resolves a leading ~ so the settings field can hold "~/Dicts".
func expandHome(path string) string {
	if path == "" || (path[0] != '~') {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if len(path) == 1 {
		return home
	}
	if path[1] == '/' || path[1] == filepath.Separator {
		return filepath.Join(home, path[2:])
	}
	return path
}
