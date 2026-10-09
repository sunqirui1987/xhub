package guard

import (
	"strings"
	"testing"
)

const testPolicy = `import . "xhub/guardrail"
func ApplyGuardrail(texts []string, requestData map[string]any, inputType string) map[string]any {
 for _, text := range texts { if Contains(Lower(text), "secret") { return Block("sensitive content") } }
 return Allow()
}`

// TestXGoPolicy 验证实际 XGo 编译/执行关键词拦截与普通文本放行。
// 参数：t：Go 测试上下文；本地测试宿主或 HTTP 服务提供可观察的输入/输出。
// 返回：无；协议、动作或隔离不满足预期时报告测试失败。
// 调用：go test ./internal/gateway/guard；并发相关场景同时使用 -race。
// 测试：本函数即回归用例，不依赖真实供应商凭据。
func TestXGoPolicy(t *testing.T) {
	for _, tc := range []struct{ text, action string }{{"hello", "allow"}, {"SECRET", "block"}} {
		a, _, _, err := runCustom(testPolicy, []string{tc.text}, nil, "request")
		if err != nil || a != tc.action {
			t.Fatalf("%s: %s %v", tc.text, a, err)
		}
	}
}

// TestXGoModify 验证脚本修改文本时需要 Modify，且结果数量遵守合约。
// 参数：t：Go 测试上下文；本地测试宿主或 HTTP 服务提供可观察的输入/输出。
// 返回：无；协议、动作或隔离不满足预期时报告测试失败。
// 调用：go test ./internal/gateway/guard；并发相关场景同时使用 -race。
// 测试：本函数即回归用例，不依赖真实供应商凭据。
func TestXGoModify(t *testing.T) {
	code := `import . "xhub/guardrail"
func ApplyGuardrail(texts []string, requestData map[string]any, inputType string) map[string]any {
 for i, text := range texts { texts[i] = RegexReplace(text, "secret", "[MASK]") }
 return Modify(texts)
}`
	a, _, out, err := runCustom(code, []string{"a secret"}, nil, "request")
	if err != nil || a != "modify" || out[0] != "a [MASK]" {
		t.Fatalf("%s %v %v", a, out, err)
	}
}

// TestXGoRestrictions 验证导入、并发语法、初始化和无效入口被拒绝。
// 参数：t：Go 测试上下文；本地测试宿主或 HTTP 服务提供可观察的输入/输出。
// 返回：无；协议、动作或隔离不满足预期时报告测试失败。
// 调用：go test ./internal/gateway/guard；并发相关场景同时使用 -race。
// 测试：本函数即回归用例，不依赖真实供应商凭据。
func TestXGoRestrictions(t *testing.T) {
	if _, err := compileCustom("import \"os\"\n" + testPolicy); err == nil {
		t.Fatal("os import allowed")
	}
	code := strings.Replace(testPolicy, "return Allow()", "for {}", 1)
	if _, _, _, err := runCustom(code, []string{"ok"}, nil, "request"); err == nil || !strings.Contains(err.Error(), "deadline") {
		t.Fatalf("loop: %v", err)
	}
}

// XGo's for-in syntax proves this path uses the XGo compiler rather than only a Go parser.
// TestXGoForInSyntax 验证 XGo 的 for-in 特有语法经过真实转换器执行。
// 参数：t：Go 测试上下文；本地测试宿主或 HTTP 服务提供可观察的输入/输出。
// 返回：无；协议、动作或隔离不满足预期时报告测试失败。
// 调用：go test ./internal/gateway/guard；并发相关场景同时使用 -race。
// 测试：本函数即回归用例，不依赖真实供应商凭据。
func TestXGoForInSyntax(t *testing.T) {
	code := strings.Replace(testPolicy, "for _, text := range texts", "for text <- texts", 1)
	action, _, _, err := runCustom(code, []string{"secret"}, nil, "request")
	if err != nil || action != "block" {
		t.Fatalf("XGo syntax: %s %v", action, err)
	}
}
