package query

import "testing"

// The screenshot overlay is handed a JPEG preview while every crop stays PNG;
// the data URL has to name what the bytes actually are, because WebKit only
// tolerates a mislabelled image by sniffing.
func TestDataURLNamesTheImageType(t *testing.T) {
	jpeg := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10}
	png := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}
	cases := []struct {
		name  string
		image []byte
		kind  string
	}{
		{"jpeg", jpeg, "image/jpeg"},
		{"png", png, "image/png"},
		{"empty", nil, ""},
	}
	for _, tc := range cases {
		got := DataURL(tc.image)
		if tc.kind == "" {
			if got != "" {
				t.Errorf("%s: 想得到空串，得到 %q", tc.name, got)
			}
			continue
		}
		prefix := "data:" + tc.kind + ";base64,"
		if len(got) < len(prefix) || got[:len(prefix)] != prefix {
			t.Errorf("%s: 想得到前缀 %q，得到 %q", tc.name, prefix, got)
		}
	}
}
