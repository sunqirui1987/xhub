package regression

import (
	"encoding/json"
	"github.com/sunqirui1987/xhub/internal/catalog"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestProviderFormsContract 验证真实公开目录和供应商凭据 CRUD 契约。
// 参数 t：回归上下文；返回无。前置 PostgreSQL 和网关，断言专属字段、Custom 入口、协议目录、认证、错误、元数据持久化与脱敏。
// 凭据显式删除，harness 自动销毁隔离 schema，不调用外部付费供应商。
func TestProviderFormsContract(t *testing.T) {
	h := newHarness(t)
	admin := h.adminSession()
	response := h.ok(http.MethodGet, "/public/providers/fields", "", nil)
	var rows []map[string]any
	if err := json.Unmarshal(response.body, &rows); err != nil {
		t.Fatalf("公开字段必须返回数组：%v", err)
	}
	byID := map[string]map[string]any{}
	for _, row := range rows {
		id := stringField(row, "provider")
		if byID[id] != nil {
			t.Fatalf("重复标识 %s", id)
		}
		byID[id] = row
	}
	for _, id := range []string{"Qiniu", "QINIU", "Fenno", "FENNO", "FennoAI", "FENNOAI"} {
		if byID[id] != nil {
			t.Fatalf("公开凭据目录不应预置 %s", id)
		}
	}
	for _, id := range []string{"CUSTOM", "CUSTOM_OPENAI"} {
		fields := map[string]bool{}
		for _, value := range byID[id]["credential_fields"].([]any) {
			fields[stringField(value.(map[string]any), "key")] = true
		}
		if !fields["api_base"] || !fields["api_key"] {
			t.Fatalf("%s 缺少自定义地址或密钥字段：%v", id, fields)
		}
	}
	endpoints := h.ok(http.MethodGet, "/public/endpoints", "", nil).json()
	transports := listField(endpoints, "transports")
	foundTransport := map[string]bool{}
	for _, transport := range transports {
		id := stringField(transport, "id")
		if id == "qiniu_contents_generation" || id == "qiniu_fal_kling" {
			foundTransport[id] = true
		}
	}
	if !foundTransport["qiniu_contents_generation"] || !foundTransport["qiniu_fal_kling"] {
		t.Fatalf("Custom 可选的七牛传输不完整：%v", foundTransport)
	}
	for id, key := range map[string]string{"Deepseek": "api_base", "Azure": "azure_ad_token", "Vertex_AI": "vertex_credentials", "Bedrock": "aws_region_name"} {
		found := false
		for _, value := range byID[id]["credential_fields"].([]any) {
			if value.(map[string]any)["key"] == key {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s 未提供专属字段 %s", id, key)
		}
	}
	const name = "provider-form-deepseek"
	payload := map[string]any{"credential_name": name, "credential_info": map[string]any{"custom_llm_provider": "deepseek", "provider_id": "Deepseek"}, "credential_values": map[string]any{"api_base": "https://api.deepseek.com", "api_key": "form-secret-123456"}}
	denied := h.do(http.MethodPost, "/credentials", "", payload)
	if denied.status != 401 {
		t.Fatalf("匿名创建凭据未拒绝：%s", denied.describe())
	}
	h.ok(http.MethodPost, "/credentials", admin, payload)
	stored, err := h.gw.RecordStore().GetKV("credentials", name)
	if err != nil || stored["credential_info"].(map[string]any)["provider_id"] != "Deepseek" {
		t.Fatalf("表单标识未持久化：%v %v", stored, err)
	}
	listed := h.ok(http.MethodGet, "/credentials", admin, nil).json()
	found := false
	for _, row := range listField(listed, "credentials") {
		if row["credential_name"] == name {
			found = true
			if row["credential_values"].(map[string]any)["api_key"] == "form-secret-123456" {
				t.Fatal("凭据列表泄露密钥")
			}
		}
	}
	if !found {
		t.Fatal("新建凭据未进入列表")
	}
	h.ok(http.MethodPatch, "/credentials/"+name, admin, map[string]any{"credential_values": map[string]any{"api_base": "http://local.example/v1"}})
	stored, err = h.gw.RecordStore().GetKV("credentials", name)
	if err != nil {
		t.Fatal(err)
	}
	values := stored["credential_values"].(map[string]any)
	if values["api_base"] != "http://local.example/v1" || values["api_key"] != "form-secret-123456" {
		t.Fatal("更新地址应保留已保存密钥")
	}
	missing := h.do(http.MethodPatch, "/credentials/"+name, "invalid-session", map[string]any{"credential_values": map[string]any{}})
	if missing.status != 401 {
		t.Fatalf("无效会话编辑错误契约：%s", missing.describe())
	}
	h.ok(http.MethodDelete, "/credentials/"+name, admin, nil)
	for _, row := range listField(h.ok(http.MethodGet, "/credentials", admin, nil).json(), "credentials") {
		if row["credential_name"] == name {
			t.Fatal("已删除凭据仍在列表")
		}
	}
}

// TestPriceReloadRequiresConfiguredSource 验证远程源失败时真实管理路由保留价格，允许保存定时计划。
// 前置隔离数据库及本地失败源；验证502、计划持久化与取消；harness 清理，不访问真实供应商。
func TestPriceReloadRequiresConfiguredSource(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) }))
	defer up.Close()
	t.Setenv("XHUB_PRICE_FEED_URL", up.URL)
	h := newHarness(t)
	admin := h.adminSession()
	before := h.ok(http.MethodGet, "/price/catalog", admin, nil).json()
	if r := h.do(http.MethodPost, "/reload/model_cost_map", admin, nil); r.status != 502 {
		t.Fatal(r.describe())
	}
	after := h.ok(http.MethodGet, "/price/catalog", admin, nil).json()
	if before["count"] != after["count"] || before["source"] != after["source"] {
		t.Fatal("失败刷新改变价格")
	}
	h.ok(http.MethodPost, "/schedule/model_cost_map_reload?hours=24", admin, nil)
	if h.ok(http.MethodGet, "/schedule/model_cost_map_reload/status", admin, nil).json()["scheduled"] != true {
		t.Fatal("计划未保存")
	}
	h.ok(http.MethodDelete, "/schedule/model_cost_map_reload", admin, nil)
}

// TestPriceLocalListingRoutes 验证真实HTTP刷新、本地快照、上下架、旧删除兼容和错误契约。
// 前置本地市场源及隔离schema；价格数据始终保留，最后恢复共享目录并由harness清理持久数据，无外部凭据。
func TestPriceLocalListingRoutes(t *testing.T) {
	before := catalog.PriceDocument{Version: 1, Source: catalog.PriceSource(), GeneratedAt: catalog.PriceGeneratedAt(), Models: catalog.CostMap(), Providers: catalog.Providers()}
	t.Cleanup(func() { _, _ = catalog.ApplyDocument(before) })
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"status": true, "data": []any{map[string]any{"id": "route-market-model", "name": "Route Market Model", "issuer": map[string]any{"name": "OpenAI"}, "pricing_rules_v2": []any{map[string]any{"details_v2": map[string]any{"text_input": map[string]any{"unit_name": "token", "unit_size": 1000000, "unit_price_usd": 2}}}}}}})
	}))
	defer up.Close()
	t.Setenv("XHUB_PRICE_FEED_URL", up.URL)
	h := newHarness(t)
	admin := h.adminSession()
	h.ok(http.MethodPost, "/reload/model_cost_map", admin, nil)
	h.ok(http.MethodPost, "/price/model/listing", admin, map[string]any{"id": "route-market-model", "delisted": true})
	h.ok(http.MethodPost, "/reload/model_cost_map", admin, nil)
	found := false
	for _, row := range listField(h.ok(http.MethodGet, "/price/catalog", admin, nil).json(), "models") {
		if row["id"] == "route-market-model" {
			found = true
			if row["delisted"] != true || row["pricing_rules_v2"] == nil {
				t.Fatalf("刷新后完整价格或下架状态丢失: %#v", row)
			}
		}
	}
	if !found {
		t.Fatal("本地模型记录丢失")
	}
	h.ok(http.MethodPost, "/price/model/listing", admin, map[string]any{"id": "route-market-model", "delisted": false})
	h.ok(http.MethodDelete, "/price/model", admin, map[string]any{"id": "route-market-model"})
	if r := h.do(http.MethodPost, "/price/model/listing", admin, map[string]any{"id": "route-market-model", "delisted": "true"}); r.status != 400 {
		t.Fatal(r.describe())
	}
	if r := h.do(http.MethodPost, "/price/model/listing", admin, map[string]any{"id": "unknown", "delisted": true}); r.status != 404 {
		t.Fatal(r.describe())
	}
	if r := h.do(http.MethodPost, "/price/model/listing", "", map[string]any{"id": "route-market-model", "delisted": true}); r.status != 401 {
		t.Fatal(r.describe())
	}
}
