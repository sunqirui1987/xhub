package qiniu_test

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/sunqirui1987/xhub/internal/catalog"
	"github.com/sunqirui1987/xhub/internal/provider"
)

// falHit 查找测试所需的具体 Fal 创建路径；缺少登记时立即终止当前测试。
// 参数 t：测试上下文；path：不含 /queue/ 的具体模型路径。返回 provider.Hit：已登记操作。
// 调用：本文件的计量和计价测试；只查询本地注册表。
func falHit(t *testing.T, path string) provider.Hit {
	t.Helper()
	h, ok := provider.Match("POST", "/queue/"+path, nil)
	if !ok {
		t.Fatal("missing Fal model", path)
	}
	return h
}

// TestFalRegistrationAndConcreteRoutes 验证所有登记的具体创建路径、鉴权和公共查询队列，并拒绝未开放路径。
// 参数 t：Go 测试上下文。返回：无；断言失败时报告协议或计费契约回归。
// 调用：go test；使用本地注册表、价格目录或假上游，不创建真实付费任务。
func TestFalRegistrationAndConcreteRoutes(t *testing.T) {
	families := map[string]int{}
	count := 0
	for _, tr := range provider.Transports() {
		if !tr.QueueURLs {
			continue
		}
		families[tr.ID] = 0
		if tr.Auth.Prefix != "Key" || tr.Auth.Header != "Authorization" || tr.TaskID != "request_id" || tr.Billing == nil {
			t.Fatal(tr.ID)
		}
		for _, a := range tr.Actions {
			if a.Name != "create" {
				continue
			}
			count++
			families[tr.ID]++
			h := falHit(t, a.Model)
			if h.Transport.ID != tr.ID || h.Action.UpstreamPath != a.PublicPath || provider.ModelEndpoints()["qiniu/"+a.Model] != tr.ID {
				t.Fatal(a)
			}
			if a.PublicPath != "/queue/"+a.Model {
				t.Fatal(a)
			}
			for _, follow := range tr.Actions {
				if follow.Name == "create" {
					continue
				}
				path := provider.Expand(follow.PublicPath, map[string]string{"request_id": "task-1"})
				h, ok := provider.Match("GET", path, nil)
				if !ok || h.Transport.ID != tr.ID || h.Names["request_id"] != "task-1" {
					t.Fatal(path)
				}
			}
		}
	}
	if len(families) != 9 || count < 85 {
		t.Fatalf("incomplete catalog: %d endpoints, families %v", count, families)
	}
	t.Logf("registered %d concrete create endpoints across %d queue families", count, len(families))
	for _, path := range []string{"fal-ai/kling-video/v2.5-turbo/standard/text-to-video", "fal-ai/kling-video/v2.6/standard/image-to-video", "fal-ai/kling-video/o1/4k/text-to-video", "unknown/video", "fal-ai/veo3.1/requests/id"} {
		if _, ok := provider.Match("POST", "/queue/"+path, nil); ok {
			t.Fatal("unregistered path accepted", path)
		}
	}
}

// TestFalMeasurementsAndSellerPrices 验证成功结果按实际 token 或生成秒数计量，并精确绑定供应商价格变体。
// 参数 t：Go 测试上下文。返回：无；断言失败时报告协议或计费契约回归。
// 调用：go test；使用本地注册表、价格目录或假上游，不创建真实付费任务。
func TestFalMeasurementsAndSellerPrices(t *testing.T) {
	for _, tc := range []struct {
		path, body, variant string
		quantity, rate      float64
		token               bool
	}{
		{"byteplus/seedance-2.5/text-to-video", "{}", "woiv", 1000, 0, true},
		{"bytedance/seedance-2.0/mini/reference-to-video", "{\"video_urls\":[\"https://example.org/input.mp4\"],\"resolution\":\"720p\"}", "wiv", 1000, 0, true},
		{"fal-ai/kling-video/v2.5-turbo/pro/text-to-video", "{}", "pro_norefv_v_duration", 5.5, 0.07246377, false},
		{"fal-ai/kling-video/v2.5-turbo/standard/image-to-video", "{}", "std_norefv_v_duration", 5.5, 0.04347826, false},
		{"fal-ai/kling-video/v2.6/pro/text-to-video", "{}", "pro_novoice_av_duration", 5.5, 0.14492754, false},
		{"fal-ai/kling-video/v3/4k/text-to-video", "{\"generate_audio\":false}", "4k_norefv_v_duration", 5.5, 0.43478261, false},
		{"fal-ai/kling-video/o1/pro/video-to-video/reference", "{\"video_url\":\"https://example.org/input.mp4\"}", "pro_refv_v_duration", 5.5, 0.17391304, false},
		{"fal-ai/vidu/q1/start-end-to-video", "{}", "1080p_i2v_duration", 5.5, 0.07246377, false},
		{"fal-ai/vidu/q3/reference-to-video", "{}", "720p_r2v_duration", 5.5, 0.05434783, false},
		{"fal-ai/vidu/q3/text-to-video/pro", "{}", "720p_t2v_duration", 5.5, 0.13586957, false},
		{"fal-ai/veo3.1", "{}", "av", 5.5, 0.4, false},
		{"fal-ai/veo3.1/image-to-video", "{\"generate_audio\":false,\"resolution\":\"4k\"}", "4k_v_duration", 5.5, 0.4, false},
		{"minimax/h3-max/text-to-video", "{}", "768p_v_duration", 5.5, 0.07246377, false},
	} {
		t.Run(tc.path, func(t *testing.T) {
			h := falHit(t, tc.path)
			b := h.Transport.Billing
			var body map[string]any
			if json.Unmarshal([]byte(tc.body), &body) != nil {
				t.Fatal(tc.body)
			}
			body["model"] = tc.path
			body["duration"] = "auto"
			ctx := b.Context(body)
			result := map[string]any{"video": map[string]any{"url": "https://example.org/video.mp4", "duration": 5.5}, "usage": map[string]any{"completion_tokens": 1000., "total_tokens": 99999.}, "metrics": map[string]any{"inference_time": 10000.}}
			for _, doc := range []map[string]any{result, {"status": "COMPLETED", "result": result}} {
				u := catalog.NormalizeUsage(b.Usage(doc, ctx))
				if u.OutputVariant != tc.variant {
					t.Fatal(u)
				}
				if tc.token {
					if u.CompletionTokens != 1000 || u.PromptTokens != 0 || u.Seconds != 0 {
						t.Fatal(u)
					}
				} else if u.Seconds != 5.5 || u.CompletionTokens != 0 {
					t.Fatal(u)
				}
				charge, ok := catalog.CostAt("qiniu/"+tc.path, u, time.Now())
				if !ok || len(charge.Applied) != 1 || charge.Applied[0].Fallback {
					t.Fatalf("incorrect seller binding %v %+v", ok, charge)
				}
				if tc.rate > 0 && math.Abs(charge.Total-tc.quantity*tc.rate) > 1e-8 {
					t.Fatal(charge)
				}
			}
		})
	}
}

// TestFalRejectsFailureAndUnmeasuredTime 验证等待、业务失败和缺少实际时长不能结算，缺少上下文不能猜最低价。
// 参数 t：Go 测试上下文。返回：无；断言失败时报告协议或计费契约回归。
// 调用：go test；使用本地注册表、价格目录或假上游，不创建真实付费任务。
func TestFalRejectsFailureAndUnmeasuredTime(t *testing.T) {
	b := falHit(t, "fal-ai/kling-video/v2.5-turbo/pro/text-to-video").Transport.Billing
	ctx := b.Context(map[string]any{"model": "fal-ai/kling-video/v2.5-turbo/pro/text-to-video", "duration": "10"})
	result := map[string]any{"video": map[string]any{"url": "https://example.org/video.mp4", "duration": 5.}}
	for _, doc := range []map[string]any{
		{"status": "COMPLETED", "detail": map[string]any{"message": "failed"}, "result": result},
		{"status": "IN_QUEUE", "result": result}, {"status": "IN_PROGRESS", "result": result},
		{"status": "FAILED", "result": result}, {"status": "COMPLETED"},
		{"video": map[string]any{"duration": 5.}},
		{"video": map[string]any{"url": "https://example.org/video.mp4"}, "metrics": map[string]any{"inference_time": 5.}},
	} {
		if u := b.Usage(doc, ctx); u != nil {
			t.Fatal("billed failure or guessed duration", u)
		}
	}
	u := catalog.NormalizeUsage(b.Usage(result, provider.TaskContext{}))
	if u.Seconds != 5 || u.OutputVariant != "unknown" {
		t.Fatal(u)
	}
	if _, ok := catalog.CostAt("qiniu/fal-ai/kling-video/v2.5-turbo/pro/text-to-video", u, time.Now()); ok {
		t.Fatal("guessed missing context")
	}
}

// TestFalIncompletePricingRemainsUnpricedWithUsage 验证区间及复合计价保留实测事实和待核价状态，人工扁平费率也不能掩盖缺失维度。
// 参数 t：Go 测试上下文。返回：无；断言失败时报告协议或计费契约回归。
// 调用：go test；使用本地注册表、价格目录或假上游，不创建真实付费任务。
func TestFalIncompletePricingRemainsUnpricedWithUsage(t *testing.T) {
	for _, path := range []string{"fal-ai/vidu/q2/text-to-video", "fal-ai/vidu/q2/image-to-video/turbo", "minimax/h3/text-to-video", "minimax/h3-max/reference-to-video"} {
		h := falHit(t, path)
		b := h.Transport.Billing
		ctx := b.Context(map[string]any{"model": path})
		doc := map[string]any{"video": map[string]any{"url": "https://example.org/video.mp4", "duration": 5.}, "usage": map[string]any{"input_seconds": 3., "input_image_count": 6.}}
		u := catalog.NormalizeUsage(b.Usage(doc, ctx))
		if u.Seconds != 5 || u.InputSeconds != 3 || u.InputImages != 6 || u.PricingBlocked == "" {
			t.Fatal(u)
		}
		if _, ok := catalog.CostAt("qiniu/"+path, u, time.Now()); ok {
			t.Fatal("flattened composite/tier price", path)
		}
		flat := []catalog.Rate{{Measure: "second", Side: "output", Window: "all", UnitSize: 1, USD: 0.1}}
		if _, ok := catalog.CostFromRates(flat, u, time.Now()); ok {
			t.Fatal("flat override hid incomplete quantities")
		}
		snapshot := catalog.SnapshotUsage(catalog.Charge{}, u, false)
		var parsed catalog.PriceSnapshot
		if json.Unmarshal([]byte(snapshot), &parsed) != nil || parsed.PricingStatus != "unpriced" || parsed.Usage.PricingBlocked == "" {
			t.Fatal(snapshot)
		}
	}
}

// TestFalSeedanceMissingContextRetainsTokensWithoutSecondsPricing 验证 Seedance 缺少任务上下文时仍保留完成 token，不改用秒数计价。
// 参数 t：Go 测试上下文。返回：无；断言失败时报告协议或计费契约回归。
// 调用：go test；使用本地注册表、价格目录或假上游，不创建真实付费任务。
func TestFalSeedanceMissingContextRetainsTokensWithoutSecondsPricing(t *testing.T) {
	const model = "bytedance/seedance-2.0/mini/text-to-video"
	b := falHit(t, model).Transport.Billing
	result := map[string]any{
		"video": map[string]any{"url": "https://example.org/video.mp4", "duration": 4.},
		"usage": map[string]any{"completion_tokens": 40594., "output_seconds": 4.},
	}
	u := catalog.NormalizeUsage(b.Usage(result, provider.TaskContext{}))
	if u.CompletionTokens != 40594 || u.Seconds != 0 || u.OutputVariant != "unknown" {
		t.Fatal("missing context changed the billing unit", u)
	}
	if _, ok := catalog.CostAt("qiniu/"+model, u, time.Now()); ok {
		t.Fatal("missing context guessed Seedance price")
	}
}
