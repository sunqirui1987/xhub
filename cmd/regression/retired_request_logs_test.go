package regression

import (
	"net/http"
	"testing"

	"github.com/sunqirui1987/xhub/internal/iam"
)

// TestRetiredRequestsDoNotCreateModelLogs 验证真实 HTTP 退役请求保留错误契约且不污染日志和用量。
// 参数 t 为测试上下文；前置隔离配置及身份库、本地供应商，可选独立 Redis；无返回值。
// 覆盖认证和未认证的旧接口、404 已删除接口、真实模型错误及成功记录；服务与私有 schema 自动清理。
func TestRetiredRequestsDoNotCreateModelLogs(t *testing.T) {
	h := newHarness(t, chatDeployment("retired-logs-control"))
	admin := h.adminSession()
	for _, tc := range []struct {
		method, path, credential string
		status                   int
	}{
		{http.MethodGet, "/v1/agents/agent1", admin, 410},
		{http.MethodGet, "/v1/tool/x", admin, 410},
		{http.MethodPost, "/v1/agents", admin, 410},
		{http.MethodGet, "/v1/agents/agent1", "", 401},
		{http.MethodGet, "/v1/tool/x", "", 401},
		{http.MethodGet, "/v1/mcp", admin, 404},
	} {
		result := h.do(tc.method, tc.path, tc.credential, nil)
		if result.status != tc.status {
			t.Fatalf("%s %s 应返回 %d: %s", tc.method, tc.path, tc.status, result.describe())
		}
		if tc.status == 410 {
			errorBody, _ := result.json()["error"].(map[string]any)
			if errorBody["type"] != "removed" {
				t.Fatalf("退役接口错误契约发生变化: %s", result.text())
			}
		}
		id := result.header("x-litellm-call-id")
		if id == "" {
			t.Fatal("错误响应必须保留调用关联 ID")
		}
		h.flushSpend()
		detail := h.do(http.MethodGet, "/spend/logs/ui/"+id, admin, nil)
		if detail.status != 404 {
			t.Fatalf("退役请求不应生成日志详情: %s", detail.describe())
		}
		list := h.ok(http.MethodGet, "/spend/logs/ui?status_filter=error&search="+id, admin, nil)
		if len(rowsOf(list, "data")) != 0 {
			t.Fatal("错误日志列表不应包含退役请求")
		}
	}
	for _, table := range []any{new(iam.UsageEvent), new(iam.RequestLog)} {
		count, err := h.db.Engine.Count(table)
		if err != nil || count != 0 {
			t.Fatalf("退役请求不得写入用量或正文表: count=%d err=%v", count, err)
		}
	}
	tenant := h.provision(t, admin, "retired-log-control")
	rejected := h.do(http.MethodPost, "/v1/chat/completions", tenant.key, map[string]any{"model": "missing-model", "messages": []any{}})
	if rejected.status < 400 {
		t.Fatal("未知模型必须被拒绝")
	}
	detail := logDetail(t, h, admin, rejected.header("x-litellm-call-id"))
	if detail["error"] != rejected.text() {
		t.Fatal("真实推理失败必须完整保存客户端错误")
	}
	success := h.ok(http.MethodPost, "/v1/chat/completions", tenant.key, map[string]any{"model": "retired-logs-control", "messages": []any{map[string]any{"role": "user", "content": "ping"}}})
	logDetail(t, h, admin, success.header("x-litellm-call-id"))
	count, err := h.db.Engine.Count(new(iam.UsageEvent))
	if err != nil || count != 2 {
		t.Fatalf("仅真实模型成功和失败应进入用量: count=%d err=%v", count, err)
	}
}
