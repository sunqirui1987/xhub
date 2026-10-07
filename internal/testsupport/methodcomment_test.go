package testsupport

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestProductionMethodsDocumentParameters walks non-test Go under internal and
// fails when a function or interface method has no immediately attached comment
// with a purpose sentence plus 参数, 返回, 调用, and 测试 lines.
func TestProductionMethodsDocumentParameters(t *testing.T) {
	root := moduleInternal(t)
	fset := token.NewFileSet()
	missing := 0
	total := 0
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			t.Errorf("parse %s: %v", path, err)
			return nil
		}
		check := func(doc *ast.CommentGroup, name string, pos token.Pos) {
			total++
			text := ""
			if doc != nil {
				text = doc.Text()
			}
			ok := hasPurpose(text) &&
				strings.Contains(text, "参数") &&
				strings.Contains(text, "返回") &&
				hasCallerLine(text) &&
				strings.Contains(text, "测试")
			if !ok {
				missing++
				if missing <= 400 {
					t.Errorf("missing method comment %s:%d %s", path, fset.Position(pos).Line, name)
				}
			}
		}
		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if ok && fn.Name != nil {
				check(fn.Doc, fn.Name.Name, fn.Pos())
			}
		}
		for _, d := range f.Decls {
			gd, ok := d.(*ast.GenDecl)
			if !ok || gd.Tok != token.TYPE {
				continue
			}
			for _, spec := range gd.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				it, ok := ts.Type.(*ast.InterfaceType)
				if !ok || it.Methods == nil {
					continue
				}
				for _, field := range it.Methods.List {
					if _, ok := field.Type.(*ast.FuncType); !ok || len(field.Names) == 0 {
						continue
					}
					for _, n := range field.Names {
						check(field.Doc, n.Name, n.Pos())
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("method comments total=%d missing=%d", total, missing)
	if missing != 0 {
		t.Fatalf("missing=%d", missing)
	}
}

// hasPurpose reports whether the doc has a sentence that is not a 参数, 返回, 调用, or 测试 line.
// A structured return line starts with 返回： or 返回 plus a result name. A sentence that merely begins with 返回 is still a purpose.
func hasPurpose(text string) bool {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || isCommentMarker(line) {
			continue
		}
		return true
	}
	return false
}

// hasCallerLine requires a line that starts with 调用：. The substring 调用方 does not count.
func hasCallerLine(text string) bool {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "调用：") || strings.HasPrefix(line, "调用:") {
			return true
		}
	}
	return false
}

func isCommentMarker(line string) bool {
	if strings.HasPrefix(line, "参数") || strings.HasPrefix(line, "调用") || strings.HasPrefix(line, "测试") {
		return true
	}
	if !strings.HasPrefix(line, "返回") {
		return false
	}
	rest := strings.TrimPrefix(line, "返回")
	if rest == "" {
		return true
	}
	r := []rune(rest)[0]
	return r == '：' || r == ':' || r == ' ' || r == '*' || r == '[' || (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z')
}

func moduleInternal(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return filepath.Join(dir, "internal")
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}
