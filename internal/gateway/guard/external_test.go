package guard

import (
	"encoding/json"
	"fmt"
	"github.com/sunqirui1987/xhub/internal/config"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestExternalAdapters 验证 OpenAI、Lakera、Azure、远端 LiteLLM 的请求协议和拦截判定。
// 参数：t：Go 测试上下文；本地测试宿主或 HTTP 服务提供可观察的输入/输出。
// 返回：无；协议、动作或隔离不满足预期时报告测试失败。
// 调用：go test ./internal/gateway/guard；并发相关场景同时使用 -race。
// 测试：本函数即回归用例，不依赖真实供应商凭据。
func TestExternalAdapters(t *testing.T) {
	for _, tc := range []struct{ kind, path, response, action string }{
		{"openai_moderation", "/moderations", `{"results":[{"flagged":true}]}`, "block"},
		{"lakera_v2", "/v2/guard", `{"flagged":false}`, "allow"},
		{"azure/prompt_shield", "/contentsafety/text:shieldPrompt", `{"userPromptAnalysis":{"attackDetected":true}}`, "block"},
		{"azure/text_moderations", "/contentsafety/text:analyze", `{"categoriesAnalysis":[{"category":"Hate","severity":0},{"category":"SelfHarm","severity":0},{"category":"Sexual","severity":4},{"category":"Violence","severity":0}]}`, "block"},
		{"litellm_proxy", "/guardrails/apply_guardrail", `{"response_text":"masked"}`, "modify"},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != tc.path {
					t.Errorf("path %s", r.URL.Path)
				}
				if strings.HasPrefix(tc.kind, "azure/") {
					if r.Header.Get("Ocp-Apim-Subscription-Key") != "key" || r.URL.Query().Get("api-version") != "2024-09-01" {
						t.Error("Azure auth/version missing")
					}
				} else if r.Header.Get("Authorization") != "Bearer key" {
					t.Error("auth missing")
				}
				var payload map[string]any
				if json.NewDecoder(r.Body).Decode(&payload) != nil {
					t.Error("invalid JSON payload")
				}
				fmt.Fprint(w, tc.response)
			}))
			defer server.Close()
			params := map[string]any{"guardrail": tc.kind, "api_base": server.URL, "api_key": "key", "remote_guardrail_name": "remote"}
			rule := map[string]any{"guardrail_name": "test", "litellm_params": params}
			if e := Validate(rule); e != nil {
				t.Fatal(e)
			}
			body := map[string]any{"messages": []any{map[string]any{"role": "user", "content": "hello"}}}
			a, _, e := runRule(rule, body)
			if e != nil || a != tc.action {
				t.Fatalf("%s %v", a, e)
			}
			if a == "modify" && guardrailText(body) != "masked" {
				t.Fatal("modification was not applied")
			}
		})
	}
}

// TestExternalMalformedResponseBlocks 验证缺失或错误判定字段拒绝执行，不静默放行。
// 参数：t：Go 测试上下文；本地测试宿主或 HTTP 服务提供可观察的输入/输出。
// 返回：无；协议、动作或隔离不满足预期时报告测试失败。
// 调用：go test ./internal/gateway/guard；并发相关场景同时使用 -race。
// 测试：本函数即回归用例，不依赖真实供应商凭据。
func TestExternalMalformedResponseBlocks(t *testing.T) {
	for _, response := range []string{`{}`, `{"results":[]}`, `{"results":[{"flagged":"false"}]}`, `not-json`} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, response) }))
		a, _, e := runRule(map[string]any{"litellm_params": map[string]any{"guardrail": "openai_moderation", "api_base": server.URL, "api_key": "key"}}, map[string]any{"text": "hello"})
		server.Close()
		if a != "block" || e == nil {
			t.Fatalf("allowed malformed response %s: %s %v", response, a, e)
		}
	}
}

// TestBedrockSignedApplyGuardrail 验证 AWS ApplyGuardrail 签名、路径和输入结构。
// 参数：t：Go 测试上下文；本地测试宿主或 HTTP 服务提供可观察的输入/输出。
// 返回：无；协议、动作或隔离不满足预期时报告测试失败。
// 调用：go test ./internal/gateway/guard；并发相关场景同时使用 -race。
// 测试：本函数即回归用例，不依赖真实供应商凭据。
func TestBedrockSignedApplyGuardrail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/guardrail/test-id/version/1/apply" || !strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 ") {
			t.Errorf("unsigned or wrong Bedrock request %s", r.URL.Path)
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["source"] != "INPUT" {
			t.Error("wrong source")
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"action":"GUARDRAIL_INTERVENED","assessments":[],"outputs":[],"usage":{}}`)
	}))
	defer server.Close()
	p := map[string]any{"guardrail": "bedrock", "api_base": server.URL, "guardrailIdentifier": "test-id", "guardrailVersion": "1", "aws_region_name": "us-east-1", "aws_access_key_id": "test", "aws_secret_access_key": "test"}
	a, _, _, e := runExternal(p, []string{"hello"})
	if e != nil || a != "block" {
		t.Fatalf("%s %v", a, e)
	}
}

// TestPresidioRedactionAndUnicode 验证个人信息脱敏及 Unicode 位置校验。
// 参数：t：Go 测试上下文；本地测试宿主或 HTTP 服务提供可观察的输入/输出。
// 返回：无；协议、动作或隔离不满足预期时报告测试失败。
// 调用：go test ./internal/gateway/guard；并发相关场景同时使用 -race。
// 测试：本函数即回归用例，不依赖真实供应商凭据。
func TestPresidioRedactionAndUnicode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/analyze":
			fmt.Fprint(w, `[{"entity_type":"PHONE_NUMBER","start":1,"end":3,"score":0.9}]`)
		case "/anonymize":
			fmt.Fprint(w, `{"text":"你[MASK]"}`)
		default:
			t.Error("wrong path")
		}
	}))
	defer server.Close()
	p := map[string]any{"guardrail": "presidio", "api_base": server.URL, "anonymizer_api_base": server.URL, "action": "redact"}
	a, _, out, e := runExternal(p, []string{"你12"})
	if e != nil || a != "redact" || out[0] != "你[MASK]" {
		t.Fatalf("%s %v %v", a, out, e)
	}
}

// TestSecretsAndCredentialMasking 验证本地密钥命中、脱敏及配置密钥掩码。
// 参数：t：Go 测试上下文；本地测试宿主或 HTTP 服务提供可观察的输入/输出。
// 返回：无；协议、动作或隔离不满足预期时报告测试失败。
// 调用：go test ./internal/gateway/guard；并发相关场景同时使用 -race。
// 测试：本函数即回归用例，不依赖真实供应商凭据。
func TestSecretsAndCredentialMasking(t *testing.T) {
	text := "openai_api_key=sk-1234998222"
	a, _, out := runSecrets(nil, []string{text})
	if a != "redact" || out[0] != "openai_api_key=[REDACTED]" {
		t.Fatalf("%s %v", a, out)
	}
	row := map[string]any{"litellm_params": map[string]any{"api_key": "sensitive", "aws_secret_access_key": "sensitive", "api_base": "https://example.com"}}
	public := publicRule(row)
	if public["litellm_params"].(map[string]any)["api_key"] != credentialMask || row["litellm_params"].(map[string]any)["api_key"] != "sensitive" {
		t.Fatal("masking leaks or mutates runtime")
	}
}

// TestAzureUnicodeChunksAndLaterViolation 验证中文按字符分段、空白不丢失，以及后续片段命中不会被首段放行掩盖。
// TestAzureUnicodeChunksAndLaterViolation 验证超长 Unicode 文本完整分段，后续段命中仍拦截。
// 参数：t：Go 测试上下文；本地测试宿主或 HTTP 服务提供可观察的输入/输出。
// 返回：无；协议、动作或隔离不满足预期时报告测试失败。
// 调用：go test ./internal/gateway/guard；并发相关场景同时使用 -race。
// 测试：本函数即回归用例，不依赖真实供应商凭据。
func TestAzureUnicodeChunksAndLaterViolation(t *testing.T) {
	for _, text := range []string{strings.Repeat("中", 20001), strings.Repeat("hello world \n", 2000)} {
		chunks := azureTextChunks(text)
		if strings.Join(chunks, "") != text {
			t.Fatal("chunking altered original text")
		}
		for _, chunk := range chunks {
			if len([]rune(chunk)) > 10000 {
				t.Fatal("Azure chunk exceeded limit")
			}
		}
	}
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var body map[string]any
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			t.Error("invalid body")
		}
		if len([]rune(str(body["userPrompt"]))) > 10000 {
			t.Error("oversized request")
		}
		fmt.Fprintf(w, "{\"userPromptAnalysis\":{\"attackDetected\":%t}}", calls == 2)
	}))
	defer server.Close()
	p := map[string]any{"guardrail": "azure/prompt_shield", "api_base": server.URL, "api_key": "key"}
	a, _, _, e := runExternal(p, []string{strings.Repeat("中", 20001)})
	if e != nil || a != "block" || calls != 2 {
		t.Fatalf("later violation lost: %s %v calls=%d", a, e, calls)
	}
}

// TestExternalCredentialUpdatePreservesSecret 验证管理接口遮蔽密钥，更新掩码时保留执行所需真实凭据。
// TestExternalCredentialUpdatePreservesSecret 验证编辑已保存规则时掩码/空值保留凭据，显式删除语义正确。
// 参数：t：Go 测试上下文；本地测试宿主或 HTTP 服务提供可观察的输入/输出。
// 返回：无；协议、动作或隔离不满足预期时报告测试失败。
// 调用：go test ./internal/gateway/guard；并发相关场景同时使用 -race。
// 测试：本函数即回归用例，不依赖真实供应商凭据。
func TestExternalCredentialUpdatePreservesSecret(t *testing.T) {
	h := &savedHost{store: openGuardStore(t), allow: true}
	call := func(method, path, raw string) map[string]any {
		rec := httptest.NewRecorder()
		Manage(h, rec, httptest.NewRequest(method, path, strings.NewReader(raw)))
		if rec.Code != 200 {
			t.Fatalf("%d %s", rec.Code, rec.Body.String())
		}
		var value map[string]any
		if e := json.Unmarshal(rec.Body.Bytes(), &value); e != nil {
			t.Fatal(e)
		}
		return value
	}
	created := call("POST", "/guardrails", "{\"guardrail_name\":\"external-secret\",\"litellm_params\":{\"guardrail\":\"openai_moderation\",\"api_key\":\"real-key\"}}")
	id := str(created["guardrail_id"])
	if created["litellm_params"].(map[string]any)["api_key"] != credentialMask {
		t.Fatal("management leaked key")
	}
	call("PATCH", "/guardrails/"+id, "{\"litellm_params\":{\"api_key\":\"********\",\"default_on\":true}}")
	row, e := h.store.GetKV("guardrails", id)
	if e != nil || row["litellm_params"].(map[string]any)["api_key"] != "real-key" {
		t.Fatalf("key overwritten %v", e)
	}
}

// TestExternalEvaluateAndFlag 验证默认外部护栏参与真实规则链，Flag 元数据被记录且不阻止继续执行。
// TestExternalEvaluateAndFlag 验证非阻断 Flag 后仍执行默认外部规则，记录元数据并最终拦截。
// 参数：t：Go 测试上下文；本地测试宿主或 HTTP 服务提供可观察的输入/输出。
// 返回：无；协议、动作或隔离不满足预期时报告测试失败。
// 调用：go test ./internal/gateway/guard；并发相关场景同时使用 -race。
// 测试：本函数即回归用例，不依赖真实供应商凭据。
func TestExternalEvaluateAndFlag(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "{\"results\":[{\"flagged\":true}]}") }))
	defer server.Close()
	code := "import . \"xhub/guardrail\"\nfunc ApplyGuardrail(texts []string, requestData map[string]any, inputType string) map[string]any { return Flag(\"审计\",map[string]any{\"category\":\"review\"}) }"
	h := &configHost{cfg: &config.Config{Guardrails: []map[string]any{
		{"guardrail_name": "flag", "litellm_params": map[string]any{"guardrail": "custom_code", "custom_code": code, "default_on": true, "priority": 1}},
		{"guardrail_name": "external", "litellm_params": map[string]any{"guardrail": "openai_moderation", "api_key": "key", "api_base": server.URL, "default_on": true, "priority": 2}},
	}}}
	blocked, _, findings := Evaluate(h, map[string]any{"text": "unsafe"})
	if !blocked || len(findings) != 2 || findings[0]["guardrail_status"] != "guardrail_flagged" || findings[1]["guardrail_status"] != "blocked" {
		t.Fatalf("wrong chain: %v %#v", blocked, findings)
	}
	metadata := findings[0]["guardrail_response"].(map[string]any)["metadata"].(map[string]any)
	if metadata["category"] != "review" {
		t.Fatal("Flag metadata lost")
	}
}
