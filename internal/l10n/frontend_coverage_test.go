package l10n

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// The webview has its own dictionary — frontend/src/i18n.ts, keyed by the
// Chinese literal — and it is the authority for everything drawn inside the
// window. A t('…') call whose text is missing there fails silently in English
// mode, which is how a fresh hint in the settings page slipped through.
//
// Same shape as TestEveryUserVisiblePhraseHasAnEnglishEntry, but for the other
// half of the interface: it only checks a literal that is passed straight to t()
// as a single-quote string, because anything built from a variable has no key to
// look up here.

// tsKeyLine is one row of the en table, in both the `'k': 'v',` form and the
// `'k':` / `'v',` split across two lines.
var tsKeyLine = regexp.MustCompile(`(?m)^[ \t]*'((?:[^'\\]|\\.)*)'[ \t]*:`)

// tsCall is a t('literal') — optionally with a vars object after it.
var tsCall = regexp.MustCompile(`\bt\('((?:[^'\\]|\\.)*)'[ \t]*[,)]`)

func TestEveryWebviewPhraseHasAnEnglishEntry(t *testing.T) {
	// This package lives in internal/l10n, so the sources are two levels up.
	dir := filepath.Join("..", "..", "frontend", "src")

	keys, err := tableKeys(filepath.Join(dir, "i18n.ts"))
	if err != nil {
		t.Fatalf("reading the dictionary: %v", err)
	}
	if len(keys) == 0 {
		t.Fatal("no keys parsed from i18n.ts — the test has gone blind")
	}

	var missing []string
	sources, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("listing %s: %v", dir, err)
	}
	for _, entry := range sources {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".ts") && !strings.HasSuffix(entry.Name(), ".tsx") {
			continue
		}
		if entry.Name() == "i18n.ts" {
			continue
		}
		text, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			t.Fatalf("reading %s: %v", entry.Name(), err)
		}
		src := string(text)
		for _, loc := range tsCall.FindAllStringSubmatchIndex(src, -1) {
			phrase := unquote(src[loc[2]:loc[3]])
			// Comments and identifiers that happen to contain 中文 are not keys;
			// neither is a phrase so generic that falling back to Chinese is fine.
			if _, ok := keys[phrase]; ok || !hasHan(phrase) {
				continue
			}
			line := 1 + strings.Count(src[:loc[0]], "\n")
			missing = append(missing, entry.Name()+":"+strconv.Itoa(line)+" "+strconv.Quote(phrase))
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		t.Errorf("%d webview phrases have no entry in frontend/src/i18n.ts:\n%s",
			len(missing), strings.Join(dedupe(missing), "\n"))
	}
}

func tableKeys(path string) (map[string]bool, error) {
	text, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, m := range tsKeyLine.FindAllStringSubmatch(string(text), -1) {
		out[unquote(m[1])] = true
	}
	return out, nil
}

func unquote(s string) string {
	r := strings.NewReplacer(`\'`, `'`, `\"`, `"`, `\\`, `\`)
	return r.Replace(s)
}
