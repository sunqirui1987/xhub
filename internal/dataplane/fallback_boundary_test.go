package dataplane

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/gateway/prefs"
	"github.com/sunqirui1987/xhub/internal/provider"
	"github.com/sunqirui1987/xhub/internal/router"
)

// TestFallbackTransportFailures 验证连接错误和超时在输出前能回退，网关开关不会泄漏上游。
// 前置无网络的确定性HTTP传输；分别执行适配和原生入口，断言仅a/b两次及成功响应，无资源需清理。
func TestFallbackTransportFailures(t *testing.T) {
	for _, failure := range []struct {
		name string
		err  error
	}{{"connection", errors.New("connection reset")}, {"timeout", context.DeadlineExceeded}} {
		for _, native := range []bool{false, true} {
			t.Run(failure.name+map[bool]string{false: "-adapted", true: "-native"}[native], func(t *testing.T) {
				calls := []string{}
				client := &http.Client{Transport: auditRoundTripper(func(r *http.Request) (*http.Response, error) {
					var doc map[string]any
					if err := json.NewDecoder(r.Body).Decode(&doc); err != nil {
						t.Fatal(err)
					}
					model, _ := doc["model"].(string)
					calls = append(calls, model)
					if _, exists := doc["disable_fallbacks"]; exists {
						t.Fatal("网关字段发送至上游")
					}
					if model == "a" {
						return nil, failure.err
					}
					raw := []byte(`{"id":"ok","choices":[{"index":0,"message":{"role":"assistant","content":"recovered"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`)
					return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(bytes.NewReader(raw))}, nil
				})}
				models := []config.ModelEntry{deployment("a", "a", "local", "https://example.invalid", "bypass_openai_chat", nil), deployment("b", "b", "local", "https://example.invalid", "bypass_openai_chat", nil)}
				settings := prefs.BuiltinSettings()
				settings.ModelFallbacks = map[string]router.FallbackPolicy{"a": {Fallbacks: []string{"b"}}}
				// 适配宿主的chatConfig会规范化入口；原生场景保留部署声明的bypass入口。
				cfg := &config.Config{ModelList: models}
				if !native {
					cfg = chatConfig(models...)
				}
				w := httptest.NewRecorder()
				if native {
					h := &fallbackNativeHost{logicHost: &logicHost{cfg: cfg, client: client, models: models, pins: map[string]string{}}, policy: settings}
					r := httptest.NewRequest("POST", "/bypass/openai/v1/chat/completions", bytes.NewBufferString(`{"model":"a","messages":[{"role":"user","content":"hi"}],"disable_fallbacks":false}`))
					hit, ok := provider.Match("POST", r.URL.Path, models)
					if !ok {
						t.Fatal("原生入口未匹配")
					}
					ServeBypass(h, w, r, hit)
				} else {
					h := &fallbackServeHost{logHost: newLogHost(cfg, client), settings: settings}
					Serve(h, w, chatRequest(t, "a", false), "chat")
				}
				if w.Code != 200 || !reflect.DeepEqual(calls, []string{"a", "b"}) {
					t.Fatalf("传输失败恢复错误: status=%d calls=%v body=%s", w.Code, calls, w.Body.String())
				}
			})
		}
	}
}

// TestFallbackClassificationMarkers 验证全部专用标记、大小写与状态优先级，并拒绝非错误包络。
// 前置纯内存错误正文，断言正确分类且不依赖供应商或存储，无资源清理。
func TestFallbackClassificationMarkers(t *testing.T) {
	for _, category := range []struct {
		kind    string
		markers []string
	}{
		{"context", []string{"CONTEXT_LENGTH_EXCEEDED", "context_window_exceeded", "Maximum Context Length", "prompt is too long", "input is too long"}},
		{"content", []string{"CONTENT_POLICY_VIOLATION", "content_filter", "content filtering policy", "Safety Policy", "safety_violation"}},
	} {
		for _, marker := range category.markers {
			t.Run(marker, func(t *testing.T) {
				raw, _ := json.Marshal(map[string]any{"error": map[string]any{"message": marker}})
				if got := fallbackKind(400, raw); got != category.kind {
					t.Fatalf("标记%s分类为%s", marker, got)
				}
				if got := fallbackKind(503, raw); got != "general" {
					t.Fatalf("503应优先通用策略: %s", got)
				}
			})
		}
	}
	for _, raw := range []string{`null`, `[]`, `{"error":null}`, `{"message":"context_length_exceeded"}`, `{"error":{"code":"invalid_request"},"text":"content_filter"}`} {
		if got := fallbackKind(400, []byte(raw)); got != "" {
			t.Fatalf("非专用错误触发回退: %s -> %s", raw, got)
		}
	}
}

// TestFallbackQueueNestedIsolation 验证嵌套链保持兄弟目标顺序、空池子链继续和请求间隔离。
// 前置共享不可变策略与两条独立请求队列；断言每条链顺序、类别切换和耗尽，不修改设置，无外部数据。
func TestFallbackQueueNestedIsolation(t *testing.T) {
	settings := prefs.BuiltinSettings()
	settings.ModelFallbacks = map[string]router.FallbackPolicy{
		"a":       {Fallbacks: []string{"missing", "b"}, ContextWindow: []string{"c"}},
		"missing": {Fallbacks: []string{"d"}},
		"b":       {ContentPolicy: []string{"c"}},
	}
	models := []config.ModelEntry{}
	for _, name := range []string{"b", "c", "d"} {
		models = append(models, config.ModelEntry{ModelName: name, LiteLLMParams: map[string]any{"deployment_id": name}})
	}
	before, _ := json.Marshal(settings)
	first, second := newFallbackQueue(settings, "a", false), newFallbackQueue(settings, "a", false)
	for _, tc := range []struct {
		q          *fallbackQueue
		kind, want string
	}{
		{first, "general", "b"}, {second, "context", "c"}, {first, "content", "d"}, {first, "general", "c"}, {first, "general", ""}, {second, "general", ""},
	} {
		pool, result := tc.q.next(tc.kind, models, router.State{})
		if result.Err != nil {
			t.Fatal(result.Err)
		}
		got := ""
		if len(pool) > 0 {
			got = pool[0].ModelName
		}
		if got != tc.want {
			t.Fatalf("%s下一目标%s，预期%s", tc.kind, got, tc.want)
		}
	}
	after, _ := json.Marshal(settings)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("请求队列修改了共享策略")
	}
	if len(second.seen) != 2 {
		t.Fatalf("其他请求污染已访问集: %v", second.seen)
	}
}
