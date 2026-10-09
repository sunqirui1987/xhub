package models

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/sunqirui1987/xhub/internal/catalog"
)

// TestReloadDefaultFeed 验证缺少源与错误源的价格刷新边界。
// 参数 t：测试上下文；返回：无。核对失败不修改当前目录，本地 HTTP 服务自动关闭，不访问外部服务。
func TestReloadDefaultFeed(t *testing.T) {
	before := catalog.PriceSource()
	t.Setenv("XHUB_PRICE_FEED_URL", "")
	if priceFeedURL() != catalog.MarketURL {
		t.Fatal("未配置镜像时没有使用 Modelink 来源")
	}
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) }))
	defer up.Close()
	t.Setenv("XHUB_PRICE_FEED_URL", up.URL)
	if _, err := reloadNow(context.Background()); err == nil {
		t.Fatal("503源未报告失败")
	}
	if catalog.PriceSource() != before {
		t.Fatal("失败源改变了当前价格")
	}
}

// TestLocalCatalogRefreshPersistence 验证刷新快照、下架、价格覆盖、上游撤回及离线启动。
// 前置隔离数据库和本地HTTP源，验证完整价格记录不丢失，失败刷新保持快照；结束恢复共享目录并清理schema。
func TestLocalCatalogRefreshPersistence(t *testing.T) {
	before := catalog.PriceDocument{Version: 1, Source: catalog.PriceSource(), GeneratedAt: catalog.PriceGeneratedAt(), Models: catalog.CostMap(), Providers: catalog.Providers()}
	t.Cleanup(func() { _, _ = catalog.ApplyDocument(before) })
	h := &priceHost{store: openPriceStore(t), allow: true}
	payload := map[string]any{"status": true, "data": []any{map[string]any{"id": "snapshot-a", "name": "Snapshot A", "issuer": map[string]any{"name": "OpenAI"}, "pricing_rules_v2": []any{map[string]any{"details_v2": map[string]any{"text_input": map[string]any{"unit_name": "token", "unit_size": 1000, "unit_price_usd": 0.002}}}}}}}
	failing := false
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if failing {
			w.WriteHeader(503)
			return
		}
		_ = json.NewEncoder(w).Encode(payload)
	}))
	defer up.Close()
	t.Setenv("XHUB_PRICE_FEED_URL", up.URL)
	if _, err := refreshLocalCatalog(context.Background(), h); err != nil {
		t.Fatal(err)
	}
	if code, _ := call(h, http.MethodPost, "/price/model/listing", map[string]any{"id": "snapshot-a", "delisted": true}); code != 200 {
		t.Fatal(code)
	}
	call(h, http.MethodPost, "/price/model", map[string]any{"id": "snapshot-a", "litellm_provider": "openai", "input_cost_per_token": 0.000007})
	payload["data"] = []any{map[string]any{"id": "snapshot-b", "issuer": map[string]any{"name": "OpenAI"}}}
	if _, err := refreshLocalCatalog(context.Background(), h); err != nil {
		t.Fatal(err)
	}
	catalog.RemoveModel("snapshot-a")
	LoadPriceOverrides(h)
	_, body := call(h, http.MethodGet, "/price/catalog", nil)
	row := priceRow(t, body, "snapshot-a")
	if row == nil || row["delisted"] != true || row["pricing_rules_v2"] == nil || row["input_cost_per_token"] != 0.000007 {
		t.Fatalf("本地记录或覆盖丢失: %#v", row)
	}
	// 手工覆盖只覆盖价格，不应掩盖最新市场撤回状态。
	base, _ := catalog.BaselineModel("snapshot-a")
	if base["feed_unavailable"] != true || row["feed_unavailable"] != true {
		t.Fatal("上游撤回未存档")
	}
	saved, _ := h.store.ListConfig(priceSnapshotNS)
	failing = true
	if _, err := refreshLocalCatalog(context.Background(), h); err == nil {
		t.Fatal("失败源未返回错误")
	}
	after, _ := h.store.ListConfig(priceSnapshotNS)
	if !reflect.DeepEqual(saved, after) {
		t.Fatal("失败源污染快照")
	}
}
