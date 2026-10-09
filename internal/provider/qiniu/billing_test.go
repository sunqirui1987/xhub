package qiniu_test

import (
	"math"
	"testing"
	"time"

	"github.com/sunqirui1987/xhub/internal/catalog"
	"github.com/sunqirui1987/xhub/internal/provider"
	_ "github.com/sunqirui1987/xhub/internal/provider/qiniu"
)

// TestSeedanceMeasuredBands 验证分辨率和参考视频决定准确计价变体，用量只取实际完成 token。
// 参数 t：Go 测试上下文。返回：无；断言失败时报告协议或计费契约回归。
// 调用：go test；使用本地注册表、价格目录或假上游，不创建真实付费任务。
func TestSeedanceMeasuredBands(t *testing.T) {
	b := provider.SeedanceBilling()
	for _, tc := range []struct {
		model, resolution string
		video             bool
		variant           string
		usd               float64
	}{
		{"2-0-260128", "480p", false, "woiv", 6.66667e-6},
		{"2-0-260128", "720p", true, "wiv", 4.05797e-6},
		{"2-0-260128", "1080p", false, "1080p_woiv_v", 7.3913e-6},
		{"2-0-260128", "4k", true, "4k_wiv_v", 2.31884e-6},
		{"2-0-fast-260128", "480p", false, "woiv", 5.36232e-6},
		{"2-0-mini-260615", "720p", false, "woiv", 3.33333e-6},
		{"2-5-260628", "1080p", true, "1080p_wiv_v", 6.66667e-6},
	} {
		t.Run(tc.model+tc.variant, func(t *testing.T) {
			body := map[string]any{"model": "bytedance/doubao-seedance-" + tc.model, "resolution": "720p"}
			if tc.video {
				body["content"] = []any{map[string]any{"type": "video_url", "video_url": map[string]any{"url": "secret-url"}}}
			}
			doc := map[string]any{"status": "succeeded", "resolution": tc.resolution, "duration": 4, "usage": map[string]any{"completion_tokens": 1000, "total_tokens": 99999}}
			u := catalog.NormalizeUsage(b.Usage(doc, b.Context(body)))
			if u.OutputVariant != tc.variant || u.PromptTokens != 0 || u.CompletionTokens != 1000 || u.Seconds != 0 {
				t.Fatalf("incorrect measurement: %+v", u)
			}
			c, ok := catalog.CostAt("qiniu/"+body["model"].(string), u, time.Now())
			if !ok || len(c.Applied) != 1 || c.Applied[0].Fallback || c.Applied[0].Variant != tc.variant || math.Abs(c.Total-1000*tc.usd) > 1e-9 {
				t.Fatalf("incorrect price: %+v %v", c, ok)
			}
		})
	}
}

// TestSeedanceOnlySettlesExplicitSuccess 验证只有明确成功且无业务错误时提取用量，未知规格保持未定价。
// 参数 t：Go 测试上下文。返回：无；断言失败时报告协议或计费契约回归。
// 调用：go test；使用本地注册表、价格目录或假上游，不创建真实付费任务。
func TestSeedanceOnlySettlesExplicitSuccess(t *testing.T) {
	b := provider.SeedanceBilling()
	c := b.Context(map[string]any{"model": "m"})
	for _, status := range []string{"", "queued", "creating_assets", "running", "cancelled", "failed", "expired", "unknown"} {
		if u := b.Usage(map[string]any{"status": status, "usage": map[string]any{"completion_tokens": 100}}, c); u != nil {
			t.Fatalf("settled %q: %v", status, u)
		}
	}
	if b.Usage(map[string]any{"status": "succeeded", "error": map[string]any{"message": "failed"}, "usage": map[string]any{"completion_tokens": 100}}, c) != nil {
		t.Fatal("settled business error")
	}
	doc := map[string]any{"status": "succeeded", "usage": map[string]any{"completion_tokens": 100}, "tool_usage": map[string]any{"web_search": 2}}
	u := catalog.NormalizeUsage(b.Usage(doc, provider.TaskContext{}))
	if u.Searches != 2 || u.OutputVariant != "unknown" {
		t.Fatal(u)
	}
	if _, ok := catalog.CostAt("qiniu/bytedance/doubao-seedance-2-0-260128", u, time.Now()); ok {
		t.Fatal("missing context guessed cheapest price")
	}
}

// TestSeedanceSearchWithoutRateIsUnpriced 验证存在搜索用量但无对应费率时，不能将部分 token 费用当作完整账单。
// 参数 t：Go 测试上下文。返回：无；断言失败时报告协议或计费契约回归。
// 调用：go test；使用本地注册表、价格目录或假上游，不创建真实付费任务。
func TestSeedanceSearchWithoutRateIsUnpriced(t *testing.T) {
	b := provider.SeedanceBilling()
	facts := b.Context(map[string]any{"model": "bytedance/doubao-seedance-2-0-mini-260615"})
	u := catalog.NormalizeUsage(b.Usage(map[string]any{"status": "succeeded", "usage": map[string]any{"completion_tokens": 1000}, "tool_usage": map[string]any{"web_search": 2}}, facts))
	if _, ok := catalog.CostAt("qiniu/"+facts.Model, u, time.Now()); ok {
		t.Fatal("missing search price treated as complete bill")
	}
}
