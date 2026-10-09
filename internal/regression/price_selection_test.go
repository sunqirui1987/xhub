package regression

import (
	"net/http"
	"testing"

	"github.com/sunqirui1987/xhub/internal/catalog"
)

// priceSelectionRow 从真实价格目录响应中定位测试行；参数为测试上下文、HTTP 结果与模型 ID，返回模型字段。
// 供分类回归断言调用；缺失模型即失败，无持久化副作用，数据由调用用例清理。
func priceSelectionRow(t *testing.T, response reply, id string) map[string]any {
	t.Helper()
	for _, row := range listField(response.json(), "models") {
		if row["id"] == id {
			return row
		}
	}
	t.Fatalf("价格目录缺少模型 %s", id)
	return nil
}

// TestPriceSelectionContract 验证价格下拉分类经过真实路由持久化并影响计价目录。
// 参数 t 为测试上下文，返回无；前置隔离 PostgreSQL 与网关，覆盖保存、编辑、清空、鉴权和错误不覆盖。
// 清理钩子移除进程目录记录，harness 销毁隔离 schema；不依赖外部服务凭据。
func TestPriceSelectionContract(t *testing.T) {
	h := newHarness(t)
	admin := h.adminSession()
	const id = "regression-price-selection"
	t.Cleanup(func() { catalog.RemoveModel(id) })
	h.ok(http.MethodPost, "/price/model", admin, map[string]any{
		"id": id, "litellm_provider": "openai", "mode": "chat", "endpoint_id": "bypass_openai_chat", "input_cost_per_token": 0.000002,
	})
	// 从真实读取路由取回分类，避免只断言 POST 请求没有真正验证写入结果。
	row := priceSelectionRow(t, h.ok(http.MethodGet, "/price/catalog", admin, nil), id)
	if row["litellm_provider"] != "openai" || row["mode"] != "chat" || row["endpoint_id"] != "bypass_openai_chat" {
		t.Fatalf("下拉分类未保存到价格目录: %#v", row)
	}
	if input, _, ok := catalog.TokenRates(id); !ok || input != 0.000002 {
		t.Fatalf("保存分类后计价不可用: input=%v ok=%v", input, ok)
	}
	for _, tc := range []struct {
		token  string
		body   map[string]any
		status int
	}{
		{"", map[string]any{"id": id, "litellm_provider": "custom", "endpoint_id": "bad"}, 401},
		{admin, map[string]any{"id": id}, 400},
		{admin, map[string]any{"id": id, "litellm_provider": "custom", "input_cost_per_token": -1, "endpoint_id": "bad"}, 400},
	} {
		response := h.do(http.MethodPost, "/price/model", tc.token, tc.body)
		if response.status != tc.status || errorMessage(response) == "" {
			t.Fatalf("分类写入错误响应不正确: %s", response.describe())
		}
	}
	row = priceSelectionRow(t, h.ok(http.MethodGet, "/price/catalog", admin, nil), id)
	if row["endpoint_id"] != "bypass_openai_chat" {
		t.Fatalf("失败请求覆盖了端点: %#v", row)
	}
	h.ok(http.MethodPost, "/price/model", admin, map[string]any{"id": id, "litellm_provider": "custom", "mode": "video_generation", "endpoint_id": "ark_contents_generation"})
	row = priceSelectionRow(t, h.ok(http.MethodGet, "/price/catalog", admin, nil), id)
	if row["mode"] != "video_generation" || row["endpoint_id"] != "ark_contents_generation" {
		t.Fatalf("编辑分类未生效: %#v", row)
	}
	h.ok(http.MethodPost, "/price/model", admin, map[string]any{"id": id, "litellm_provider": "custom", "mode": nil, "endpoint_id": nil})
	row = priceSelectionRow(t, h.ok(http.MethodGet, "/price/catalog", admin, nil), id)
	if row["mode"] != nil || row["endpoint_id"] != nil || row["input_cost_per_token"] != 0.000002 {
		t.Fatalf("未设置未清除分类或误清除单价: %#v", row)
	}
}
