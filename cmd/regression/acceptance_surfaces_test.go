package regression

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// TestAcceptanceGuardrailMonitor 验证真实数据面执行的拦截、脱敏和标记会持久化并进入监控页面所用接口。
// 前置条件是私有数据库 schema、本地假上游和三条非默认护栏；验证 overview、detail、logs 及管理员权限，规则和 schema 由测试清理。
func TestAcceptanceGuardrailMonitor(t *testing.T) {
	h := newHarness(t, chatDeployment("acceptance-guardrail-monitor"))
	admin := h.adminSession()
	tenant := h.provision(t, admin, "acceptance-monitor")

	blockID := createAcceptanceGuardrail(t, h, admin, "acceptance-monitor-block", map[string]any{
		"guardrail": "blocked_words", "blocked_words": []string{"monitor-block-secret"}, "mode": "pre_call", "default_on": false,
	})
	redactID := createAcceptanceGuardrail(t, h, admin, "acceptance-monitor-redact", map[string]any{
		"guardrail": "redact", "blocked_words": []string{"monitor-redact-secret"}, "mode": "pre_call", "default_on": false,
	})
	flagSource := "import . \"xhub/guardrail\"\n" +
		"func ApplyGuardrail(texts []string, requestData map[string]any, inputType string) map[string]any {\n" +
		" return Flag(\"acceptance manual review\", map[string]any{\"source\":\"acceptance\"})\n}"
	flagID := createAcceptanceGuardrail(t, h, admin, "acceptance-monitor-flag", map[string]any{
		"guardrail": "custom_code", "custom_code_language": "xgo", "custom_code": flagSource, "mode": "pre_call", "default_on": false,
	})
	for _, id := range []string{blockID, redactID, flagID} {
		id := id
		t.Cleanup(func() { h.do(http.MethodDelete, "/guardrails/"+url.PathEscape(id), admin, nil) })
	}

	h.resetUpstream()
	blocked := acceptanceChat(h, tenant.key, "acceptance-guardrail-monitor", "monitor-block-secret", "acceptance-monitor-block")
	if blocked.status != http.StatusBadRequest || len(h.upstreamCalls()) != 0 {
		t.Fatalf("真实拦截未在上游前终止: status=%d upstream=%d body=%s", blocked.status, len(h.upstreamCalls()), blocked.text())
	}
	redacted := acceptanceChat(h, tenant.key, "acceptance-guardrail-monitor", "monitor-redact-secret", "acceptance-monitor-redact")
	if redacted.status != http.StatusOK {
		t.Fatalf("脱敏请求失败: %s", redacted.describe())
	}
	calls := h.upstreamCalls()
	if len(calls) != 1 || strings.Contains(string(mustJSON(calls[0].Body)), "monitor-redact-secret") || !strings.Contains(string(mustJSON(calls[0].Body)), "[REDACTED]") {
		t.Fatalf("上游未收到脱敏正文: %#v", calls)
	}
	flagged := acceptanceChat(h, tenant.key, "acceptance-guardrail-monitor", "ordinary flagged text", "acceptance-monitor-flag")
	if flagged.status != http.StatusOK || len(h.upstreamCalls()) != 2 {
		t.Fatalf("标记动作应放行并访问上游一次: status=%d upstream=%d", flagged.status, len(h.upstreamCalls()))
	}
	h.flushSpend()

	overview := h.ok(http.MethodGet, "/guardrails/usage/overview", admin, nil).json()
	total, _ := floatField(overview, "totalRequests")
	blockedTotal, _ := floatField(overview, "totalBlocked")
	if total < 3 || blockedTotal < 1 {
		t.Fatalf("监控总览没有真实非零事件: %#v", overview)
	}
	rows := listField(overview, "rows")
	for _, want := range []struct{ id, action string }{{blockID, "blocked"}, {redactID, "flagged"}, {flagID, "flagged"}} {
		row := findBy(rows, "id", want.id)
		if row == nil {
			t.Fatalf("监控总览缺少护栏 %s: %#v", want.id, rows)
		}
		requests, _ := floatField(row, "requestsEvaluated")
		if requests < 1 {
			t.Fatalf("护栏 %s 的评估数为空: %#v", want.id, row)
		}
		detail := h.ok(http.MethodGet, "/guardrails/usage/detail/"+url.PathEscape(want.id), admin, nil).json()
		detailRequests, _ := floatField(detail, "requestsEvaluated")
		if detailRequests < 1 {
			t.Fatalf("护栏 %s 详情没有真实评估: %#v", want.id, detail)
		}
		logs := h.ok(http.MethodGet, "/guardrails/usage/logs?guardrail_id="+url.QueryEscape(want.id), admin, nil).json()
		logRows := listField(logs, "logs")
		if len(logRows) == 0 || stringField(logRows[0], "action") != want.action {
			t.Fatalf("护栏 %s 日志动作错误，期望 %s: %#v", want.id, want.action, logs)
		}
		if logRows[0]["input_snippet"] != nil || logRows[0]["output_snippet"] != nil {
			t.Fatalf("监控日志泄漏了请求正文: %#v", logRows[0])
		}
		if stringField(logRows[0], "guardrail_name") == "" || stringField(logRows[0], "guardrail_mode") != "pre_call" {
			t.Fatalf("监控日志缺少护栏名称或执行阶段: %#v", logRows[0])
		}
	}
	protectedPaths := []string{
		"/guardrails/usage/overview",
		"/guardrails/usage/detail/" + url.PathEscape(blockID),
		"/guardrails/usage/logs?guardrail_id=" + url.QueryEscape(blockID),
	}
	for _, path := range protectedPaths {
		for _, token := range []string{"", tenant.session, tenant.key} {
			denied := h.do(http.MethodGet, path, token, nil)
			if denied.status >= 200 && denied.status < 300 {
				t.Fatalf("非管理员访问护栏监控未被拒绝: path=%s token-present=%t body=%s", path, token != "", denied.text())
			}
		}
	}
}

// createAcceptanceGuardrail 通过真实管理 API 保存一条验收护栏。
// 参数包含测试句柄、管理员令牌、唯一名称和执行参数；返回持久化 ID，失败直接终止当前测试，调用方负责删除。
func createAcceptanceGuardrail(t *testing.T, h *harness, admin, name string, params map[string]any) string {
	t.Helper()
	created := h.ok(http.MethodPost, "/guardrails", admin, map[string]any{
		"guardrail": map[string]any{"guardrail_name": name, "litellm_params": params},
	}).json()
	id := stringField(created, "guardrail_id")
	if id == "" {
		t.Fatalf("护栏 %s 创建后没有持久化 ID: %#v", name, created)
	}
	return id
}

// acceptanceChat 调用本地假上游支持的真实聊天数据面并显式选择一条护栏。
// 参数是网关句柄、租户密钥、模型、正文和护栏名称；返回完整 HTTP 响应，不读取供应商凭据且不产生外部付费调用。
func acceptanceChat(h *harness, key, model, text, guardrail string) reply {
	return h.do(http.MethodPost, "/v1/chat/completions", key, map[string]any{
		"model": model, "guardrails": []string{guardrail},
		"messages": []any{map[string]any{"role": "user", "content": text}},
	})
}

// TestAcceptanceModelLifecycle 验证模型管理 API 的创建约束、停用故障和恢复可用形成完整真实数据面闭环。
// 前置条件是私有 schema、本地假上游和无限定模型的租户密钥；验证无上游、零计费拒绝及调用次数，部署和 schema 由测试清理。
func TestAcceptanceModelLifecycle(t *testing.T) {
	h := newHarness(t)
	admin := h.adminSession()
	tenant := h.provision(t, admin, "acceptance-lifecycle")
	info := map[string]any{"id": "acceptance-lifecycle-id", "transport": "bypass_openai_chat", "endpoint_types": []string{"chat"}, "pricing_source": "manual"}
	missing := h.do(http.MethodPost, "/model/new", admin, map[string]any{
		"model_name":     "acceptance-lifecycle-model",
		"litellm_params": map[string]any{"model": "acceptance-upstream", "custom_llm_provider": "custom", "api_key": "local-only", "input_cost_per_token": 0.000001},
		"model_info":     info,
	})
	if missing.status != http.StatusBadRequest || !strings.Contains(errorMessage(missing), "api_base") {
		t.Fatalf("无上游部署未按契约拒绝: %s", missing.describe())
	}
	params := map[string]any{"model": "acceptance-upstream", "custom_llm_provider": "custom", "api_key": "local-only", "api_base": h.prices.URL + "/v1", "input_cost_per_token": 0, "output_cost_per_token": 0}
	zero := h.do(http.MethodPost, "/model/new", admin, map[string]any{"model_name": "acceptance-lifecycle-model", "litellm_params": params, "model_info": info})
	if zero.status != http.StatusBadRequest || !strings.Contains(errorMessage(zero), "positive") {
		t.Fatalf("全零手工计费部署未按契约拒绝: %s", zero.describe())
	}
	params["input_cost_per_token"] = 0.000001
	params["output_cost_per_token"] = 0.000002
	created := h.ok(http.MethodPost, "/model/new", admin, map[string]any{"model_name": "acceptance-lifecycle-model", "litellm_params": params, "model_info": info}).json()
	id := stringField(created["model_info"].(map[string]any), "id")
	if id != "acceptance-lifecycle-id" {
		t.Fatalf("模型创建 ID 错误: %#v", created)
	}
	t.Cleanup(func() { h.do(http.MethodPost, "/model/delete", admin, map[string]any{"id": id}) })

	h.resetUpstream()
	callNumber := 0
	call := func() reply {
		callNumber++
		return h.do(http.MethodPost, "/v1/chat/completions", tenant.key, map[string]any{
			"model": "acceptance-lifecycle-model", "messages": []any{map[string]any{"role": "user", "content": "local lifecycle check " + string(rune('0'+callNumber))}},
		})
	}
	if first := call(); first.status != http.StatusOK || len(h.upstreamCalls()) != 1 {
		t.Fatalf("新上架模型不可调用: status=%d upstream=%d body=%s", first.status, len(h.upstreamCalls()), first.text())
	}
	h.ok(http.MethodPost, "/model/disable", admin, map[string]any{"id": id})
	rows := listField(h.ok(http.MethodGet, "/v2/model/info?modelId="+url.QueryEscape(id), admin, nil).json(), "data")
	if len(rows) != 1 || rows[0]["model_info"].(map[string]any)["disabled"] != true {
		t.Fatalf("停用状态未持久化并展示: %#v", rows)
	}
	before := len(h.upstreamCalls())
	if disabled := call(); disabled.status >= 200 && disabled.status < 300 {
		t.Fatalf("已下架模型仍可调用: %s", disabled.describe())
	}
	if len(h.upstreamCalls()) != before {
		t.Fatalf("已下架模型仍访问上游: before=%d after=%d", before, len(h.upstreamCalls()))
	}
	h.ok(http.MethodPost, "/model/enable", admin, map[string]any{"id": id})
	if restored := call(); restored.status != http.StatusOK || len(h.upstreamCalls()) != before+1 {
		t.Fatalf("恢复后模型未重新可用: status=%d upstream=%d body=%s", restored.status, len(h.upstreamCalls()), restored.text())
	}
}
