package ocr

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"transdict/internal/netp"
)

// youdaoWebEndpoint is Youdao's public image endpoint. Unlike the 智领 OCR
// provider it needs no app key: the picture is posted as a data URI and the
// response carries the recognized lines. Easydict falls back to it when Apple's
// recogniser returns nothing, and the request below keeps that shape.
var (
	youdaoWebEndpoint = "https://aidemo.youdao.com/ocrtransapi1"
	youdaoWebTimeout  = 30 * time.Second
)

type youdaoWebLine struct {
	Context     string `json:"context"`
	TranContent string `json:"tranContent"`
}

type youdaoWebPayload struct {
	LanFrom   string          `json:"lanFrom"`
	LanTo     string          `json:"lanTo"`
	ErrorCode string          `json:"errorCode"`
	Lines     []youdaoWebLine `json:"lines"`
}

// YoudaoWebOCR reads a PNG through Youdao's public endpoint. The blocks carry
// text only: the endpoint reports lines without coordinates, and the caller
// just needs what was written.
func YoudaoWebOCR(ctx context.Context, png []byte) ([]Block, error) {
	if len(png) == 0 {
		return nil, errors.New("没有可识别的图片")
	}
	form := url.Values{}
	form.Set("imgBase", "data:image/png;base64,"+base64.StdEncoding.EncodeToString(png))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, youdaoWebEndpoint,
		strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", "https://fanyi.youdao.com")
	req.Header.Set("User-Agent", "Transdict/0.1.0 (macOS)")

	resp, err := netp.Client(youdaoWebTimeout).Do(req)
	if err != nil {
		return nil, fmt.Errorf("有道 OCR 网络错误: %s", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("有道 OCR 读取响应失败: %s", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("有道 OCR HTTP %d: %s", resp.StatusCode, snippet(raw))
	}

	var payload youdaoWebPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("有道 OCR 响应解析失败: %s", snippet(raw))
	}
	if code, ok := youdaoWebErrorCode(payload.ErrorCode); ok {
		return nil, fmt.Errorf("有道 OCR 返回错误 %d: %s", code, snippet(raw))
	}

	blocks := make([]Block, 0, len(payload.Lines))
	for _, line := range payload.Lines {
		if t := strings.TrimSpace(line.Context); t != "" {
			blocks = append(blocks, Block{Text: t})
		}
	}
	if len(blocks) == 0 {
		return nil, errors.New("有道 OCR 没有识别出文字")
	}
	return blocks, nil
}

// youdaoWebErrorCode reports whether the response carries a failure. The
// endpoint answers "0" on success but sometimes omits the field entirely.
func youdaoWebErrorCode(s string) (int, bool) {
	s = strings.TrimSpace(s)
	if s == "" || s == "0" {
		return 0, false
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return -1, true
	}
	return n, true
}
