package l10n

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"unicode"
)

// The interface has two dictionaries: frontend/src/i18n.ts for the webview and
// en.go for everything Go draws — the tray, the window titles, and the error
// text the panels show. Only a test can keep Go's side in step, because a
// missing entry is silent: T returns the Chinese key and an English user reads
// Chinese.
//
// The rule is deliberately narrower than "translate every Chinese string". It
// covers a literal only where it is handed straight to the user, and only when
// it can be a dictionary key at all.

// sinkFields are the struct members that end up on screen.
var sinkFields = map[string]bool{
	"Error": true, "Label": true, "Name": true, "Note": true,
	"Description": true, "Desc": true, "Title": true, "Text": true,
	"Placeholder": true,
}

func TestEveryUserVisiblePhraseHasAnEnglishEntry(t *testing.T) {
	missing := visiblePhrases(t)
	if len(missing) == 0 {
		return
	}
	// One line per phrase, so a failure names the file to edit rather than a
	// number nobody can act on.
	sort.Strings(missing)
	t.Errorf("%d user-visible Chinese phrases have no entry in en.go:\n%s",
		len(missing), strings.Join(missing, "\n"))
}

// visiblePhrases walks the rest of internal/ and reports each phrase that Go
// puts in front of the user without an English side.
func visiblePhrases(t *testing.T) []string {
	t.Helper()

	// The tests live in internal/l10n, so the sources are one level up.
	root := filepath.Clean("..")
	fset := token.NewFileSet()

	var found []phrase
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// This package's own file is the dictionary, not a caller.
			if d.Name() == "l10n" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, perr := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if perr != nil {
			return perr
		}
		found = append(found, phrasesIn(fset, file, path)...)
		return nil
	})
	if err != nil {
		t.Fatalf("scanning %s: %v", root, err)
	}

	var missing []string
	for _, p := range found {
		if _, ok := en[p.text]; !ok {
			missing = append(missing, p.where+" "+strconv.Quote(p.text))
		}
	}
	return dedupe(missing)
}

type phrase struct {
	where string
	text  string
}

func phrasesIn(fset *token.FileSet, file *ast.File, path string) []phrase {
	var out []phrase
	seen := map[string]bool{}
	push := func(lit *ast.BasicLit) {
		text, ok := literal(lit)
		if !ok || seen[text] {
			return
		}
		seen[text] = true
		pos := fset.Position(lit.Pos())
		out = append(out, phrase{
			where: filepath.Base(path) + ":" + strconv.Itoa(pos.Line),
			text:  text,
		})
	}

	ast.Inspect(file, func(n ast.Node) bool {
		switch at := n.(type) {
		case *ast.CallExpr:
			switch fn := at.Fun.(type) {
			// errors.New and fmt.Errorf reach the user once the error is
			// stored in Result.Error.
			case *ast.SelectorExpr:
				switch fn.Sel.Name {
				case "New", "Errorf":
					if len(at.Args) == 1 {
						if lit, ok := at.Args[0].(*ast.BasicLit); ok {
							push(lit)
						}
					}
				case "T", "Zh":
					if len(at.Args) == 1 {
						if lit, ok := at.Args[0].(*ast.BasicLit); ok {
							push(lit)
						}
					}
				}
			}
		case *ast.KeyValueExpr:
			if key, ok := at.Key.(*ast.Ident); ok && sinkFields[key.Name] {
				if lit, ok := at.Value.(*ast.BasicLit); ok {
					push(lit)
				}
			}
		case *ast.AssignStmt:
			if len(at.Lhs) != 1 || len(at.Rhs) != 1 || at.Tok.String() != "=" {
				return true
			}
			lhs, ok := at.Lhs[0].(*ast.SelectorExpr)
			if !ok || !sinkFields[lhs.Sel.Name] {
				return true
			}
			if lit, ok := at.Rhs[0].(*ast.BasicLit); ok {
				push(lit)
			}
		}
		return true
	})

	// Drop everything that cannot be a key: not Chinese, or carrying a verb that
	// means the sentence is assembled at run time.
	var keep []phrase
	for _, p := range out {
		if hasHan(p.text) && !strings.Contains(p.text, "%") {
			keep = append(keep, p)
		}
	}
	return keep
}

func literal(lit *ast.BasicLit) (string, bool) {
	if lit.Kind != token.STRING {
		return "", false
	}
	s, err := strconv.Unquote(lit.Value)
	if err != nil {
		return "", false
	}
	return s, true
}

func hasHan(text string) bool {
	for _, r := range text {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

func dedupe(in []string) []string {
	out := in[:0]
	seen := map[string]bool{}
	for _, s := range in {
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}
