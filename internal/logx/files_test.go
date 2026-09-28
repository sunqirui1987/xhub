package logx

import (
	"bytes"
	"go/parser"
	"go/token"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestEveryServerFileUsesLeveledLogger(t *testing.T) {
	root := moduleRoot(t)
	call := regexp.MustCompile(`\blogx\.(Trace|Debug|Info|Error)\(`)
	used := map[string]bool{}
	var missing []string
	for _, dir := range []string{"cmd", "internal"} {
		err := filepath.Walk(filepath.Join(root, dir), func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return err
			}
			body, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(root, path)
			if filepath.ToSlash(rel) == "internal/logx/log.go" {
				if !definesLevels(string(body)) {
					t.Fatalf("%s does not define trace, debug, info, and error", rel)
				}
				for _, level := range []string{"trace", "debug", "info", "error"} {
					used[level] = true
				}
				return nil
			}
			found := call.FindAllStringSubmatch(string(body), -1)
			if len(found) == 0 {
				missing = append(missing, rel)
				return nil
			}
			for _, m := range found {
				used[strings.ToLower(m[1])] = true
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("files with no leveled log call:\n%s", strings.Join(missing, "\n"))
	}
	for _, level := range []string{"trace", "debug", "info", "error"} {
		if !used[level] {
			t.Fatalf("level %s is unused", level)
		}
	}
}

func definesLevels(src string) bool {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "log.go", src, 0)
	if err != nil {
		return false
	}
	need := map[string]bool{"Trace": false, "Debug": false, "Info": false, "Error": false}
	for _, name := range []string{"Trace", "Debug", "Info", "Error"} {
		if f.Scope.Lookup(name) == nil {
			return false
		}
		need[name] = true
	}
	for _, ok := range need {
		if !ok {
			return false
		}
	}
	return strings.Contains(src, `log.Printf("%s %s"`)
}

func TestRedactHidesBearerAndKey(t *testing.T) {
	var buf bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(prev) })

	Info("header %s key %s url %s", "Bearer sk-local-master", "sk-local-master", "https://api.example.com/v1/chat?key=sk-local-master")
	line := buf.String()
	t.Log(line)
	if !strings.Contains(line, "info ") || !strings.Contains(line, "api.example.com") {
		t.Fatalf("missing info level or host: %s", line)
	}
	if strings.Contains(line, "http://") || strings.Contains(line, "https://") {
		t.Fatalf("log included a full url: %s", line)
	}
	if strings.Contains(line, "sk-local-master") || strings.Contains(strings.ToLower(line), "bearer") || strings.Contains(line, "sk-") {
		t.Fatalf("log leaked credentials: %s", line)
	}
}

func moduleRoot(t *testing.T) string {
	t.Helper()
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
