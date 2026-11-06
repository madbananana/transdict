package ocr

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"transdict/internal/netp"
	"transdict/internal/settings"
)

// tencent implements 腾讯云 OCR. Credentials: SecretID -> AppID, SecretKey ->
// Secret, optional region -> Extra["region"], optional action name -> Model.
//
// The signature follows the documented TC3-HMAC-SHA256 recipe:
// CanonicalRequest -> StringToSign -> key-derivation chain (date, service,
// "tc3_request"), hex-lowercase of the final HMAC. Every value that would
// otherwise make a test flaky (clock, nonce) is read through a package var so
// tests can pin it.
type tencent struct {
	SecretID  string
	SecretKey string
	Region    string
	Action    string
	Version   string
	Endpoint  string

	httpc *http.Client
}

var (
	// Package-level seams so tests can pin every nondeterministic input.
	tencentEndpoint = "https://ocr.tencentcloudapi.com"
	tencentVersion  = "2018-11-19"
	tencentAction   = "GeneralBasicOCR"
	tencentNow      = func() time.Time { return time.Unix(1700000000, 0).UTC() }
	tencentNonceFn  = func() int64 { return rand.Int63n(65535) + 1 }
	tencentTimeout  = 30 * time.Second
)

func init() {
	Register(settings.ServiceType{
		Kind: "tencent", Name: "腾讯云通用 OCR",
		Note: "通用 OCR (GeneralBasicOCR) 或高精度 OCR (GeneralAccurateOCR)，需 SecretId/SecretKey",
		Fields: []settings.Field{
			{Key: settings.FieldAppID, Label: "SecretId", Secret: true, Required: true},
			{Key: settings.FieldSecret, Label: "SecretKey", Secret: true, Required: true},
			{Key: "region", Label: "地域", Placeholder: "ap-guangzhou（默认）"},
			{Key: settings.FieldModel, Label: "接口", Placeholder: "GeneralBasicOCR / GeneralAccurateOCR"},
			{Key: settings.FieldBaseURL, Label: "接口地址（可选覆盖）", Placeholder: tencentEndpoint},
		},
	}, func(p settings.Provider) (Provider, error) {
		if p.AppID == "" || p.Secret == "" {
			return nil, fmt.Errorf("腾讯云 OCR 缺少 SecretId 或 SecretKey")
		}
		region := p.Extra["region"]
		if region == "" {
			region = "ap-guangzhou"
		}
		action := p.Model
		if action != "GeneralAccurateOCR" && action != "GeneralBasicOCR" {
			action = tencentAction
		}
		endpoint := strings.TrimRight(p.BaseURL, "/")
		if endpoint == "" {
			endpoint = tencentEndpoint
		}
		return &tencent{
			SecretID: p.AppID, SecretKey: p.Secret, Region: region,
			Action: action, Version: tencentVersion, Endpoint: endpoint,
			httpc: netp.Client(tencentTimeout),
		}, nil
	})
}

func (t *tencent) Label() string {
	return "腾讯云 OCR · " + strings.TrimPrefix(t.Action, "General")
}

func (t *tencent) Recognize(ctx context.Context, png []byte) ([]Block, error) {
	now := tencentNow()
	nonce := tencentNonceFn()
	ts := strconv.FormatInt(now.Unix(), 10)
	payload := "{}"
	if len(png) > 0 {
		body, err := json.Marshal(map[string]string{
			"ImageBase64": base64.StdEncoding.EncodeToString(png),
		})
		if err != nil {
			return nil, fmt.Errorf("腾讯云 OCR 请求编码失败: %s", err)
		}
		payload = string(body)
	}

	host := hostOf(t.Endpoint)
	auth, err := tc3Sign(t.SecretID, t.SecretKey, "ocr", host,
		now, ts, t.Action, payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.Endpoint, strings.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Host", host)
	req.Header.Set("X-TC-Action", t.Action)
	req.Header.Set("X-TC-Version", t.Version)
	req.Header.Set("X-TC-Timestamp", ts)
	req.Header.Set("X-TC-Region", t.Region)
	req.Header.Set("X-TC-Nonce", strconv.FormatInt(nonce, 10))
	req.Header.Set("Authorization", auth)

	resp, err := t.httpc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("腾讯云 OCR 网络错误: %s", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("腾讯云 OCR 读取响应失败: %s", err)
	}

	var envelope struct {
		Response struct {
			TextDetections []struct {
				DetectedText string `json:"DetectedText"`
				Confidence   int    `json:"Confidence"`
				Polygon      []struct {
					X int `json:"X"`
					Y int `json:"Y"`
				} `json:"Polygon"`
				AdvancedInfo string `json:"AdvancedInfo"`
			} `json:"TextDetections"`
			Error *struct {
				Code    string `json:"Code"`
				Message string `json:"Message"`
			} `json:"Error"`
			REQUESTID string `json:"RequestId"`
		} `json:"Response"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("腾讯云 OCR 响应解析失败 (HTTP %d): %s", resp.StatusCode, snippet(raw))
	}
	if e := envelope.Response.Error; e != nil {
		return nil, vendorError("腾讯云 OCR", e.Code, e.Message, string(raw))
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("腾讯云 OCR HTTP %d: %s", resp.StatusCode, snippet(raw))
	}

	blocks := make([]Block, 0, len(envelope.Response.TextDetections))
	for _, td := range envelope.Response.TextDetections {
		text := strings.TrimSpace(td.DetectedText)
		if text == "" {
			continue
		}
		x, y, w, h := tencentBox(td.Polygon, td.AdvancedInfo)
		blocks = append(blocks, Block{
			Text: text, X: x, Y: y, Width: w, Height: h,
			Score: float64(td.Confidence) / 100.0,
		})
	}
	sortBlocksReadingOrder(blocks)
	return blocks, nil
}

// tencentBox prefers the axis-aligned ItemPolygon/Location the vendor encodes
// inside AdvancedInfo (a JSON string) because that is exactly the shape Block
// wants; when it is absent we fall back to the 4-corner Polygon.
func tencentBox(polygon []struct {
	X int `json:"X"`
	Y int `json:"Y"`
}, advancedInfo string) (int, int, int, int) {
	if advancedInfo != "" {
		var adv struct {
			ItemPolygon *struct {
				X      int `json:"X"`
				Y      int `json:"Y"`
				Width  int `json:"Width"`
				Height int `json:"Height"`
			} `json:"ItemPolygon"`
			Location *struct {
				Left   int `json:"Left"`
				Top    int `json:"Top"`
				Width  int `json:"Width"`
				Height int `json:"Height"`
			} `json:"Location"`
		}
		if err := json.Unmarshal([]byte(advancedInfo), &adv); err == nil {
			if adv.ItemPolygon != nil {
				return adv.ItemPolygon.X, adv.ItemPolygon.Y, adv.ItemPolygon.Width, adv.ItemPolygon.Height
			}
			if adv.Location != nil {
				return adv.Location.Left, adv.Location.Top, adv.Location.Width, adv.Location.Height
			}
		}
	}
	if len(polygon) == 0 {
		return 0, 0, 0, 0
	}
	minX, minY := polygon[0].X, polygon[0].Y
	maxX, maxY := minX, minY
	for _, p := range polygon[1:] {
		if p.X < minX {
			minX = p.X
		}
		if p.X > maxX {
			maxX = p.X
		}
		if p.Y < minY {
			minY = p.Y
		}
		if p.Y > maxY {
			maxY = p.Y
		}
	}
	return minX, minY, maxX - minX, maxY - minY
}

// hostOf extracts the host header value from an absolute URL. Test servers
// return 127.0.0.1:port which is a valid Host for the canonical request.
func hostOf(endpoint string) string {
	if i := strings.Index(endpoint, "://"); i >= 0 {
		endpoint = endpoint[i+3:]
	}
	if i := strings.IndexAny(endpoint, "/?#"); i >= 0 {
		endpoint = endpoint[:i]
	}
	return endpoint
}

// tc3Sign builds the Authorization header value. Splitting it out lets tests
// assert both the deterministic signature and the canonical shape without
// needing an HTTP server. The vendor's canonical-headers rule lowercases the
// key but preserves the value verbatim (with runs of spaces collapsed), so
// "X-TC-Action: GeneralBasicOCR" contributes the pair
// "x-tc-action:GeneralBasicOCR\n".
func tc3Sign(secretID, secretKey, service, host string,
	now time.Time, timestamp, action, payload string) (string, error) {

	hashedPayload := sha256Hex([]byte(payload))

	headers := [][2]string{
		{"content-type", "application/json; charset=utf-8"},
		{"host", host},
		{"x-tc-action", strings.TrimSpace(action)},
	}
	sort.SliceStable(headers, func(i, j int) bool { return headers[i][0] < headers[j][0] })

	var canon bytes.Buffer
	for _, h := range headers {
		canon.WriteString(h[0])
		canon.WriteByte(':')
		canon.WriteString(collapseSpaces(h[1]))
		canon.WriteByte('\n')
	}
	signed := make([]string, len(headers))
	for i, h := range headers {
		signed[i] = h[0]
	}

	canonicalRequest := strings.Join([]string{
		http.MethodPost,
		"/",
		"",
		canon.String(),
		strings.Join(signed, ";"),
		hashedPayload,
	}, "\n")

	date := now.UTC().Format("2006-01-02")
	scope := date + "/" + service + "/tc3_request"
	stringToSign := strings.Join([]string{
		"TC3-HMAC-SHA256",
		timestamp,
		scope,
		sha256Hex([]byte(canonicalRequest)),
	}, "\n")

	// Key derivation chain: every step feeds the previous raw HMAC digest as
	// the key, not its hex form. The prefix "TC3" on the outermost key is
	// documented and easy to miss.
	secretDate := hmacSHA256([]byte("TC3"+secretKey), []byte(date))
	secretService := hmacSHA256(secretDate, []byte(service))
	secretSigning := hmacSHA256(secretService, []byte("tc3_request"))
	signature := hex.EncodeToString(hmacSHA256(secretSigning, []byte(stringToSign)))

	auth := fmt.Sprintf("TC3-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		secretID, scope, strings.Join(signed, ";"), signature)
	return auth, nil
}

// collapseSpaces turns any run of ASCII whitespace inside a header value into
// a single space, matching the vendor's canonicalisation rule.
func collapseSpaces(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	prevSpace := false
	for _, r := range strings.TrimSpace(s) {
		isSpace := r == ' ' || r == '\t'
		if isSpace && prevSpace {
			continue
		}
		if isSpace {
			b.WriteByte(' ')
		} else {
			b.WriteRune(r)
		}
		prevSpace = isSpace
	}
	return b.String()
}

func hmacSHA256(key, data []byte) []byte {
	m := hmac.New(sha256.New, key)
	m.Write(data)
	return m.Sum(nil)
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
