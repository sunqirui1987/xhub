package gateway

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sunqirui1987/xhub/internal/iam"
)

// TestStatusRecorderCapturesOnlyCompleteFailures 验证错误收集器对多次写入保留完整字节且不缓存成功响应。
// 参数 t 为测试上下文；前置内存 HTTP writer，覆盖隐式 200、显式 200、400 边界和 502；
// 首个状态不可被后续状态覆盖，返回字节数与客户端正文一致；不访问外部存储，无需清理。
func TestStatusRecorderCapturesOnlyCompleteFailures(t *testing.T) {
	for _, code := range []int{0, 200, 400, 502} {
		t.Run(http.StatusText(code), func(t *testing.T) {
			output := httptest.NewRecorder()
			recorder := &statusRecorder{ResponseWriter: output, code: 200}
			if code != 0 {
				recorder.WriteHeader(code)
			}
			chunks := []string{"诊断开始", strings.Repeat("full body ", 2048), "终末原因"}
			for _, chunk := range chunks {
				n, err := recorder.Write([]byte(chunk))
				if err != nil || n != len(chunk) {
					t.Fatalf("响应写入结果错误: %d %v", n, err)
				}
			}
			recorder.WriteHeader(503)
			expectedStatus := code
			if code == 0 {
				expectedStatus = 200
			}
			if recorder.code != expectedStatus || output.Code != expectedStatus {
				t.Fatal("后续状态覆盖了首个响应状态")
			}
			if output.Body.String() != strings.Join(chunks, "") {
				t.Fatal("客户端正文被改写")
			}
			expectedBody := ""
			if code >= 400 {
				expectedBody = output.Body.String()
			}
			if recorder.body.String() != expectedBody {
				t.Fatal("失败正文不完整或成功正文被缓存")
			}
		})
	}
}

// TestRecordEarlyErrorExcludesRemovedRoutes 验证退役接口在鉴权失败、410 和已删除 404 时均不写模型日志。
// 参数 t 为测试上下文；前置私有数据库与真实结算路径，直接调用兜底函数覆盖 GET、POST 和尾斜杠。
// 返回：无；受支持推理接口的 401 仍须完整落库，连接池和私有 schema 自动清理。
func TestRecordEarlyErrorExcludesRemovedRoutes(t *testing.T) {
	cfg, st, db := testGatewayStores(t)
	s := New(cfg, st, db)
	for _, tc := range []struct {
		method, path string
		code         int
	}{
		{"GET", "/v1/agents/agent1", 410},
		{"GET", "/v1/tool/x", 410},
		{"POST", "/v1/agents/", 401},
		{"POST", "/v1beta/agents", 410},
		{"GET", "/v1/memory/x", 410},
		{"POST", "/v1/workflows", 410},
		{"GET", "/v1/mcp", 404},
		{"GET", "/v1/search/x", 404},
	} {
		t.Run(tc.method+tc.path, func(t *testing.T) {
			recorder := &statusRecorder{ResponseWriter: httptest.NewRecorder(), code: 200}
			recorder.WriteHeader(tc.code)
			recorder.Header().Set("x-litellm-call-id", tc.path)
			_, _ = recorder.Write([]byte(`{"error":{"type":"removed"}}`))
			s.recordEarlyError(recorder, httptest.NewRequest(tc.method, tc.path, nil), nil, nil, time.Now())
			count, err := db.Engine.Count(new(iam.UsageEvent))
			if err != nil || count != 0 {
				t.Fatalf("已删除接口不应写用量: count=%d err=%v", count, err)
			}
			count, err = db.Engine.Count(new(iam.RequestLog))
			if err != nil || count != 0 {
				t.Fatalf("已删除接口不应写正文: count=%d err=%v", count, err)
			}
		})
	}
	recorder := &statusRecorder{ResponseWriter: httptest.NewRecorder(), code: 200}
	recorder.WriteHeader(401)
	recorder.Header().Set("x-litellm-call-id", "supported-auth-failure")
	_, _ = recorder.Write([]byte(`{"error":{"type":"authentication_error"}}`))
	s.recordEarlyError(recorder, httptest.NewRequest("POST", "/v1/chat/completions", nil), nil, nil, time.Now())
	count, err := db.Engine.Count(new(iam.UsageEvent))
	if err != nil || count != 1 {
		t.Fatalf("真实推理接口鉴权失败必须保留日志: count=%d err=%v", count, err)
	}
}

// TestRecordEarlyErrorSkipsNonDataPlane 验证兜底记录仅处理有请求 ID 的受支持数据面失败。
// 参数 t 为测试上下文；前置无数据库的服务，覆盖成功、后台错误、缺失 ID、退役路径和空接收者；
// 均应直接返回且不创建暂存正文；内存数据随测试释放。
func TestRecordEarlyErrorSkipsNonDataPlane(t *testing.T) {
	for _, tc := range []struct {
		path   string
		code   int
		callID string
	}{
		{"/v1/chat/completions", 200, "success"},
		{"/model/new", 500, "management-error"},
		{"/v1/chat/completions", 400, ""},
		{"/v1/agents/agent1", 410, "retired-agent"},
		{"/v1/tool/x", 410, "retired-tool"},
		{"/v1/agents/", 401, "retired-auth"},
		{"/v1/mcp", 404, "removed-mcp"},
	} {
		s := &Server{}
		recorder := &statusRecorder{ResponseWriter: httptest.NewRecorder(), code: tc.code}
		recorder.Header().Set("x-litellm-call-id", tc.callID)
		s.recordEarlyError(recorder, httptest.NewRequest("POST", tc.path, nil), nil, nil, time.Now())
		if len(s.exchanges) != 0 {
			t.Fatal("非数据面错误或缺失 ID 的请求被记录")
		}
	}
	var absent *Server
	absent.recordEarlyError(nil, nil, nil, nil, time.Now())
}
