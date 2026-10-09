package guard

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/goplus/ixgo"
	"io"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"strings"
	"time"
)

// jsonParse 将 JSON 字符串解析为脚本可访问的动态对象。
// 参数：text：最多 1 MiB 的 JSON 文本。
// 返回：any：对象、数组或基础值；非法 JSON 或超限会 panic，执行器按失败处理。
// 调用：JSONParse primitive。
// 测试：primitives_test.go。
func jsonParse(text string) any {
	if len(text) > maxCustomText {
		panic("JSON input too large")
	}
	var value any
	if err := json.Unmarshal([]byte(text), &value); err != nil {
		panic("invalid JSON")
	}
	return value
}

// jsonStringify 将脚本对象编码为 JSON 字符串并限制结果大小。
// 参数：value：可被 encoding/json 序列化的对象。
// 返回：JSON 字符串；不支持的值、循环引用或超过 1 MiB 会 panic。
// 调用：JSONStringify primitive、HTTP/LLM 示例。
// 测试：primitives_test.go。
func jsonStringify(value any) string {
	raw, err := json.Marshal(value)
	if err != nil || len(raw) > maxCustomText {
		panic("invalid or oversized JSON")
	}
	return string(raw)
}

// credential 解析专用于护栏的环境变量引用，避免把任意进程环境变量暴露给脚本。
// 参数：value：明文凭据，或 os.environ/XHUB_GUARDRAIL_* 引用。
// 返回：明文凭据；未授权的环境变量名或未配置变量返回空串，由调用方拒绝无效认证。
// 调用：httpPrimitive、Bedrock 凭据配置。
// 测试：primitives_test.go、external_test.go。
func credential(value string) string {
	if strings.HasPrefix(value, "os.environ/") {
		name := strings.TrimPrefix(value, "os.environ/")
		if !strings.HasPrefix(name, "XHUB_GUARDRAIL_") {
			return ""
		}
		return os.Getenv(name)
	}
	return value
}

// httpPrimitive 发送受控 HTTP 请求，统一超时、JSON 编码、响应上限和错误对象。
// 参数：ctx：本次执行的取消上下文；endpoint：无 URL 凭据的 HTTP(S) 地址；method：受支持的方法；headers：请求头及受控密钥引用；body：JSON 数据或 nil；seconds：超时秒数，默认及最大 10 秒。
// 返回：结果对象包含 success、status_code、body、headers、error；非 2xx、超时和无效配置均 success=false。错误信息不回显请求头或响应正文。
// 调用：网络 primitives 及 externalRequest、runPresidio。
// 测试：primitives_test.go、external_test.go；仅使用本地 HTTP 服务。
func httpPrimitive(ctx context.Context, endpoint, method string, headers map[string]string, body any, seconds int) map[string]any {
	result := map[string]any{"success": false, "status_code": 0, "body": nil, "headers": map[string]string{}, "error": nil}
	fail := func(message string) map[string]any { result["error"] = message; return result }
	u, err := url.Parse(endpoint)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		return fail("invalid HTTP URL")
	}
	method = strings.ToUpper(method)
	switch method {
	case "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS":
	default:
		return fail("unsupported HTTP method")
	}
	if seconds <= 0 {
		seconds = 10
	}
	if seconds > 10 {
		seconds = 10
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(seconds)*time.Second)
	defer cancel()
	var raw []byte
	if body != nil {
		raw, err = json.Marshal(body)
		if err != nil || len(raw) > maxCustomText {
			return fail("invalid or oversized HTTP body")
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(raw))
	if err != nil {
		return fail("invalid HTTP request")
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for key, value := range headers {
		reference := strings.Contains(value, "os.environ/")
		if strings.HasPrefix(value, "Bearer os.environ/") {
			value = "Bearer " + credential(strings.TrimPrefix(value, "Bearer "))
		} else {
			value = credential(value)
		}
		if reference && (value == "" || value == "Bearer ") {
			return fail("guardrail credential reference is missing or not allowed")
		}
		req.Header.Set(key, value)
	}
	// 禁止自动重定向，避免凭据被转发到另一地址，也避免跳转消耗无限预算。
	client := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(req)
	if err != nil {
		return fail("HTTP request failed or timed out")
	}
	defer response.Body.Close()
	raw, err = io.ReadAll(io.LimitReader(response.Body, maxCustomText+1))
	if err != nil || len(raw) > maxCustomText {
		return fail("HTTP response exceeds limit or cannot be read")
	}
	var parsed any
	if json.Unmarshal(raw, &parsed) != nil {
		parsed = string(raw)
	}
	// 不把 Set-Cookie 等远端敏感响应头暴露给脚本，只保留诊断所需字段。
	safeHeaders := map[string]string{}
	for _, key := range []string{"Content-Type", "Retry-After", "X-Request-Id"} {
		if v := response.Header.Get(key); v != "" {
			safeHeaders[key] = v
		}
	}
	result["status_code"] = response.StatusCode
	result["body"] = parsed
	result["headers"] = safeHeaders
	result["success"] = response.StatusCode >= 200 && response.StatusCode < 300
	if !result["success"].(bool) {
		result["error"] = fmt.Sprintf("HTTP status %d", response.StatusCode)
	}
	return result
}

// bindNetwork 把 HTTP/LLM 闭包绑定到当前解释器，保证并发脚本不会共享取消上下文。
// 参数：interp：已完成 RunInit 的解释器；ctx：当前脚本总时限上下文。
// 返回：无；内部绑定缺失时 panic，执行器会转换为执行错误。
// 调用：runCustomDetailed；编译器注入的变量仅供运行时使用。
// 测试：primitives_test.go 的并发及取消测试。
func bindNetwork(interp *ixgo.Interp, ctx context.Context) {
	request := func(url, method string, headers map[string]string, body any, timeout int) map[string]any {
		return httpPrimitive(ctx, url, method, headers, body, timeout)
	}
	functions := map[string]any{
		"HTTPRequest": request,
		"HTTPGet": func(url string, headers map[string]string, timeout int) map[string]any {
			return request(url, "GET", headers, nil, timeout)
		},
		"HTTPPost": func(url string, body any, headers map[string]string, timeout int) map[string]any {
			return request(url, "POST", headers, body, timeout)
		},
		// 直接调用 OpenAI 兼容端点，不经过本地模型路由；地址不应指回启用本护栏的网关。
		"LLMChat": func(base, key, model string, messages []map[string]any, timeout int) map[string]any {
			return request(strings.TrimRight(base, "/")+"/chat/completions", "POST", map[string]string{"Authorization": "Bearer " + key}, map[string]any{"model": model, "messages": messages, "stream": false}, timeout)
		},
	}
	for name, fn := range functions {
		ptr, ok := interp.GetVarAddr("__xhub" + name)
		if !ok {
			panic("missing runtime binding")
		}
		reflect.ValueOf(ptr).Elem().Set(reflect.ValueOf(fn))
	}
	ptr, ok := interp.GetVarAddr("__xhubBindings")
	if !ok {
		panic("missing runtime bindings")
	}
	value := reflect.ValueOf(ptr).Elem()
	for name, fn := range functions {
		value.FieldByName(name).Set(reflect.ValueOf(fn))
	}
}
