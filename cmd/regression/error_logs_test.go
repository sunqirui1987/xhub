package regression

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sunqirui1987/xhub/internal/iam"
)

// TestErrorLogsAreSeparateAndComplete 验证真实 HTTP 请求失败后独立筛选和完整落库。
// 参数 t 为测试上下文；前置关闭提示词保存并使用本地失败上游和真实数据库；
// 验证长 JSON、纯文本、提前拒绝、成功及任务状态归类、费用和错误详情；服务与私有 schema 自动清理。
func TestErrorLogsAreSeparateAndComplete(t *testing.T) {
	for _, format := range []string{"json", "text"} {
		t.Run(format, func(t *testing.T) {
			diagnostic := "diagnostic begin " + strings.Repeat("完整诊断", 1024) + " terminal cause"
			raw := diagnostic
			if format == "json" {
				raw = `{"error":{"code":"UPSTREAM_503","details":"` + diagnostic + `"}}`
			}
			up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(503)
				_, _ = w.Write([]byte(raw))
			}))
			t.Cleanup(up.Close)
			dep := chatDeployment("error-log-" + format)
			dep.LiteLLMParams["api_base"] = up.URL
			h := openHarness(t, false, dep)
			admin := h.adminSession()
			tn := h.provision(t, admin, "error-log-"+format)
			failed := h.do(http.MethodPost, "/v1/chat/completions", tn.key, map[string]any{
				"model": dep.ModelName, "messages": []any{map[string]any{"role": "user", "content": "private prompt"}},
			})
			if failed.status != 502 {
				t.Fatalf("上游 503 应返回 502: %s", failed.describe())
			}
			id := failed.header("x-litellm-call-id")
			detail := logDetail(t, h, admin, id)
			if detail["error"] != raw || !strings.Contains(string(mustJSON(detail["response"])), "terminal cause") {
				t.Fatalf("提示词保存关闭时丢失上游完整错误: %s", truncate(string(mustJSON(detail)), 300))
			}
			if strings.Contains(string(mustJSON(detail["messages"])), "private prompt") || firstFloat(detail, "spend") != 0 {
				t.Fatal("失败请求保存了私有提示词或产生费用")
			}
			rejected := h.do(http.MethodPost, "/v1/chat/completions", tn.key, map[string]any{"model": "missing-model", "messages": []any{}})
			if rejected.status < 400 {
				t.Fatalf("未知模型应拒绝: %s", rejected.describe())
			}
			rejectedDetail := logDetail(t, h, admin, rejected.header("x-litellm-call-id"))
			if rejectedDetail["error"] != rejected.text() {
				t.Fatal("提前拒绝未保存客户端完整错误")
			}

			// 写入非失败任务和兼容失败状态，证明过滤在分页之前执行，并保留执行中的任务。
			now := time.Now().UTC()
			for _, status := range []string{"success", "completed", "executing", "polling", "failed", "failure"} {
				err := h.db.RecordUsage(t.Context(), []iam.UsageRecord{{RequestID: "status-" + status, TS: now, UserID: tn.userID, Status: status, Model: dep.ModelName, HTTPStatus: 200}})
				if err != nil {
					t.Fatalf("写入状态夹具: %v", err)
				}
			}
			// 旧数据可能仍写 success，但 HTTP 已失败，必须归入错误日志且不漏掉。
			if err := h.db.RecordUsage(t.Context(), []iam.UsageRecord{{RequestID: "legacy-http-error", TS: now, UserID: tn.userID, Status: "success", Model: dep.ModelName, HTTPStatus: 400}}); err != nil {
				t.Fatal(err)
			}
			normal := h.ok(http.MethodGet, "/spend/logs/ui?status_filter=non_error&page_size=200", admin, nil)
			normalRows := rowsOf(normal, "data")
			if len(normalRows) != 4 {
				t.Fatalf("普通日志应只含 4 条非失败状态，得到 %d", len(normalRows))
			}
			for _, row := range normalRows {
				if strings.Contains(firstString(row, "status"), "fail") || firstString(row, "status") == "error" {
					t.Fatalf("错误混入普通日志: %v", row)
				}
			}
			errors := rowsOf(h.ok(http.MethodGet, "/spend/logs/ui?status_filter=error&page_size=200", admin, nil), "data")
			if len(errors) != 5 || findLogByRequestID(errors, id) == nil || findLogByRequestID(errors, "legacy-http-error") == nil {
				t.Fatalf("错误日志应含两条实际错误、两个兼容失败状态和 HTTP 失败旧数据: %v", errors)
			}
			missing := h.do(http.MethodGet, "/spend/logs/ui/nonexistent-error-log", admin, nil)
			for _, tc := range []struct {
				query string
				count int
			}{
				{"status_filter=non_error&search=" + id, 0}, {"status_filter=error&search=" + id, 1},
				{"status_filter=non_error&request_id=status-success", 1}, {"status_filter=error&request_id=status-success", 0},
			} {
				result := rowsOf(h.ok(http.MethodGet, "/spend/logs/ui?"+tc.query, admin, nil), "data")
				if len(result) != tc.count {
					t.Fatalf("分类与 ID 搜索不一致 %s: %v", tc.query, result)
				}
			}
			if missing.status != 404 {
				t.Fatalf("缺失日志应返回 404: %s", missing.describe())
			}
		})
	}
}
