package dict

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf16"
)

// This file is the binary reader for MDict's .mdx container.
//
// The layout implemented here is the one the MDict format notes describe for
// the engine versions MdxBuilder 2.x/3.x produce (header attribute
// GeneratedByEngineVersion="2.0"):
//
//	header section   4-byte big-endian length, an XML string (UTF-16LE by
//	                 default) and a 4-byte Adler32 of that string
//	keyword section  five 8-byte big-endian counts (four 4-byte counts for
//	                 engine 1.2) plus, for 2.0, a 4-byte Adler32; then the
//	                 compressed key index, then the compressed key blocks
//	record section   four 8-byte counts, the per-block compressed/decompressed
//	                 sizes, then the compressed record blocks
//
// Every compressed block on disk starts with a 4-byte compression type and a
// 4-byte big-endian Adler32 of the plain data. Type 0 is stored raw, type 2 is
// zlib, and type 1 is LZO, which the standard library cannot decode.
//
// Note on "MDX v9/v10": no public MDict format document describes a file that
// begins with an MDX1V9/MDX1V10 magic and a packed-integer header, so nothing
// here claims to read one. Those signatures are detected and reported rather
// than parsed.

const (
	mdxCompressPlain = 0
	mdxCompressLZO   = 1
	mdxCompressZlib  = 2

	mdxMaxHeaderBytes = 8 << 20
)

// mdxKeyBlock is one entry of the key index: the headword range the block
// covers plus where its compressed bytes live.
type mdxKeyBlock struct {
	first      string
	last       string
	numEntries int64
	compSize   int64
	decompSize int64
	diskOffset int64
}

// mdxRecordBlock is one compressed run of record bodies.
type mdxRecordBlock struct {
	compSize   int64
	decompSize int64
	diskOffset int64
}

// mdxFile is a parsed .mdx index. It holds no file handle: blocks are read on
// demand so a long-running app never runs out of descriptors.
type mdxFile struct {
	path   string
	title  string
	engine float64

	unit          int // bytes per text unit: 1 for UTF-8, 2 for UTF-16
	numWidth      int // width of the counts: 4 before engine 2.0, 8 from 2.0 up
	caseSensitive bool

	numEntries       int64
	keyBlocks        []mdxKeyBlock
	recordBlocks     []mdxRecordBlock
	keyDataOffset    int64
	recordDataOffset int64
}

type mdxKeyEntry struct {
	recordOffset int64
	word         string
}

// openMDX parses the header and both indexes. Anything this reader deliberately
// does not support comes back as a Chinese message rather than a silent miss.
func openMDX(path string) (*mdxFile, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	head, err := readAt(f, 0, 16)
	if err != nil {
		return nil, fmt.Errorf("词典文件过小，不是可识别的 MDX 词典：%s", clip(pathBase(path), 40))
	}
	if msg, ok := detectUnsupportedMagic(head); ok {
		return nil, errors.New(msg)
	}

	headerLen := int(binary.BigEndian.Uint32(head[0:4]))
	if headerLen <= 0 || headerLen > mdxMaxHeaderBytes {
		return nil, fmt.Errorf("无法解析词典头部（%s）：长度 %d 不合理", clip(pathBase(path), 40), headerLen)
	}
	rawHeader, err := readAt(f, 4, headerLen)
	if err != nil {
		return nil, fmt.Errorf("读取词典头部失败：%s", clip(pathBase(path), 40))
	}
	sum, err := readAt(f, int64(4+headerLen), 4)
	if err != nil {
		return nil, fmt.Errorf("词典头部不完整：%s", clip(pathBase(path), 40))
	}
	headerText, err := decodeMDXHeaderXML(rawHeader)
	if err != nil {
		return nil, fmt.Errorf("词典头部不是合法的 XML：%s", clip(pathBase(path), 40))
	}
	_ = sum // checksum is advisory: mismatched files still read correctly

	attrs := mdxXMLAttrs(headerText)
	m := &mdxFile{path: path}
	m.title = mdxFirstNonEmpty(attrs["Title"], attrs["DictionaryTitle"], attrs["Name"])
	m.engine = mdxFloat(attrs["GeneratedByEngineVersion"], 2)

	switch strings.ToLower(attrs["Encoding"]) {
	case "utf-8", "utf8":
		m.unit = 1
	case "utf-16", "utf-16le", "utf16", "utf-16LE", "unicode":
		m.unit = 2
	case "gbk", "gb2312", "big5":
		return nil, fmt.Errorf("暂不支持 %s 编码的 MDX 词典，请改用 UTF-8 或 UTF-16 版本", strings.ToUpper(attrs["Encoding"]))
	default:
		m.unit = 1
	}

	if enc := mdxInt(attrs["Encrypted"], 0); enc != 0 {
		return nil, errors.New("该 MDX 词典已加密，暂不支持")
	}
	if m.engine >= 3 {
		return nil, fmt.Errorf("暂不支持 MDict %.1f 格式（分块已加密），请转换到 2.0 版本", m.engine)
	}
	m.caseSensitive = strings.EqualFold(attrs["KeyCaseSensitive"], "Yes")
	if m.engine < 2 {
		m.numWidth = 4
	} else {
		m.numWidth = 8
	}

	if err := m.readKeySection(f, int64(8+headerLen)); err != nil {
		return nil, err
	}
	if err := m.readRecordSection(f); err != nil {
		return nil, err
	}
	if len(m.keyBlocks) == 0 || m.numEntries == 0 {
		return nil, fmt.Errorf("MDX 词典没有可用词条：%s", clip(pathBase(path), 40))
	}
	return m, nil
}

// detectUnsupportedMagic reports the signatures that are not the MDict engine
// layout, including the MDX1V9/MDX1V10 headers some converters advertise.
func detectUnsupportedMagic(head []byte) (string, bool) {
	switch {
	case bytes.HasPrefix(head, []byte("MDX1V1")), bytes.HasPrefix(head, []byte("MDX1V10")):
		return "暂不支持 MDX v10/LZMA 格式", true
	case bytes.HasPrefix(head, []byte("MDX1V9")):
		return "暂不支持 MDX v9 二进制头格式（公开格式文档未收录该布局）", true
	default:
		return "", false
	}
}

// readKeySection parses the counts, the key index and lays out the key blocks.
func (m *mdxFile) readKeySection(f *os.File, offset int64) error {
	count := 5
	if m.numWidth == 4 {
		count = 4
	}
	raw, err := readAt(f, offset, count*m.numWidth)
	if err != nil {
		return fmt.Errorf("读取 MDX 索引区失败：%s", clip(pathBase(m.path), 40))
	}
	pos := 0
	numKeyBlocks := m.takeNumber(raw, &pos)
	m.numEntries = m.takeNumber(raw, &pos)
	if m.numWidth == 8 {
		_ = m.takeNumber(raw, &pos) // decompressed length of the key index
	}
	keyIndexLen := m.takeNumber(raw, &pos)
	keyBlocksLen := m.takeNumber(raw, &pos)

	// Engine 2.0 closes the five counts with a 4-byte Adler32 over them; the
	// 1.2 layout has neither the extra count nor the checksum.
	indexOffset := offset + int64(count*m.numWidth)
	if m.numWidth == 8 {
		if _, err := readAt(f, indexOffset, 4); err != nil {
			return fmt.Errorf("MDX 索引区校验和缺失：%s", clip(pathBase(m.path), 40))
		}
		indexOffset += 4
	}
	if numKeyBlocks <= 0 || numKeyBlocks > 1<<20 || keyIndexLen <= 0 {
		return fmt.Errorf("MDX 索引区数值异常：%s", clip(pathBase(m.path), 40))
	}

	compressed, err := readAt(f, indexOffset, int(keyIndexLen))
	if err != nil {
		return fmt.Errorf("读取 MDX 关键字索引失败：%s", clip(pathBase(m.path), 40))
	}
	index, err := m.decompressBlock(compressed)
	if err != nil {
		return err
	}

	m.keyDataOffset = indexOffset + keyIndexLen
	cursor := 0
	disk := m.keyDataOffset
	for i := int64(0); i < numKeyBlocks && cursor < len(index); i++ {
		var b mdxKeyBlock
		b.numEntries = m.takeNumber(index, &cursor)
		b.first = m.readUnits(index, &cursor, int(m.takeNumber16(index, &cursor)))
		b.last = m.readUnits(index, &cursor, int(m.takeNumber16(index, &cursor)))
		b.compSize = m.takeNumber(index, &cursor)
		b.decompSize = m.takeNumber(index, &cursor)
		b.diskOffset = disk
		disk += b.compSize
		m.keyBlocks = append(m.keyBlocks, b)
	}
	if len(m.keyBlocks) == 0 {
		return fmt.Errorf("MDX 关键字索引为空：%s", clip(pathBase(m.path), 40))
	}
	// The record section follows the key blocks; keyBlocksLen is the total size
	// of the compressed key blocks only.
	m.recordDataOffset = m.keyDataOffset + keyBlocksLen
	return nil
}

// readRecordSection parses the record counts and per-block sizes.
func (m *mdxFile) readRecordSection(f *os.File) error {
	header, err := readAt(f, m.recordDataOffset, 4*m.numWidth)
	if err != nil {
		return fmt.Errorf("读取 MDX 记录区失败：%s", clip(pathBase(m.path), 40))
	}
	pos := 0
	numRecordBlocks := m.takeNumber(header, &pos)
	entries := m.takeNumber(header, &pos)
	infoLen := m.takeNumber(header, &pos)
	blocksLen := m.takeNumber(header, &pos)
	_ = entries

	if numRecordBlocks <= 0 || numRecordBlocks > 1<<22 || infoLen <= 0 {
		return fmt.Errorf("MDX 记录区数值异常：%s", clip(pathBase(m.path), 40))
	}
	info, err := readAt(f, m.recordDataOffset+int64(4*m.numWidth), int(infoLen))
	if err != nil {
		return fmt.Errorf("读取 MDX 记录索引失败：%s", clip(pathBase(m.path), 40))
	}
	cursor := 0
	disk := m.recordDataOffset + int64(4*m.numWidth) + infoLen
	for i := int64(0); i < numRecordBlocks && cursor+2*m.numWidth <= len(info); i++ {
		var b mdxRecordBlock
		b.compSize = m.takeNumber(info, &cursor)
		b.decompSize = m.takeNumber(info, &cursor)
		b.diskOffset = disk
		disk += b.compSize
		m.recordBlocks = append(m.recordBlocks, b)
	}
	if len(m.recordBlocks) == 0 {
		return fmt.Errorf("MDX 记录区为空：%s", clip(pathBase(m.path), 40))
	}
	if blocksLen > 0 {
		// Sanity only: a mismatch means the file was truncated on download.
		var total int64
		for _, b := range m.recordBlocks {
			total += b.compSize
		}
		if total > blocksLen {
			return fmt.Errorf("MDX 记录区长度与声明不符（文件可能不完整）：%s", clip(pathBase(m.path), 40))
		}
	}
	return nil
}

// lookup finds one headword and returns its raw record text plus the key the
// file actually stores.
func (m *mdxFile) lookup(word string) (string, string, error) {
	blocks := m.keyBlocks
	if len(blocks) == 0 {
		return "", "", fmt.Errorf("MDX 词典没有可用词条")
	}
	// Blocks are sorted by headword range: take the first block whose last key
	// is not before the query, which is the block that could hold it.
	low, high := 0, len(blocks)-1
	for low < high {
		mid := (low + high) / 2
		if m.less(blocks[mid].last, word) {
			low = mid + 1
		} else {
			high = mid
		}
	}
	target := low
	if m.less(blocks[target].last, word) {
		return "", "", errMDXNotFound
	}

	entries, err := m.readKeyBlock(blocks[target])
	if err != nil {
		return "", "", err
	}

	low, high = 0, len(entries)-1
	for low <= high {
		mid := (low + high) / 2
		switch {
		case m.less(entries[mid].word, word):
			low = mid + 1
		case m.less(word, entries[mid].word):
			high = mid - 1
		default:
			// The key entry carries a byte offset into the concatenation of all
			// decompressed record blocks; the next key bounds the record when it
			// lives in the same block.
			limit := int64(-1)
			if mid+1 < len(entries) {
				limit = entries[mid+1].recordOffset
			}
			text, err := m.readRecord(entries[mid].recordOffset, limit)
			if err != nil {
				return "", "", err
			}
			return text, entries[mid].word, nil
		}
	}
	return "", "", errMDXNotFound
}

// readKeyBlock decompresses one block of headwords.
func (m *mdxFile) readKeyBlock(b mdxKeyBlock) ([]mdxKeyEntry, error) {
	f, err := os.Open(m.path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	compressed, err := readAt(f, b.diskOffset, int(b.compSize))
	if err != nil {
		return nil, fmt.Errorf("读取 MDX 关键字块失败：%s", clip(pathBase(m.path), 40))
	}
	plain, err := m.decompressBlock(compressed)
	if err != nil {
		return nil, err
	}

	var out []mdxKeyEntry
	cursor := 0
	for int64(len(out)) < b.numEntries && cursor < len(plain) {
		offset := m.takeNumber(plain, &cursor)
		word, ok := m.readTerminated(plain, &cursor)
		if !ok {
			break
		}
		out = append(out, mdxKeyEntry{recordOffset: offset, word: word})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("MDX 关键字块解析为空：%s", clip(pathBase(m.path), 40))
	}
	return out, nil
}

// readRecord returns the record body starting at a byte offset in the
// concatenation of all decompressed record blocks, cut off at limit when known.
func (m *mdxFile) readRecord(start, limit int64) (string, error) {
	f, err := os.Open(m.path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	cursor := int64(0)
	for _, b := range m.recordBlocks {
		if start < cursor+b.decompSize {
			compressed, err := readAt(f, b.diskOffset, int(b.compSize))
			if err != nil {
				return "", fmt.Errorf("读取 MDX 记录块失败：%s", clip(pathBase(m.path), 40))
			}
			plain, err := m.decompressBlock(compressed)
			if err != nil {
				return "", err
			}
			from := int(start - cursor)
			if from > len(plain) {
				return "", errMDXNotFound
			}
			to := len(plain)
			if limit >= 0 {
				candidate := int(limit - cursor)
				if candidate > 0 && candidate <= len(plain) {
					to = candidate
				}
			}
			if to <= from {
				return "", errMDXNotFound
			}
			// Records end with a NUL that the next offset does not skip.
			return strings.TrimRight(m.decode(plain[from:to]), "\x00"), nil
		}
		cursor += b.decompSize
	}
	return "", errMDXNotFound
}

// decompressBlock strips the 8-byte block prefix and inflates the payload.
func (m *mdxFile) decompressBlock(raw []byte) ([]byte, error) {
	if len(raw) < 8 {
		return nil, fmt.Errorf("MDX 数据块不完整：%s", clip(pathBase(m.path), 40))
	}
	kind := int(binary.LittleEndian.Uint32(raw[0:4]))
	payload := raw[8:]
	switch kind {
	case mdxCompressPlain:
		return payload, nil
	case mdxCompressZlib:
		reader, err := zlib.NewReader(bytes.NewReader(payload))
		if err != nil {
			return nil, fmt.Errorf("MDX 数据块解压失败（zlib）：%s", clip(pathBase(m.path), 40))
		}
		defer reader.Close()
		plain, err := io.ReadAll(io.LimitReader(reader, 1<<30))
		if err != nil {
			return nil, fmt.Errorf("MDX 数据块解压中断：%s", clip(pathBase(m.path), 40))
		}
		return plain, nil
	case mdxCompressLZO:
		return nil, errors.New("暂不支持 LZO 压缩的 MDX 词典（engine 1.2 旧版），请转换为 zlib 版本")
	default:
		return nil, fmt.Errorf("未知的 MDX 压缩类型 %d", kind)
	}
}

// takeNumber reads one big-endian count of the width this engine version uses.
func (m *mdxFile) takeNumber(b []byte, pos *int) int64 {
	if *pos < 0 || *pos+m.numWidth > len(b) {
		return 0
	}
	var v uint64
	for i := 0; i < m.numWidth; i++ {
		v = v<<8 | uint64(b[*pos+i])
	}
	*pos += m.numWidth
	return int64(v)
}

func (m *mdxFile) takeNumber16(b []byte, pos *int) int64 {
	if *pos+2 > len(b) {
		return 0
	}
	v := int64(binary.BigEndian.Uint16(b[*pos : *pos+2]))
	*pos += 2
	return v
}

// readUnits consumes count text units and decodes them.
func (m *mdxFile) readUnits(b []byte, pos *int, count int) string {
	if count <= 0 || *pos+count*m.unit > len(b) {
		return ""
	}
	out := m.decode(b[*pos : *pos+count*m.unit])
	*pos += count * m.unit
	return strings.TrimRight(out, "\x00")
}

// readTerminated reads a NUL-terminated key, which is how headwords are stored.
func (m *mdxFile) readTerminated(b []byte, pos *int) (string, bool) {
	start := *pos
	if m.unit == 1 {
		end := bytes.IndexByte(b[start:], 0)
		if end < 0 {
			return "", false
		}
		word := m.decode(b[start : start+end])
		*pos = start + end + 1
		return word, true
	}
	for i := start; i+1 < len(b); i += 2 {
		if b[i] == 0 && b[i+1] == 0 {
			word := m.decode(b[start:i])
			*pos = i + 2
			return word, true
		}
	}
	return "", false
}

func (m *mdxFile) decode(b []byte) string {
	if m.unit == 2 {
		units := make([]uint16, 0, len(b)/2)
		for i := 0; i+1 < len(b); i += 2 {
			units = append(units, binary.LittleEndian.Uint16(b[i:i+2]))
		}
		return string(utf16.Decode(units))
	}
	return string(b)
}

// less compares headwords the way the file orders them.
func (m *mdxFile) less(a, b string) bool {
	if !m.caseSensitive {
		return strings.ToLower(a) < strings.ToLower(b)
	}
	return a < b
}

var errMDXNotFound = errors.New("MDX 词典未收录该词")

// NotFound reports a plain miss so the provider can tell it apart from a broken
// file.
func mdxIsNotFound(err error) bool { return errors.Is(err, errMDXNotFound) }

func readAt(f *os.File, offset int64, n int) ([]byte, error) {
	if n <= 0 {
		return nil, io.EOF
	}
	buf := make([]byte, n)
	read, err := f.ReadAt(buf, offset)
	if read < n {
		return buf[:read], io.ErrUnexpectedEOF
	}
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	return buf, nil
}

// decodeMDXHeaderXML turns the header string into Go text. MDict writes it as
// UTF-16LE, sometimes with a byte-order mark.
func decodeMDXHeaderXML(raw []byte) (string, error) {
	if len(raw) == 0 {
		return "", io.EOF
	}
	if len(raw) >= 2 && raw[0] == 0xFF && raw[1] == 0xFE {
		raw = raw[2:]
	}
	if looksLikeUTF16(raw) {
		units := make([]uint16, 0, len(raw)/2)
		for i := 0; i+1 < len(raw); i += 2 {
			units = append(units, binary.LittleEndian.Uint16(raw[i:i+2]))
		}
		return string(utf16.Decode(units)), nil
	}
	return string(raw), nil
}

// looksLikeUTF16 spots the ASCII-in-UTF16 pattern of an XML tag start.
func looksLikeUTF16(raw []byte) bool {
	if len(raw) < 4 || len(raw)%2 != 0 {
		return false
	}
	return raw[0] == '<' && raw[1] == 0 && raw[2] != 0
}

// mdxXMLAttrs collects name="value" pairs from the header element. The header is
// a single tag, so a plain scanner is enough and never fails on the odd
// attribute names some builders emit.
func mdxXMLAttrs(header string) map[string]string {
	attrs := map[string]string{}
	for i := 0; i < len(header); {
		if header[i] != '"' {
			i++
			continue
		}
		end := strings.IndexByte(header[i+1:], '"')
		if end < 0 {
			break
		}
		value := header[i+1 : i+1+end]
		name := mdxAttrNameBefore(header[:i])
		if name != "" {
			attrs[name] = mdxUnescapeXML(value)
		}
		i += end + 2
	}
	return attrs
}

// mdxAttrNameBefore returns the attribute name that precedes a quoted value.
// The text ending at the quote looks like ` Title=`, so the trailing '=' and
// any spacing are dropped before taking the name run.
func mdxAttrNameBefore(prefix string) string {
	end := len(prefix)
	for end > 0 {
		c := prefix[end-1]
		if c == '=' || c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			end--
			continue
		}
		break
	}
	start := end
	for start > 0 {
		c := prefix[start-1]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '<' || c == '/' {
			break
		}
		start--
	}
	return prefix[start:end]
}

func mdxUnescapeXML(s string) string {
	s = strings.ReplaceAll(s, "&quot;", "\"")
	s = strings.ReplaceAll(s, "&apos;", "'")
	s = strings.ReplaceAll(s, "&lt;", "<")
	s = strings.ReplaceAll(s, "&gt;", ">")
	s = strings.ReplaceAll(s, "&#34;", "\"")
	s = strings.ReplaceAll(s, "&#39;", "'")
	return strings.ReplaceAll(s, "&amp;", "&")
}

func mdxInt(s string, fallback int) int {
	v, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return fallback
	}
	return v
}

func mdxFloat(s string, fallback float64) float64 {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return fallback
	}
	return v
}

func mdxFirstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func pathBase(path string) string { return filepath.Base(path) }
