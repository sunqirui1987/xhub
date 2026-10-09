package models

import (
	"net/http"
	"testing"

	"github.com/sunqirui1987/xhub/internal/catalog"
)

// TestPriceSelectionSurvivesReload 验证新端点字段写入数据库、重载回显、空值清除及历史字段兼容。
// 参数 t 为测试上下文，返回无；前置真实隔离存储，测试后销毁 schema 并移除进程目录记录。
func TestPriceSelectionSurvivesReload(t *testing.T) {
	h := &priceHost{store: openPriceStore(t), allow: true}
	const id = "price-selection-reload"
	t.Cleanup(func() { catalog.RemoveModel(id) })
	status, _ := call(h, http.MethodPost, "/price/model", map[string]any{
		"id": id, "litellm_provider": "custom", "mode": "video_generation", "endpoint_id": "ark_contents_generation", "endpoint_type": "legacy",
	})
	if status != 200 {
		t.Fatalf("分类保存失败: %d", status)
	}
	catalog.RemoveModel(id)
	LoadPriceOverrides(h)
	_, body := call(h, http.MethodGet, "/price/catalog", nil)
	row := priceRow(t, body, id)
	if row == nil || row["mode"] != "video_generation" || row["endpoint_id"] != "ark_contents_generation" || row["endpoint_type"] != "legacy" {
		t.Fatalf("重载丢失分类或历史字段: %#v", row)
	}
	status, _ = call(h, http.MethodPost, "/price/model", map[string]any{"id": id, "litellm_provider": "custom", "mode": nil, "endpoint_id": nil, "endpoint_type": nil})
	if status != 200 {
		t.Fatalf("清除分类失败: %d", status)
	}
	catalog.RemoveModel(id)
	LoadPriceOverrides(h)
	_, body = call(h, http.MethodGet, "/price/catalog", nil)
	row = priceRow(t, body, id)
	if row == nil || row["mode"] != nil || row["endpoint_id"] != nil || row["endpoint_type"] != nil {
		t.Fatalf("重载恢复了已清除的分类: %#v", row)
	}
}
