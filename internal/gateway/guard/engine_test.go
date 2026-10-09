package guard

import (
	"encoding/json"
	"github.com/sunqirui1987/xhub/internal/config"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// configHost 为回归测试提供独立 YAML 配置和已有 KV 测试宿主。
type configHost struct {
	savedHost
	cfg *config.Config
}

// GatewayConfig 返回测试配置，供全局设置继承和 YAML 规则加载。
// 参数：h：独立测试宿主。返回：配置指针。调用：configuredRules/effectiveDefaults。测试：本文件。
func (h *configHost) GatewayConfig() *config.Config { return h.cfg }

// localRule 构造可观察的关键词规则，减少测试中重复的配置样板。
// 参数：name/action：规则名称和动作；on：默认启用；order：优先级。
// 返回：完整规则对象。调用：本文件及 external_test.go。测试：规则链回归。
func localRule(name, action string, on bool, order int) map[string]any {
	return map[string]any{"guardrail_name": name, "litellm_params": map[string]any{"guardrail": "local", "mode": "pre_call", "action": action, "blocked_words": []any{"secret"}, "default_on": on, "priority": order}}
}

// TestSelectionDefaultsAndOrdering 验证默认与显式选择取并集、去重、优先级，以及非法选择失败拦截。
// 参数：t：Go 测试上下文；本地测试宿主或 HTTP 服务提供可观察的输入/输出。
// 返回：无；协议、动作或隔离不满足预期时报告测试失败。
// 调用：go test ./internal/gateway/guard；并发相关场景同时使用 -race。
// 测试：本函数即回归用例，不依赖真实供应商凭据。
func TestSelectionDefaultsAndOrdering(t *testing.T) {
	mask := localRule("mask", "redact", true, 10)
	explicit := localRule("explicit", "block", false, 20)
	h := &configHost{cfg: &config.Config{Guardrails: []map[string]any{explicit, mask}}}
	body := map[string]any{"text": "SECRET", "guardrails": []any{"explicit", "mask", "explicit"}}
	blocked, _, findings := Evaluate(h, body)
	if blocked || len(findings) != 2 || body["text"] != "[REDACTED]" || findings[0]["guardrail_name"] != "mask" {
		t.Fatalf("chain: %v %#v %#v", blocked, body, findings)
	}
	mask["litellm_params"].(map[string]any)["action"] = "block"
	blocked, _, findings = Evaluate(h, map[string]any{"text": "secret", "guardrails": []any{}})
	if !blocked || len(findings) != 1 {
		t.Fatal("empty client selection disabled default rule")
	}
	for _, v := range []any{[]any{"missing"}, []any{map[string]any{"mask": false}}, false} {
		if blocked, _, _ := Evaluate(h, map[string]any{"text": "hello", "guardrails": v}); !blocked {
			t.Fatalf("invalid selection allowed: %#v", v)
		}
	}
}

// TestRoleInheritanceAndPositionalModification 验证全局角色跳过、单规则覆盖与文本/图片位置保护。
// 参数：t：Go 测试上下文；本地测试宿主或 HTTP 服务提供可观察的输入/输出。
// 返回：无；协议、动作或隔离不满足预期时报告测试失败。
// 调用：go test ./internal/gateway/guard；并发相关场景同时使用 -race。
// 测试：本函数即回归用例，不依赖真实供应商凭据。
func TestRoleInheritanceAndPositionalModification(t *testing.T) {
	rule := localRule("mask", "redact", true, 1)
	h := &configHost{cfg: &config.Config{Guardrails: []map[string]any{rule}, LiteLLMSettings: map[string]any{"skip_system_message_in_guardrail": true, "skip_tool_message_in_guardrail": true}}}
	system := map[string]any{"role": "system", "content": "secret"}
	tool := map[string]any{"role": "tool", "content": "secret"}
	user := map[string]any{"role": "user", "content": []any{map[string]any{"type": "text", "text": "SECRET"}, map[string]any{"type": "image_url", "image_url": map[string]any{"url": "secret.png"}}}}
	body := map[string]any{"messages": []any{system, tool, user}}
	if blocked, _, _ := Evaluate(h, body); blocked {
		t.Fatal("mask blocked")
	}
	parts := user["content"].([]any)
	if system["content"] != "secret" || tool["content"] != "secret" || parts[0].(map[string]any)["text"] != "[REDACTED]" || parts[1].(map[string]any)["image_url"].(map[string]any)["url"] != "secret.png" {
		t.Fatalf("bad mutation: %#v", body)
	}
	rule["litellm_params"].(map[string]any)["skip_system_message_in_guardrail"] = false
	Evaluate(h, body)
	if system["content"] != "[REDACTED]" {
		t.Fatal("explicit false failed to override global true")
	}
	if _, exists := configuredRules(h)[0]["litellm_params"].(map[string]any)["skip_tool_message_in_guardrail"]; exists {
		t.Fatal("global defaults leaked into editable config")
	}
}

// TestLocalRegexCannotMatchAcrossMessages 验证正则只匹配单段文本，不能跨消息制造命中。
// 参数：t：Go 测试上下文；本地测试宿主或 HTTP 服务提供可观察的输入/输出。
// 返回：无；协议、动作或隔离不满足预期时报告测试失败。
// 调用：go test ./internal/gateway/guard；并发相关场景同时使用 -race。
// 测试：本函数即回归用例，不依赖真实供应商凭据。
func TestLocalRegexCannotMatchAcrossMessages(t *testing.T) {
	rule := localRule("boundary", "block", true, 1)
	p := rule["litellm_params"].(map[string]any)
	p["blocked_words"] = []any{}
	p["patterns"] = []any{"hello world"}
	action, _, err := runRule(rule, map[string]any{"messages": []any{map[string]any{"content": "hello"}, map[string]any{"content": "world"}}})
	if err != nil || action != "allow" {
		t.Fatalf("cross-message match: %s %v", action, err)
	}
}

// TestInvalidYAMLDoesNotSilentlyDisableGuardrails 验证非法 YAML 形成默认拦截规则，不能静默丢弃。
// 参数：t：Go 测试上下文；本地测试宿主或 HTTP 服务提供可观察的输入/输出。
// 返回：无；协议、动作或隔离不满足预期时报告测试失败。
// 调用：go test ./internal/gateway/guard；并发相关场景同时使用 -race。
// 测试：本函数即回归用例，不依赖真实供应商凭据。
func TestInvalidYAMLDoesNotSilentlyDisableGuardrails(t *testing.T) {
	for _, invalid := range []map[string]any{nil, {"invalid": func() {}}} {
		h := &configHost{cfg: &config.Config{Guardrails: []map[string]any{invalid}}}
		if blocked, _, _ := Evaluate(h, map[string]any{"text": "hello"}); !blocked {
			t.Fatal("unreadable YAML skipped")
		}
	}
	rule := localRule("bad", "block", true, 1)
	rule["litellm_params"].(map[string]any)["mode"] = "post_call"
	h := &configHost{cfg: &config.Config{Guardrails: []map[string]any{rule}}}
	if blocked, _, _ := Evaluate(h, map[string]any{"text": "hello"}); !blocked {
		t.Fatal("unsupported YAML silently skipped")
	}
}

// TestManageSubmissionsContractAndCRUD 验证提交列表稳定空合约以及管理创建/更新/删除流程。
// 参数：t：Go 测试上下文；本地测试宿主或 HTTP 服务提供可观察的输入/输出。
// 返回：无；协议、动作或隔离不满足预期时报告测试失败。
// 调用：go test ./internal/gateway/guard；并发相关场景同时使用 -race。
// 测试：本函数即回归用例，不依赖真实供应商凭据。
func TestManageSubmissionsContractAndCRUD(t *testing.T) {
	h := &savedHost{store: openGuardStore(t), allow: true}
	call := func(method, path string, body any) (int, map[string]any) {
		raw, _ := json.Marshal(body)
		rec := httptest.NewRecorder()
		if !Manage(h, rec, httptest.NewRequest(method, path, strings.NewReader(string(raw)))) {
			t.Fatalf("unhandled %s", path)
		}
		var value map[string]any
		json.Unmarshal(rec.Body.Bytes(), &value)
		return rec.Code, value
	}
	status, res := call(http.MethodGet, "/guardrails/submissions", nil)
	if status != 200 || res["supported"] != false || len(res["submissions"].([]any)) != 0 || res["summary"] == nil {
		t.Fatalf("contract: %#v", res)
	}
	status, res = call(http.MethodPost, "/guardrails", map[string]any{"guardrail": localRule("saved", "block", false, 1)})
	if status != 200 {
		t.Fatalf("create: %d %#v", status, res)
	}
	id := res["guardrail_id"].(string)
	if status, _ := call(http.MethodPost, "/guardrails", localRule("saved", "block", true, 1)); status != 409 {
		t.Fatalf("duplicate: %d", status)
	}
	status, res = call(http.MethodPut, "/guardrails/"+id, map[string]any{"litellm_params": map[string]any{"action": "redact", "skip_system_message_in_guardrail": true}})
	if status != 200 || res["litellm_params"].(map[string]any)["action"] != "redact" {
		t.Fatalf("update: %#v", res)
	}
	status, res = call(http.MethodPut, "/guardrails/"+id, map[string]any{"litellm_params": map[string]any{"skip_system_message_in_guardrail": nil}})
	if _, yes := res["litellm_params"].(map[string]any)["skip_system_message_in_guardrail"]; yes {
		t.Fatal("inherit not restored")
	}
	if status, _ := call(http.MethodDelete, "/guardrails/"+id, nil); status != 200 {
		t.Fatalf("delete: %d", status)
	}
}

// TestXGoFailureAndMetadataIsolation 验证运行错误阻止请求，脚本修改 metadata 不污染真实正文。
// 参数：t：Go 测试上下文；本地测试宿主或 HTTP 服务提供可观察的输入/输出。
// 返回：无；协议、动作或隔离不满足预期时报告测试失败。
// 调用：go test ./internal/gateway/guard；并发相关场景同时使用 -race。
// 测试：本函数即回归用例，不依赖真实供应商凭据。
func TestXGoFailureAndMetadataIsolation(t *testing.T) {
	for _, statement := range []string{`return map[string]any{"action":"bogus"}`, `return Modify([]string{})`, `panic("bad script")`, `return Modify([]string{RegexReplace(texts[0], "[", "x")})`} {
		code := `import . "xhub/guardrail"
func ApplyGuardrail(texts []string, requestData map[string]any, inputType string) map[string]any { ` + statement + ` }`
		rule := map[string]any{"guardrail_name": "script", "litellm_params": map[string]any{"guardrail": "custom_code", "custom_code": code, "default_on": true}}
		h := &configHost{cfg: &config.Config{Guardrails: []map[string]any{rule}}}
		if blocked, _, _ := Evaluate(h, map[string]any{"text": "hello"}); !blocked {
			t.Fatalf("bad script allowed: %s", statement)
		}
	}
	code := `import . "xhub/guardrail"
func ApplyGuardrail(texts []string, requestData map[string]any, inputType string) map[string]any {
 requestData["metadata"].(map[string]any)["original"] = "changed"
 return Allow()
}`
	body := map[string]any{"metadata": map[string]any{"original": "kept"}}
	if _, _, _, err := runCustom(code, []string{"hello"}, body, "request"); err != nil {
		t.Fatal(err)
	}
	if body["metadata"].(map[string]any)["original"] != "kept" {
		t.Fatal("script mutated live metadata")
	}
}

// TestXGoConcurrentInvocationsHaveFreshGlobals 验证并发请求使用新解释器，脚本全局变量不会跨请求共享。
// 参数：t：Go 测试上下文；本地测试宿主或 HTTP 服务提供可观察的输入/输出。
// 返回：无；协议、动作或隔离不满足预期时报告测试失败。
// 调用：go test ./internal/gateway/guard；并发相关场景同时使用 -race。
// 测试：本函数即回归用例，不依赖真实供应商凭据。
func TestXGoConcurrentInvocationsHaveFreshGlobals(t *testing.T) {
	code := `import . "xhub/guardrail"
var count int
func ApplyGuardrail(texts []string, requestData map[string]any, inputType string) map[string]any {
 count++
 if count != 1 { return Block("shared global") }
 return Allow()
}`
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			a, _, _, err := runCustom(code, []string{"ok"}, nil, "request")
			if err != nil || a != "allow" {
				t.Errorf("isolation: %s %v", a, err)
			}
		}()
	}
	wg.Wait()
}

// TestModificationPreservesSkippedPositionsAndStringInputs 验证文本修改保持空位置、字符串输入及被跳过消息位置。
// 参数：t：Go 测试上下文；本地测试宿主或 HTTP 服务提供可观察的输入/输出。
// 返回：无；协议、动作或隔离不满足预期时报告测试失败。
// 调用：go test ./internal/gateway/guard；并发相关场景同时使用 -race。
// 测试：本函数即回归用例，不依赖真实供应商凭据。
func TestModificationPreservesSkippedPositionsAndStringInputs(t *testing.T) {
	rule := localRule("mask", "redact", true, 1)
	p := rule["litellm_params"].(map[string]any)
	p["skip_system_message_in_guardrail"] = true
	skipped := map[string]any{"role": "system", "content": "secret"}
	body := map[string]any{"input": []any{skipped, "secret", "", "normal"}}
	if action, _, err := runRule(rule, body); err != nil || action != "redact" {
		t.Fatalf("result: %s %v", action, err)
	}
	rows := body["input"].([]any)
	if len(rows) != 4 || skipped["content"] != "secret" || rows[1] != "[REDACTED]" || rows[2] != "" || rows[3] != "normal" {
		t.Fatalf("lost input positions: %#v", body)
	}
}

// TestDuplicateNamesAcrossYAMLAndStoreFailClosed 验证 YAML 和 KV 名称歧义默认拦截。
// 参数：t：Go 测试上下文；本地测试宿主或 HTTP 服务提供可观察的输入/输出。
// 返回：无；协议、动作或隔离不满足预期时报告测试失败。
// 调用：go test ./internal/gateway/guard；并发相关场景同时使用 -race。
// 测试：本函数即回归用例，不依赖真实供应商凭据。
func TestDuplicateNamesAcrossYAMLAndStoreFailClosed(t *testing.T) {
	st := openGuardStore(t)
	putGuardrail(t, st, "db-id", localRule("same", "block", false, 1))
	h := &configHost{savedHost: savedHost{store: st}, cfg: &config.Config{Guardrails: []map[string]any{localRule("same", "redact", false, 2)}}}
	if blocked, _, _ := Evaluate(h, map[string]any{"text": "hello"}); !blocked {
		t.Fatal("ambiguous configuration allowed")
	}
}
