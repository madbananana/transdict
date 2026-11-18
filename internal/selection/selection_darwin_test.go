//go:build darwin

package selection

import "testing"

func TestSelectionTextDropsPlaceholders(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain words stay", "EasyDict", "EasyDict"},
		{"a picture alone is nothing", "\uFFFC\uFFFC\uFFFC", ""},
		{"placeholders beside text go", "\uFFFCword\uFFFC", "word"},
		{"empty stays empty", "", ""},
	}
	for _, c := range cases {
		if got := selectionText(c.in); got != c.want {
			t.Errorf("%s: selectionText(%q) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}
