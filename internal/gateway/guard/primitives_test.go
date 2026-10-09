package guard

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestXGoNetworkAndJSON 验证 JSON 往返、带认证的 HTTP POST 以及 Flag 元数据，原文继续放行。
// 参数：t：Go 测试上下文；本地测试宿主或 HTTP 服务提供可观察的输入/输出。
// 返回：无；协议、动作或隔离不满足预期时报告测试失败。
// 调用：go test ./internal/gateway/guard；并发相关场景同时使用 -race。
// 测试：本函数即回归用例，不依赖真实供应商凭据。
func TestXGoNetworkAndJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("invalid request: %s", r.Method)
		}
		var body map[string]any
		if json.NewDecoder(r.Body).Decode(&body) != nil || body["text"] != "hello" {
			t.Error("missing body")
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"flagged":true}`)
	}))
	defer server.Close()
	code := fmt.Sprintf(`import . "xhub/guardrail"
 func ApplyGuardrail(texts []string, requestData map[string]any, inputType string) map[string]any {
  body := JSONParse(JSONStringify(map[string]any{"text":texts[0]}))
  response := HTTPPost(%q,body,map[string]string{"Authorization":"Bearer test-key"},2)
  if response["success"] != true { return Block("HTTP failed") }
  parsed := response["body"].(map[string]any)
  if parsed["flagged"] == true { return Flag("review", map[string]any{"source":"external"}) }
  return Allow()
 }`, server.URL)
	action, reason, out, metadata, err := runCustomDetailed(code, []string{"hello"}, nil, "request")
	if err != nil || action != "flag" || reason != "review" || out[0] != "hello" || metadata["source"] != "external" {
		t.Fatalf("%s %s %v %v %v", action, reason, out, metadata, err)
	}
}

// TestXGoLLMAndNamedImport 验证别名导入的 LLMChat 路径、请求模型和认证信息。
// 参数：t：Go 测试上下文；本地测试宿主或 HTTP 服务提供可观察的输入/输出。
// 返回：无；协议、动作或隔离不满足预期时报告测试失败。
// 调用：go test ./internal/gateway/guard；并发相关场景同时使用 -race。
// 测试：本函数即回归用例，不依赖真实供应商凭据。
func TestXGoLLMAndNamedImport(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer llm-key" {
			t.Errorf("invalid LLM request")
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["model"] != "judge" || body["stream"] != false {
			t.Error("missing model or stream")
		}
		fmt.Fprint(w, `{"choices":[{"message":{"content":"safe"}}]}`)
	}))
	defer server.Close()
	code := fmt.Sprintf(`import g "xhub/guardrail"
 func ApplyGuardrail(texts []string, requestData map[string]any, inputType string) map[string]any {
 response:=g.LLMChat(%q,"llm-key","judge",[]map[string]any{{"role":"user","content":texts[0]}},2)
 if response["success"]!=true { return g.Block("judge failed") }
 return g.Allow()
 }`, server.URL+"/v1")
	action, _, _, err := runCustom(code, []string{"hi"}, nil, "request")
	if err != nil || action != "allow" {
		t.Fatalf("%s %v", action, err)
	}
}

// TestHTTPPrimitiveLimitsAndCancellation 验证共享上下文能中止 HTTP，非法 file URL 不能调用。
// 参数：t：Go 测试上下文；本地测试宿主或 HTTP 服务提供可观察的输入/输出。
// 返回：无；协议、动作或隔离不满足预期时报告测试失败。
// 调用：go test ./internal/gateway/guard；并发相关场景同时使用 -race。
// 测试：本函数即回归用例，不依赖真实供应商凭据。
func TestHTTPPrimitiveLimitsAndCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	start := time.Now()
	result := httpPrimitive(ctx, server.URL, "GET", nil, nil, 10)
	if boolOf(result["success"]) || result["error"] == nil || time.Since(start) > time.Second {
		t.Fatalf("cancellation failed: %v", result)
	}
	if boolOf(httpPrimitive(context.Background(), "file:///etc/passwd", "GET", nil, nil, 1)["success"]) {
		t.Fatal("file URL allowed")
	}
}

// TestXGoConcurrentNetworkIsolation 验证多个解释器并发调用 HTTP 时上下文和状态互不污染。
// 参数：t：Go 测试上下文；本地测试宿主或 HTTP 服务提供可观察的输入/输出。
// 返回：无；协议、动作或隔离不满足预期时报告测试失败。
// 调用：go test ./internal/gateway/guard；并发相关场景同时使用 -race。
// 测试：本函数即回归用例，不依赖真实供应商凭据。
func TestXGoConcurrentNetworkIsolation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"ok":true}`) }))
	defer server.Close()
	code := fmt.Sprintf(strings.Replace(testPolicy, "return Allow()", `r:=HTTPGet(%q,nil,1); if r["success"]!=true { return Block("failed") }; return Allow()`, 1), server.URL)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			a, _, _, e := runCustom(code, []string{"hi"}, nil, "request")
			if e != nil || a != "allow" {
				t.Errorf("%s %v", a, e)
			}
		}()
	}
	wg.Wait()
}
