package ocr

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
)

// winOCRResponse is what the Windows helper answers: the recognized lines, the
// engine that read them, or the reason no engine could run. The payload travels
// base64-encoded so a console code page can never mangle the Chinese text.
type winOCRResponse struct {
	Blocks []Block `json:"blocks"`
	Engine string  `json:"engine"`
	Error  string  `json:"error"`
}

// decodeWinOCR parses the helper's stdout. It is separate from the provider so
// the payload contract can be tested on any platform.
func decodeWinOCR(raw []byte) ([]Block, error) {
	encoded := strings.TrimSpace(strings.TrimPrefix(string(raw), "\ufeff"))
	if encoded == "" {
		return nil, errors.New("Windows OCR 没有返回结果")
	}
	payload, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, errors.New("Windows OCR 返回了无法解码的内容")
	}
	var resp winOCRResponse
	if err := json.Unmarshal(payload, &resp); err != nil {
		return nil, errors.New("Windows OCR 返回了无法解析的结果")
	}
	if resp.Error != "" {
		return nil, errors.New(resp.Error)
	}
	blocks := make([]Block, 0, len(resp.Blocks))
	for _, b := range resp.Blocks {
		b.Text = strings.TrimSpace(b.Text)
		if b.Text == "" {
			continue
		}
		blocks = append(blocks, b)
	}
	sortBlocksReadingOrder(blocks)
	return blocks, nil
}
