package text

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode"
)

// TestNoChineseOutsideText enforces D-025: user-visible text lives only in
// internal/text. It scans every non-test Go file in the module, including
// comments, which must be in English.
func TestNoChineseOutsideText(t *testing.T) {
	root := moduleRoot(t)
	textDir := filepath.Join(root, "internal", "text")
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path == textDir || (path != root && strings.HasPrefix(d.Name(), ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		checkFile(t, root, path)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func checkFile(t *testing.T, root, path string) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		t.Errorf("parse %s: %v", path, err)
		return
	}
	rel, _ := filepath.Rel(root, path)
	report := func(pos token.Pos, s string) {
		if hasHan(s) {
			t.Errorf("%s:%d: Chinese text outside internal/text (D-025): %s", rel, fset.Position(pos).Line, s)
		}
	}
	ast.Inspect(f, func(n ast.Node) bool {
		if lit, ok := n.(*ast.BasicLit); ok && (lit.Kind == token.STRING || lit.Kind == token.CHAR) {
			report(lit.Pos(), lit.Value)
		}
		return true
	})
	for _, cg := range f.Comments {
		for _, c := range cg.List {
			report(c.Pos(), c.Text)
		}
	}
}

func hasHan(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

func moduleRoot(t *testing.T) string {
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}
