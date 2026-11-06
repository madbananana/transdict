package dict

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"hash/adler32"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	"transdict/internal/settings"
)

// The fixture is written here, byte by byte, in the layout the MDict format
// notes describe for engine 2.0: an XML header in UTF-16LE with a big-endian
// length and an Adler32, then the keyword section (five 8-byte counts, a
// checksum, the key index and the key blocks), then the record section. Every
// data block carries the 4-byte compression type, the 4-byte Adler32 and a
// zlib stream, so the test proves the reader rather than a downloaded file.
type mdxBuilder struct {
	t       *testing.T
	wide    bool
	records []string
	// keys are appended in file order, each with the record it points at.
	keys []mdxBuildKey

	// Header options so a test can emit a correctly sized header instead of
	// patching bytes after the fact, which would desynchronise the length field.
	title     string
	noTitle   bool
	encoding  string
	encrypted string
}

type mdxBuildKey struct {
	word   string
	record int
}

func newMDXBuilder(t *testing.T, wideEncoding bool) *mdxBuilder {
	return &mdxBuilder{t: t, wide: wideEncoding, title: "测试词典", encrypted: "0"}
}

func (b *mdxBuilder) add(word, html string) {
	b.records = append(b.records, html)
	b.keys = append(b.keys, mdxBuildKey{word: word, record: len(b.records) - 1})
}

func (b *mdxBuilder) encodeText(s string) []byte {
	return append(b.encodeRaw(s), b.terminator()...)
}

// encodeRaw writes text without the terminator: the key index stores a word of
// exactly the length its size field says.
func (b *mdxBuilder) encodeRaw(s string) []byte {
	if !b.wide {
		return []byte(s)
	}
	units := utf16.Encode([]rune(s))
	out := make([]byte, 0, len(units)*2)
	for _, u := range units {
		var pair [2]byte
		binary.LittleEndian.PutUint16(pair[:], u)
		out = append(out, pair[:]...)
	}
	return out
}

func (b *mdxBuilder) terminator() []byte {
	if b.wide {
		return []byte{0, 0}
	}
	return []byte{0}
}

// textUnits counts the basic units of s: bytes for UTF-8, 2-byte units for
// UTF-16, which is what the size fields in the key index carry.
func (b *mdxBuilder) textUnits(s string) int {
	if b.wide {
		return len(utf16.Encode([]rune(s)))
	}
	return len(s)
}

// block wraps payload with the compression type and checksum prefix.
func (b *mdxBuilder) block(payload []byte) []byte {
	var compressed bytes.Buffer
	writer := zlib.NewWriter(&compressed)
	if _, err := writer.Write(payload); err != nil {
		b.t.Fatalf("zlib 写入失败: %v", err)
	}
	if err := writer.Close(); err != nil {
		b.t.Fatalf("zlib 关闭失败: %v", err)
	}
	head := make([]byte, 8)
	binary.LittleEndian.PutUint32(head[0:4], mdxCompressZlib)
	binary.BigEndian.PutUint32(head[4:8], adler32.Checksum(payload))
	return append(head, compressed.Bytes()...)
}

func number(v int64) []byte {
	out := make([]byte, 8)
	binary.BigEndian.PutUint64(out, uint64(v))
	return out
}

func number16(v int) []byte {
	out := make([]byte, 2)
	binary.BigEndian.PutUint16(out, uint16(v))
	return out
}

func (b *mdxBuilder) build(keyBlockSize int) []byte {
	if keyBlockSize < 1 {
		b.t.Fatal("keyBlockSize 必须为正")
	}

	// The record bodies are concatenated; a key stores the byte offset of its
	// own record inside that concatenation.
	var recordBlobs [][]byte
	offsets := make([]int64, len(b.records))
	var running int64
	for i, record := range b.records {
		offsets[i] = running
		encoded := b.encodeText(record)
		// One record block per key keeps the block walk under test honest.
		recordBlobs = append(recordBlobs, encoded)
		running += int64(len(encoded))
	}

	var blocks [][]mdxBuildKey
	for i := 0; i < len(b.keys); i += keyBlockSize {
		end := min(i+keyBlockSize, len(b.keys))
		blocks = append(blocks, b.keys[i:end])
	}

	// Key blocks.
	var keyBlocks [][]byte
	for _, group := range blocks {
		var payload bytes.Buffer
		for _, key := range group {
			payload.Write(number(offsets[key.record]))
			payload.Write(b.encodeText(key.word))
		}
		keyBlocks = append(keyBlocks, payload.Bytes())
	}
	keyBlockWrappers := make([][]byte, len(keyBlocks))
	for i, payload := range keyBlocks {
		keyBlockWrappers[i] = b.block(payload)
	}

	// The key index describes every block: how many keys it holds, the first
	// and last headword, and its on-disk sizes.
	var index bytes.Buffer
	for i, group := range blocks {
		index.Write(number(int64(len(group))))
		first := b.encodeRaw(group[0].word)
		last := b.encodeRaw(group[len(group)-1].word)
		index.Write(number16(b.textUnits(group[0].word)))
		index.Write(first)
		index.Write(number16(b.textUnits(group[len(group)-1].word)))
		index.Write(last)
		index.Write(number(int64(len(keyBlockWrappers[i]))))
		index.Write(number(int64(len(keyBlocks[i]))))
	}
	indexBlock := b.block(index.Bytes())

	var keyBlocksRaw bytes.Buffer
	for _, wrapper := range keyBlockWrappers {
		keyBlocksRaw.Write(wrapper)
	}

	// Keyword section counts.
	var keywordHead bytes.Buffer
	keywordHead.Write(number(int64(len(blocks))))
	keywordHead.Write(number(int64(len(b.keys))))
	keywordHead.Write(number(int64(index.Len())))
	keywordHead.Write(number(int64(len(indexBlock))))
	keywordHead.Write(number(int64(keyBlocksRaw.Len())))
	sum := make([]byte, 4)
	binary.BigEndian.PutUint32(sum, adler32.Checksum(keywordHead.Bytes()))
	keywordHead.Write(sum)

	// Record section: one block per record, so the reader has to walk several.
	var recordWrappers [][]byte
	for _, blob := range recordBlobs {
		recordWrappers = append(recordWrappers, b.block(blob))
	}
	var recordInfo bytes.Buffer
	var recordsRaw bytes.Buffer
	// Each record block contributes a compressed size then a decompressed size.
	for i, wrapper := range recordWrappers {
		recordInfo.Write(number(int64(len(wrapper))))
		recordInfo.Write(number(int64(len(recordBlobs[i]))))
	}
	for i, wrapper := range recordWrappers {
		recordInfo.Write(number(int64(len(wrapper))))
		recordInfo.Write(number(int64(len(recordBlobs[i]))))
	}
	for _, wrapper := range recordWrappers {
		recordsRaw.Write(wrapper)
	}

	var recordHead bytes.Buffer
	recordHead.Write(number(int64(len(recordWrappers))))
	recordHead.Write(number(int64(len(b.keys))))
	recordHead.Write(number(int64(recordInfo.Len())))
	recordHead.Write(number(int64(recordsRaw.Len())))

	headerXML := `<Dictionary GeneratedByEngineVersion="2.0" RequiredEngineVersion="2.0" ` +
		`Encoding="` + b.encodingName() + `" Format="Html" Compact="No" Compat="No" ` +
		`KeyCaseSensitive="No" Encrypted="` + b.encryptionFlag() + `" ` +
		b.titleAttribute() + `Description="fixture" DataSourceFormat="106" />`
	var headerRaw []byte
	if b.wide {
		for _, u := range utf16.Encode([]rune(headerXML)) {
			var pair [2]byte
			binary.LittleEndian.PutUint16(pair[:], u)
			headerRaw = append(headerRaw, pair[:]...)
		}
	} else {
		headerRaw = []byte(headerXML)
	}

	var out bytes.Buffer
	out.Write(number32(len(headerRaw)))
	out.Write(headerRaw)
	checksum := make([]byte, 4)
	binary.LittleEndian.PutUint32(checksum, adler32.Checksum(headerRaw))
	out.Write(checksum)

	out.Write(keywordHead.Bytes())
	out.Write(indexBlock)
	out.Write(keyBlocksRaw.Bytes())
	out.Write(recordHead.Bytes())
	out.Write(recordInfo.Bytes())
	out.Write(recordsRaw.Bytes())
	return out.Bytes()
}

func (b *mdxBuilder) encodingName() string {
	if b.encoding != "" {
		return b.encoding
	}
	if b.wide {
		return "UTF-16"
	}
	return "UTF-8"
}

func (b *mdxBuilder) encryptionFlag() string {
	if b.encrypted != "" {
		return b.encrypted
	}
	return "0"
}

// titleAttribute omits the attribute entirely, which is what a dictionary built
// without an explicit title looks like.
func (b *mdxBuilder) titleAttribute() string {
	if b.noTitle {
		return ""
	}
	return `Title="` + b.title + `" `
}

func number32(v int) []byte {
	out := make([]byte, 4)
	binary.BigEndian.PutUint32(out, uint32(v))
	return out
}

// mtimeAhead pushes a file's timestamps forward so a rewrite is distinguishable
// from the cached copy even on filesystems with coarse granularity.
func mtimeAhead() time.Time {
	return time.Now().Add(time.Minute)
}

func writeMDX(t *testing.T, dir, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("写入测试词典失败: %v", err)
	}
	return path
}

func TestMDXLookupUTF8(t *testing.T) {
	b := newMDXBuilder(t, false)
	b.add("apple", "<p>一个苹果<br>苹果树</p>")
	b.add("banana", "<div>香蕉</div>")
	b.add("cherry", "<span>樱桃</span>")
	b.add("date", "<b>枣</b>")

	path := writeMDX(t, t.TempDir(), "fixture.mdx", b.build(2))

	dict, err := openMDX(path)
	if err != nil {
		t.Fatalf("openMDX 失败: %v", err)
	}
	if dict.title != "测试词典" {
		t.Errorf("title = %q, 期望 测试词典", dict.title)
	}
	if len(dict.keyBlocks) != 2 {
		t.Fatalf("keyBlocks = %d, 期望 2", len(dict.keyBlocks))
	}

	cases := []struct {
		query    string
		wantKey  string
		wantText string
	}{
		{"apple", "apple", "一个苹果\n苹果树"},
		{"banana", "banana", "香蕉"},
		// The last key of a block has no following offset, which exercises the
		// "record runs to the end of its block" path.
		{"date", "date", "枣"},
		{"CHERRY", "cherry", "樱桃"},
	}
	for _, c := range cases {
		html, matched, err := dict.lookup(c.query)
		if err != nil {
			t.Errorf("lookup(%q) 失败: %v", c.query, err)
			continue
		}
		if matched != c.wantKey {
			t.Errorf("lookup(%q) 命中 %q, 期望 %q", c.query, matched, c.wantKey)
		}
		if got := stripHTMLTags(html); got != c.wantText {
			t.Errorf("lookup(%q) 正文 = %q, 期望 %q", c.query, got, c.wantText)
		}
	}

	if _, _, err := dict.lookup("kiwi"); !mdxIsNotFound(err) {
		t.Errorf("未收录的词应报未收录, 实际: %v", err)
	}
	// Surrounding whitespace is the provider's business (it trims the query), so
	// the raw index stays literal and a padded key is a miss.
	if _, _, err := dict.lookup(" apple "); err == nil || !mdxIsNotFound(err) {
		t.Errorf("带空格的查询在索引层应未命中, 实际: %v", err)
	}
}

func TestMDXLookupUTF16(t *testing.T) {
	b := newMDXBuilder(t, true)
	// Keys go in the order the reader's collation expects: ASCII before CJK.
	b.add("hello", "<p>你好</p>")
	b.add("词汇", "<p>词语与概念</p>")

	path := writeMDX(t, t.TempDir(), "utf16.mdx", b.build(1))
	dict, err := openMDX(path)
	if err != nil {
		t.Fatalf("openMDX 失败: %v", err)
	}
	html, matched, err := dict.lookup("词汇")
	if err != nil {
		t.Fatalf("lookup 失败: %v", err)
	}
	if matched != "词汇" || stripHTMLTags(html) != "词语与概念" {
		t.Errorf("命中 %q / %q", matched, stripHTMLTags(html))
	}
	if _, matched, err := dict.lookup("HELLO"); err != nil {
		t.Errorf("大小写不敏感命中失败: %v", err)
	} else if matched != "hello" {
		t.Errorf("命中 %q, 期望 hello", matched)
	}
}

func TestMDXRejectsUnknownLayouts(t *testing.T) {
	cases := []struct {
		name string
		head string
		want string
		pad  bool
	}{
		{name: "v10", head: "MDX1V10\x00\x00", want: "暂不支持 MDX v10/LZMA 格式", pad: true},
		{name: "v9", head: "MDX1V90\x00\x00\x00", want: "暂不支持 MDX v9", pad: true},
		// A 16-byte file whose header length claims more than the file holds.
		{name: "truncated", head: "\x00\x00\x00\x10abc", want: "读取词典头部失败", pad: true},
		// Shorter than the smallest possible header frame: rejected up front, so
		// this case must not be padded.
		{name: "tiny", head: "ab", want: "文件过小"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			body := []byte(c.head)
			if c.pad {
				body = append(body, bytes.Repeat([]byte{0}, 16-len(body))...)
			}
			path := writeMDX(t, t.TempDir(), "x.mdx", body)
			_, err := openMDX(path)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("错误 = %v, 期望包含 %q", err, c.want)
			}
		})
	}
}

func TestMDXRejectsEncryptedAndOldEncoding(t *testing.T) {
	t.Run("encrypted", func(t *testing.T) {
		b := newMDXBuilder(t, false)
		b.encrypted = "1"
		b.add("apple", "<p>苹果</p>")
		_, err := openMDX(writeMDX(t, t.TempDir(), "enc.mdx", b.build(1)))
		if err == nil || !strings.Contains(err.Error(), "已加密") {
			t.Fatalf("错误 = %v, 期望提示已加密", err)
		}
	})
	t.Run("gbk", func(t *testing.T) {
		b := newMDXBuilder(t, false)
		b.encoding = "GBK"
		b.add("apple", "<p>苹果</p>")
		_, err := openMDX(writeMDX(t, t.TempDir(), "gbk.mdx", b.build(1)))
		if err == nil || !strings.Contains(err.Error(), "暂不支持 GBK") {
			t.Fatalf("错误 = %v, 期望提示暂不支持 GBK", err)
		}
	})
	t.Run("engine 3", func(t *testing.T) {
		b := newMDXBuilder(t, false)
		b.add("apple", "<p>苹果</p>")
		data := bytes.Replace(b.build(1), []byte(`GeneratedByEngineVersion="2.0"`),
			[]byte(`GeneratedByEngineVersion="3.0"`), 1)
		_, err := openMDX(writeMDX(t, t.TempDir(), "v3.mdx", data))
		if err == nil || !strings.Contains(err.Error(), "暂不支持 MDict 3.0") {
			t.Fatalf("错误 = %v, 期望提示暂不支持 3.0 格式", err)
		}
	})
	t.Run("lzo", func(t *testing.T) {
		b := newMDXBuilder(t, false)
		b.add("apple", "<p>苹果</p>")
		data := b.build(1)
		// Flip the key index block's compression type to LZO (magic 1).
		headerLen := int(binary.BigEndian.Uint32(data[0:4]))
		counts := 5*8 + 4
		at := 4 + headerLen + 4 + counts
		if int(data[at]) != mdxCompressZlib {
			t.Fatalf("偏移 %d 处压缩类型 = %d, 期望 %d", at, data[at], mdxCompressZlib)
		}
		data[at] = mdxCompressLZO
		_, err := openMDX(writeMDX(t, t.TempDir(), "lzo.mdx", data))
		if err == nil || !strings.Contains(err.Error(), "暂不支持 LZO") {
			t.Fatalf("错误 = %v, 期望提示暂不支持 LZO", err)
		}
	})
}

func TestMDXProviderExplain(t *testing.T) {
	dir := t.TempDir()

	first := newMDXBuilder(t, false)
	first.add("apple", "<div class=\"p\">一个<b>苹果</b></div><div>引申义</div>")
	writeMDX(t, dir, "first.mdx", first.build(1))

	second := newMDXBuilder(t, false)
	second.add("apple", "<p>苹果树</p>")
	second.add("zebra", "<p>斑马</p>")
	writeMDX(t, dir, "second.mdx", second.build(2))

	// A sidecar title wins when the header has none.
	mdxWithNoTitle := newMDXBuilder(t, false)
	mdxWithNoTitle.noTitle = true
	mdxWithNoTitle.add("apple", "<p>无标题词典</p>")
	writeMDX(t, dir, "third.mdx", mdxWithNoTitle.build(1))
	if err := os.WriteFile(filepath.Join(dir, "third.mddictionary"),
		[]byte(`<title>补充标题</title>`), 0o600); err != nil {
		t.Fatalf("写入 sidecar 失败: %v", err)
	}

	p, err := New(settings.Provider{ID: "dict-mdict", Category: settings.CatDict,
		Kind: "mdict", Label: "MDict", Enabled: true,
		Extra: map[string]string{mdictPathField: dir}})
	if err != nil {
		t.Fatalf("New(mdict) 失败: %v", err)
	}

	entries, err := p.Explain(t.Context(), "apple", "en", "zh-CN")
	if err != nil {
		t.Fatalf("Explain 失败: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("条目数 = %d, 期望 3", len(entries))
	}
	if entries[0].Word != "apple" {
		t.Errorf("Word = %q", entries[0].Word)
	}
	if !strings.HasPrefix(entries[0].Text, "《测试词典》") {
		t.Errorf("首条未带词典标题: %q", entries[0].Text)
	}
	if want := "《测试词典》\n\n一个苹果\n引申义"; entries[0].Text != want {
		t.Errorf("首条正文 = %q, 期望 %q", entries[0].Text, want)
	}
	if !strings.Contains(entries[2].Text, "《补充标题》") {
		t.Errorf("sidecar 标题未生效: %q", entries[2].Text)
	}

	// The provider trims the query, so a padded copy of the same word answers.
	padded, err := p.Explain(t.Context(), "  apple ", "en", "zh-CN")
	if err != nil || len(padded) != 3 {
		t.Fatalf("带空格的查询应由 provider 归一化, 实际 %d 条: %v", len(padded), err)
	}

	if _, err := p.Explain(t.Context(), "nobody", "en", "zh-CN"); err == nil ||
		!strings.Contains(errText(err), "未收录") {
		t.Errorf("未收录时应报错, 实际: %v", err)
	}

	if _, err := p.Explain(t.Context(), "   ", "en", "zh-CN"); err == nil {
		t.Error("空查询应报错")
	}
}

func TestMDXProviderNeedsFolder(t *testing.T) {
	if _, err := New(settings.Provider{Kind: "mdict"}); err == nil {
		t.Fatal("未填文件夹时构造应失败")
	}
	missing := filepath.Join(t.TempDir(), "nope")
	if _, err := New(settings.Provider{Kind: "mdict",
		Extra: map[string]string{mdictPathField: missing}}); err == nil ||
		!strings.Contains(errText(err), "文件夹不存在") {
		t.Fatalf("错误 = %v, 期望提示文件夹不存在", err)
	}

	meta, ok := Meta("mdict")
	if !ok {
		t.Fatal("mdict 未注册")
	}
	if !meta.Multi || !meta.Free {
		t.Errorf("meta = %+v, 期望 Multi+Free", meta)
	}
	if len(meta.Fields) != 1 || meta.Fields[0].Key != "path" || !meta.Fields[0].Required {
		t.Errorf("fields = %+v, 期望一个必填的 path", meta.Fields)
	}
	configured := settings.Provider{Extra: map[string]string{"path": "x"}}
	if !configured.IsReady(meta) {
		t.Error("填了 path 应视为就绪")
	}
	if (settings.Provider{}).IsReady(meta) {
		t.Error("空 path 不应视为就绪")
	}
}

func TestMDXSidecarTitleJSON(t *testing.T) {
	dir := t.TempDir()
	b := newMDXBuilder(t, false)
	b.noTitle = true
	b.add("apple", "<p>苹果</p>")
	path := writeMDX(t, dir, "x.mdx", b.build(1))
	if err := os.WriteFile(path+".meta", []byte(`{"title":"JSON 标题"}`), 0o600); err != nil {
		t.Fatalf("写入 sidecar 失败: %v", err)
	}
	if got := mdxSidecarTitle(path); got != "JSON 标题" {
		t.Errorf("sidecar 标题 = %q", got)
	}
	dict, err := openMDX(path)
	if err != nil {
		t.Fatalf("openMDX 失败: %v", err)
	}
	if got := mdictTitle(dict, path); got != "《JSON 标题》" {
		t.Errorf("标题回退 = %q", got)
	}
}

func TestMDXCacheInvalidatesOnEdit(t *testing.T) {
	dir := t.TempDir()
	b := newMDXBuilder(t, false)
	b.add("apple", "<p>第一版</p>")
	path := writeMDX(t, dir, "cache.mdx", b.build(1))

	first, err := cachedMDX(path)
	if err != nil {
		t.Fatalf("首次解析失败: %v", err)
	}
	if again, err := cachedMDX(path); err != nil || again != first {
		t.Fatalf("同一路径与修改时间应命中缓存, got %v/%v", again, err)
	}

	b2 := newMDXBuilder(t, false)
	b2.add("apple", "<p>第二版</p>")
	b2.add("peer", "<p>同伴</p>")
	if err := os.WriteFile(path, b2.build(2), 0o600); err != nil {
		t.Fatalf("改写词典失败: %v", err)
	}
	if err := os.Chtimes(path, mtimeAhead(), mtimeAhead()); err != nil {
		t.Fatalf("调整修改时间失败: %v", err)
	}
	second, err := cachedMDX(path)
	if err != nil {
		t.Fatalf("改写后解析失败: %v", err)
	}
	if second == first {
		t.Fatal("修改时间变化后应重新解析")
	}
	if _, _, err := second.lookup("peer"); err != nil {
		t.Errorf("改写后的新词应可查到: %v", err)
	}
}

func TestStripHTMLTags(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain", "hello", "hello"},
		{"drops tags", "<p>a</p><p>b</p>", "a\nb"},
		{"br", "一<br>二<br/>三", "一\n二\n三"},
		{"inline keeps one line", "<b>重</b>点<span>内</span>联", "重点内联"},
		{"entities", "&lt;a&gt; &amp; &quot;q&quot; &#39;x&#39; &#8212; &#x2014; &nbsp;y",
			"<a> & \"q\" 'x' — — y"},
		{"unknown entity stays", "a &nbspx b", "a &nbspx b"},
		// Script, style and comment content is not text, so it disappears without
		// leaving a seam, exactly like a browser's innerText of the fragment.
		{"script content", "x<script>var a = 1;</script>y", "xy"},
		{"style content", "x<style>.a{color:red}</style>y", "xy"},
		{"comment", "a<!-- 注释 -->b", "ab"},
		{"unterminated script", "x<script>var a = 1;", "x"},
		{"table rows", "<table><tr><td>甲</td><td>乙</td></tr></table>", "甲\n乙"},
		{"collapses blanks", "<div>第一段</div>\n\n\n\n<div>第二段</div>", "第一段\n\n第二段"},
		{"trims", "   \n <p>  多余   空格  </p> \n ", "多余 空格"},
		{"unterminated tag", "abc<def", "abc"},
		{"nested quotes", `<a href="x">链接</a>`, "链接"},
		{"invalid byte", "a\xffb", "ab"},
		{"chinese punctuation", "<i>见「详情</i>」", "见「详情」"},
		{"nbsp run", "a&nbsp;&nbsp;b", "a b"},
		{"empty", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := stripHTMLTags(c.in); got != c.want {
				t.Errorf("stripHTMLTags(%q) = %q, 期望 %q", c.in, got, c.want)
			}
		})
	}
}

func TestStripHTMLTagsIsIdempotent(t *testing.T) {
	b := newMDXBuilder(t, false)
	b.add("apple", "<div><p>第一段</p><p>第二段<br>换行</p></div>")
	path := writeMDX(t, t.TempDir(), "idem.mdx", b.build(1))
	dict, err := openMDX(path)
	if err != nil {
		t.Fatalf("openMDX 失败: %v", err)
	}
	html, _, err := dict.lookup("apple")
	if err != nil {
		t.Fatalf("lookup 失败: %v", err)
	}
	once := stripHTMLTags(html)
	if twice := stripHTMLTags(once); twice != once {
		t.Errorf("二次清洗不稳定: %q vs %q", once, twice)
	}
}
