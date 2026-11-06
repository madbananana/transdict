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

// mdict serves offline MDict dictionaries: the user points it at a folder that
// holds .mdx files, and every dictionary that knows the headword answers. It is
// Multi because one folder is one provider instance while the folder itself can
// carry dozens of dictionaries.
type mdict struct {
	dir string
}

// mdictPathField is the Extra key the settings UI writes the folder into.
const mdictPathField = "path"

func init() {
	Register(settings.ServiceType{
		Kind: "mdict", Name: "MDict 离线词典", Note: "本地 .mdx 词典文件夹，仅单词，离线可用",
		Free: true, Multi: true,
		Fields: []settings.Field{
			{Key: mdictPathField, Label: "词典文件夹", Placeholder: "~/Dicts", Required: true, Folder: true},
		},
	}, func(p settings.Provider) (Provider, error) {
		dir := expandHome(strings.TrimSpace(p.Get(mdictPathField)))
		if dir == "" {
			return nil, fmt.Errorf("MDict：请先选择词典文件夹")
		}
		info, err := os.Stat(dir)
		if err != nil || !info.IsDir() {
			return nil, fmt.Errorf("MDict：词典文件夹不存在：%s", dir)
		}
		return &mdict{dir: dir}, nil
	})
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
		return nil, fmt.Errorf("MDict：文件夹内没有找到 .mdx 词典：%s", m.dir)
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
		return nil, fmt.Errorf("MDict 本地词典未收录该词：%s", clip(word, 40))
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

// dictionaries lists the candidate files, sorted so the answer order is stable.
func (m *mdict) dictionaries() ([]string, error) {
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
