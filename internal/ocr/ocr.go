package ocr

import (
	"context"
	"strings"
	"time"

	"transdict/internal/netp"
)

// Block is one recognized line together with its position in the cropped image.
type Block struct {
	Text   string  `json:"text"`
	X      int     `json:"x"`
	Y      int     `json:"y"`
	Width  int     `json:"width"`
	Height int     `json:"height"`
	Score  float64 `json:"score"`
}

// Outcome carries either a successful recognition or the provider's error text.
// Errors travel as data because the UI shows every provider side by side.
type Outcome struct {
	Provider  string  `json:"provider"`
	Label     string  `json:"label"`
	Blocks    []Block `json:"blocks"`
	ElapsedMs int64   `json:"elapsedMs"`
	Error     string  `json:"error,omitempty"`
}

func (o Outcome) Text() string {
	parts := make([]string, 0, len(o.Blocks))
	for _, b := range o.Blocks {
		if t := strings.TrimSpace(b.Text); t != "" {
			parts = append(parts, t)
		}
	}
	return strings.Join(parts, "\n")
}

type Provider interface {
	Label() string
	Recognize(ctx context.Context, png []byte) ([]Block, error)
}

// RunQuery times a single provider and normalizes failure into Outcome.
func RunQuery(ctx context.Context, name, label string, p Provider, img []byte) Outcome {
	start := time.Now()
	blocks, err := p.Recognize(ctx, img)
	out := Outcome{Provider: name, Label: label, Blocks: blocks, ElapsedMs: time.Since(start).Milliseconds()}
	if err != nil {
		out.Error = netp.Reason(err)
	}
	return out
}
