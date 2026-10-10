package load

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// validConfig 返回短阶段单测配置；base 为本地服务器地址，返回合法参数，无外部副作用。
func validConfig(base string) Config {
	return Config{BaseURL: base, Key: "sk-test", Model: "m", Scenario: "chat", Concurrency: 4, Requests: 12, Duration: time.Second, Timeout: time.Second, PromptBytes: 1}
}

// TestSummarize 验证空、单样本和乱序分位数；前置固定数组，断言统计准确且原切片不变，无需清理。
func TestSummarize(t *testing.T) {
	values := []float64{4, 1, 3, 2}
	got := Summarize(values)
	if got.Samples != 4 || got.Mean != 2.5 || got.P50 != 2 || got.P95 != 4 || got.P99 != 4 || got.Max != 4 {
		t.Fatalf("分布错误: %+v", got)
	}
	if !reflect.DeepEqual(values, []float64{4, 1, 3, 2}) {
		t.Fatal("Summarize 修改了原样本")
	}
	if got := Summarize(nil); got != (Distribution{}) {
		t.Fatalf("空分布错误: %+v", got)
	}
	if got := Summarize([]float64{5}); got.P99 != 5 || got.Mean != 5 {
		t.Fatalf("单样本错误: %+v", got)
	}
}

// TestValidate 验证合法配置及失败边界；逐项替换参数，断言错误，无网络与清理副作用。
func TestValidate(t *testing.T) {
	base := validConfig("http://127.0.0.1:4000")
	if err := base.Validate(); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		change func(*Config)
	}{
		{"credentials", func(c *Config) { c.BaseURL = "http://user:pass@127.0.0.1" }},
		{"scheme", func(c *Config) { c.BaseURL = "file:///tmp/x" }},
		{"query", func(c *Config) { c.BaseURL += "?key=x" }},
		{"scenario", func(c *Config) { c.Scenario = "unknown" }},
		{"concurrency", func(c *Config) { c.Concurrency = 0 }},
		{"upper concurrency", func(c *Config) { c.Concurrency = 4097 }},
		{"requests", func(c *Config) { c.Requests = 0 }},
		{"duration", func(c *Config) { c.Duration = 0 }},
		{"timeout", func(c *Config) { c.Timeout = 0 }},
		{"prompt", func(c *Config) { c.PromptBytes = 1048577 }},
		{"key", func(c *Config) { c.Key = "" }},
		{"model", func(c *Config) { c.Model = "" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := base
			tc.change(&c)
			if err := c.Validate(); err == nil {
				t.Fatal("非法参数未拒绝")
			}
		})
	}
}

// TestValidateJSON 验证各场景正常、空和错误正文；t 为测试上下文，纯内存校验无需清理。
func TestValidateJSON(t *testing.T) {
	cases := []struct {
		name, body, scenario string
		valid                bool
	}{
		{"chat", "{\"choices\":[{\"message\":{\"content\":\"ok\"}}]}", "chat", true},
		{"empty", "{\"choices\":[]}", "chat", false},
		{"invalid", "broken", "chat", false},
		{"models", "{\"data\":[{\"id\":\"m\"}]}", "models", true},
		{"wrong model", "{\"data\":[{\"id\":\"other\"}]}", "models", false},
		{"auth", "{\"error\":{\"message\":\"denied\"}}", "auth-reject", true},
		{"null error", "{\"error\":null}", "auth-reject", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateJSON([]byte(tc.body), tc.scenario, "m")
			if (err == nil) != tc.valid {
				t.Fatalf("校验错误: %v wantValid=%t", err, tc.valid)
			}
		})
	}
}

// TestReadStream 验证完整 SSE 与截断/空/流内错误；固定正文读取，无外部清理。
func TestReadStream(t *testing.T) {
	content := "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n"
	for _, tc := range []struct {
		name, text string
		valid      bool
	}{
		{"complete", content + "data: [DONE]\n\n", true},
		{"truncated", content, false},
		{"empty", "data: [DONE]\n\n", false},
		{"malformed", "data: broken\n\n", false},
		{"error", "data: {\"error\":{\"message\":\"failed\"}}\n\n", false},
		{"after done", content + "data: [DONE]\n\n" + content, false},
		{"duplicate done", content + "data: [DONE]\n\ndata: [DONE]\n\n", false},
		{"crlf", strings.ReplaceAll(content+"data: [DONE]\n\n", "\n", "\r\n"), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			n, ttft, err := readStream(strings.NewReader(tc.text), time.Now().Add(-time.Millisecond))
			if (err == nil) != tc.valid || n == 0 || (tc.valid && (ttft <= 0 || n != int64(len(tc.text)))) {
				t.Fatalf("SSE n=%d ttft=%f err=%v", n, ttft, err)
			}
		})
	}
}

// TestRequestProtocols 验证真实 HTTP 的普通/流式请求字段、错误状态及畸形响应；本地服务器前置，结束关闭并回收连接。
func TestRequestProtocols(t *testing.T) {
	for _, scenario := range []string{"chat", "stream", "models", "auth-reject"} {
		t.Run(scenario, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if scenario == "models" {
					if r.Method != "GET" {
						t.Error("模型列表应 GET")
					}
					_, _ = w.Write([]byte("{\"data\":[{\"id\":\"m\"}]}"))
					return
				}
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				_, hasOptions := body["stream_options"]
				if hasOptions != (scenario == "stream") {
					t.Error("stream_options 应仅在流式请求出现")
				}
				if scenario == "auth-reject" {
					if strings.Contains(r.Header.Get("Authorization"), "sk-test") {
						t.Error("拒绝场景使用了合法 key")
					}
					w.WriteHeader(401)
					_, _ = w.Write([]byte("{\"error\":{\"message\":\"denied\"}}"))
					return
				}
				if scenario == "stream" {
					_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\ndata: [DONE]\n\n"))
					return
				}
				_, _ = w.Write([]byte("{\"choices\":[{\"message\":{\"content\":\"ok\"}}]}"))
			}))
			defer server.Close()
			c := validConfig(server.URL)
			c.Scenario = scenario
			result, err := Run(context.Background(), c)
			if err != nil || result.Succeeded != 12 {
				t.Fatalf("协议请求失败: %+v %v", result, err)
			}
		})
	}
	for _, tc := range []struct {
		name, body, category string
		status               int
	}{{"status", "denied", "http_status", 503}, {"redirect", "", "http_status", 302}, {"json", "broken", "invalid_response", 200}, {"limit", strings.Repeat("x", 4*1024*1024+1), "response_too_large", 200}} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Location", "/other")
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			c := validConfig(server.URL)
			c.Requests = 1
			c.Concurrency = 1
			result, err := Run(context.Background(), c)
			if err != nil || result.Errors[tc.category] != 1 {
				t.Fatalf("错误分类失败: %+v %v", result, err)
			}
		})
	}
}

// TestRunBoundsAndErrors 验证工作者上限、请求总数、错误统计和超时；前置本地 HTTP 服务，结束关闭服务器。
func TestRunBoundsAndErrors(t *testing.T) {
	var active, peak atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := active.Add(1)
		defer active.Add(-1)
		for {
			old := peak.Load()
			if n <= old || peak.CompareAndSwap(old, n) {
				break
			}
		}
		time.Sleep(5 * time.Millisecond)
		fmt.Fprint(w, "{\"choices\":[{\"message\":{\"content\":\"ok\"}}]}")
	}))
	defer server.Close()
	c := validConfig(server.URL)
	result, err := Run(context.Background(), c)
	if err != nil || result.Attempted != 12 || result.Succeeded != 12 || result.Failed != 0 || peak.Load() > 4 || peak.Load() < 2 {
		t.Fatalf("并发/计数错误: %+v peak=%d err=%v", result, peak.Load(), err)
	}
	c.Timeout = time.Millisecond
	result, err = Run(context.Background(), c)
	if err != nil || result.Failed != 12 || result.Errors["timeout"] != 12 {
		t.Fatalf("超时统计错误: %+v err=%v", result, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err = Run(ctx, c)
	if err != nil || result.Attempted != 0 {
		t.Fatalf("已取消阶段仍发流量: %+v err=%v", result, err)
	}
	c.Scenario = "invalid"
	if _, err := Run(context.Background(), c); err == nil {
		t.Fatal("非法配置未拒绝")
	}
}
