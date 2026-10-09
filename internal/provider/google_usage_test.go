package provider

import (
	"encoding/json"
	"testing"
)

// TestGoogleNativeUsage 验证 Gemini 和 Vertex 的缓存、候选、思考用量及失败；参数 t 为上下文，离线响应无需清理。
func TestGoogleNativeUsage(t *testing.T) {
	for _, p := range []string{"gemini", "vertex"} {
		u := NativeUsage(p)(map[string]any{"usageMetadata": map[string]any{"promptTokenCount": 12, "candidatesTokenCount": json.Number("3"), "thoughtsTokenCount": 2.0, "cachedContentTokenCount": 4}}, nil)
		if u["prompt_tokens"] != 12 || u["completion_tokens"] != 5.0 || u["prompt_tokens_details"].(map[string]any)["cached_tokens"] != 4 {
			t.Fatal(u)
		}
		if NativeUsage(p)(map[string]any{"error": "failed"}, nil) != nil {
			t.Fatal("失败响应计费")
		}
		if NativeUsage(p)(map[string]any{}, nil)["pricing_blocked"] != "upstream_usage_missing" {
			t.Fatal("缺失用量未阻断")
		}
		var s NativeStreamState
		s.Observe(p, []byte(`data: {"candidates":[{"content":{}}]}

`))
		if s.Completed {
			t.Fatal("中间事件提前完成")
		}
		final := []byte(`data: {"candidates":[{"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":8,"candidatesTokenCount":2}}

`)
		s.Observe(p, final)
		if !s.Completed || NativeStreamUsage(p, final, nil)["completion_tokens"] != 2.0 {
			t.Fatal("完成流用量丢失")
		}
		s.Observe(p, []byte(`data: {"error":{"code":500}}

`))
		if !s.Failed {
			t.Fatal("错误流未标记失败")
		}
	}
}
