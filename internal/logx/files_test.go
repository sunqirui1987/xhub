package logx

import (
	"bytes"
	"go/parser"
	"go/token"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLoggerDefinesLevels 验证日志实现提供四个等级。
// 前置：读取当前源码；结果：四个等级均存在；只读检查无需清理。
func TestLoggerDefinesLevels(t *testing.T) {
	root := moduleRoot(t)
	body, err := os.ReadFile(filepath.Join(root, "internal/logx/log.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !definesLevels(string(body)) {
		t.Fatal("日志必须定义四个等级")
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

func TestRedactHidesCaseInsensitiveKeysAndURLCredentials(t *testing.T) {
	got := redact("keys SK-UPPER Sk-Mixed_123 url https://user:SK-URLSECRET@api.example.com/v1?key=SK-QUERY")
	if strings.Contains(strings.ToLower(got), "sk-") {
		t.Fatalf("redaction leaked a case-insensitive key: %s", got)
	}
	if strings.Contains(got, "user:") || strings.Contains(got, "QUERY") || strings.Contains(got, "https://") {
		t.Fatalf("redaction leaked URL credentials or full URL: %s", got)
	}
	if !strings.Contains(got, "api.example.com") {
		t.Fatalf("redaction removed useful URL host: %s", got)
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
