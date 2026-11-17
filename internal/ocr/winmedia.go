package ocr

import (
	"errors"
	"net/url"
	"strconv"
	"strings"
)

// decodeWinOCR parses the helper's result file: one record per line, fields
// separated by a pipe, free text percent-encoded so a newline in a recognized
// line cannot split it. It is separate from the provider so the contract can be
// tested on any platform.
func decodeWinOCR(raw []byte) ([]Block, error) {
	text := strings.TrimPrefix(string(raw), "\ufeff")
	if strings.TrimSpace(text) == "" {
		return nil, errors.New("Windows OCR 没有返回结果")
	}
	var blocks []Block
	sawRecord := false
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSuffix(line, "\r")
		if line == "" {
			continue
		}
		fields := strings.SplitN(line, "|", 7)
		switch fields[0] {
		case "engine":
			sawRecord = true
		case "error":
			sawRecord = true
			reason := ""
			if len(fields) > 1 {
				reason, _ = url.PathUnescape(fields[1])
			}
			if reason == "" {
				reason = "Windows OCR 返回了无法解析的结果"
			}
			return nil, errors.New(reason)
		case "block":
			sawRecord = true
			if b, ok := parseWinOCRBlock(fields); ok {
				blocks = append(blocks, b)
			}
		}
	}
	if !sawRecord {
		return nil, errors.New("Windows OCR 返回了无法解析的结果")
	}
	sortBlocksReadingOrder(blocks)
	return blocks, nil
}

func parseWinOCRBlock(fields []string) (Block, bool) {
	if len(fields) != 7 {
		return Block{}, false
	}
	var box [4]int
	for i := range box {
		n, err := strconv.Atoi(fields[i+1])
		if err != nil {
			return Block{}, false
		}
		box[i] = n
	}
	text, err := url.PathUnescape(fields[6])
	if err != nil {
		return Block{}, false
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return Block{}, false
	}
	score, _ := strconv.ParseFloat(fields[5], 64)
	return Block{
		Text:   text,
		X:      box[0],
		Y:      box[1],
		Width:  box[2],
		Height: box[3],
		Score:  score,
	}, true
}
