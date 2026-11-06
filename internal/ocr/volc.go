package ocr

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"transdict/internal/netp"
	"transdict/internal/settings"
)

// volc talks to 火山引擎 veOCR (视觉智能). The APIKey field carries the
// temporary AccessToken used in the Bearer header; BaseURL overrides the host
// so a test can point it at an httptest server.
type volc struct {
	APIKey  string
	BaseURL string
	httpc   *http.Client
}

// Package-level seams so tests can pin the endpoint and client timeout.
var (
	volcEndpoint = "https://open-visual-services.volcimg.com/ocr/v1/general_text"
	volcTimeout  = 30 * time.Second
)

func init() {
	Register(settings.ServiceType{
		Kind: "volc", Name: "火山引擎 veOCR",
		Note: " ve-OCR 通用文字识别 (general_text)，Bearer 令牌由控制台申请",
		Fields: []settings.Field{
			{Key: settings.FieldAPIKey, Label: "Access Token", Secret: true, Required: true},
			{Key: settings.FieldBaseURL, Label: "接口地址（可选覆盖）", Placeholder: volcEndpoint},
		},
		Default: settings.Provider{BaseURL: volcEndpoint},
	}, func(p settings.Provider) (Provider, error) {
		if p.APIKey == "" {
			return nil, fmt.Errorf("火山 veOCR 缺少 Access Token")
		}
		base := strings.TrimRight(p.BaseURL, "/")
		if base == "" {
			base = volcEndpoint
		}
		return &volc{APIKey: p.APIKey, BaseURL: base, httpc: netp.Client(volcTimeout)}, nil
	})
}

func (v *volc) Label() string { return "火山 veOCR" }

func (v *volc) Recognize(ctx context.Context, png []byte) ([]Block, error) {
	payload, err := json.Marshal(map[string]any{
		"image": base64.StdEncoding.EncodeToString(png),
		// detect_pipeline enables the layout-aware pipeline documented for
		// general_text; it costs a few ms but drops the false-positive
		// noise on decorative UI fragments.
		"req_json": map[string]any{"detect_pipeline": true},
	})
	if err != nil {
		return nil, fmt.Errorf("火山 veOCR 请求编码失败: %s", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, v.BaseURL, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+v.APIKey)

	resp, err := v.httpc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("火山 veOCR 网络错误: %s", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("火山 veOCR 读取响应失败: %s", err)
	}

	var envelope struct {
		Code       int    `json:"code"`
		Message    string `json:"message"`
		RequestID  string `json:"request_id"`
		Status     int    `json:"status"`
		TipMessage string `json:"tip_message"`
		Data       struct {
			LineTexts        []string  `json:"line_texts"`
			LineProbs        []float64 `json:"line_probs"`
			LinePolygonInfos []struct {
				P1 volcPoint `json:"point_1"`
				P2 volcPoint `json:"point_2"`
				P3 volcPoint `json:"point_3"`
				P4 volcPoint `json:"point_4"`
			} `json:"line_polygon_infos"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("火山 veOCR 响应解析失败 (HTTP %d): %s", resp.StatusCode, snippet(raw))
	}
	// Documented success sentinel: code 10000. Anything else is an error even
	// when the HTTP status is 200.
	if envelope.Code != 0 && envelope.Code != 10000 {
		return nil, vendorError("火山 veOCR", strconv.Itoa(envelope.Code),
			firstNonEmpty(envelope.Message, envelope.TipMessage), string(raw))
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("火山 veOCR HTTP %d: %s", resp.StatusCode, snippet(raw))
	}

	blocks := make([]Block, 0, len(envelope.Data.LineTexts))
	for i, txt := range envelope.Data.LineTexts {
		t := strings.TrimSpace(txt)
		if t == "" {
			continue
		}
		b := Block{Text: t}
		if i < len(envelope.Data.LinePolygonInfos) {
			p := envelope.Data.LinePolygonInfos[i]
			xs := []int{p.P1.X, p.P2.X, p.P3.X, p.P4.X}
			ys := []int{p.P1.Y, p.P2.Y, p.P3.Y, p.P4.Y}
			b.X, b.Y, b.Width, b.Height = bbox(xs, ys)
		}
		if i < len(envelope.Data.LineProbs) {
			b.Score = envelope.Data.LineProbs[i]
		}
		blocks = append(blocks, b)
	}
	sortBlocksReadingOrder(blocks)
	return blocks, nil
}

type volcPoint struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// bbox converts a set of polygon corners into the axis-aligned rect Block
// uses. Shared between the volc and youdao paths.
func bbox(xs, ys []int) (int, int, int, int) {
	if len(xs) == 0 || len(ys) == 0 {
		return 0, 0, 0, 0
	}
	minX, maxX := xs[0], xs[0]
	for _, x := range xs[1:] {
		if x < minX {
			minX = x
		}
		if x > maxX {
			maxX = x
		}
	}
	minY, maxY := ys[0], ys[0]
	for _, y := range ys[1:] {
		if y < minY {
			minY = y
		}
		if y > maxY {
			maxY = y
		}
	}
	return minX, minY, maxX - minX, maxY - minY
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}
