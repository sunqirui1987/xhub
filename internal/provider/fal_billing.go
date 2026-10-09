package provider

import (
	"math"
	"strings"
)

// FalModel 保存已登记 Fal 端点的固定计价事实。
// Seedance 按实测视频 token 计价；其他模型可按实测秒数和分辨率、音频等规格计价。
// PricingBlocked 表示目录暂时不能表达的复合规则，禁止猜测价格。
type FalModel struct {
	Seedance       bool
	Resolution     string
	Variant        string
	AudioDefault   bool
	PricingBlocked string
}

// FalBilling 建立 Fal 队列任务的上下文与实测用量提取器。
// 参数 models（map[string]FalModel）：路径模型 ID 到计价规格的映射。
// 返回 *TaskBilling：创建时保存分辨率、参考视频和音频事实，成功结果按供应商实测量提取。
// 调用：qiniu.registerFal。测试：qiniu/billing_test.go、dataplane/fal_test.go。
func FalBilling(models map[string]FalModel) *TaskBilling {
	seedanceFamily := len(models) > 0
	for _, spec := range models {
		seedanceFamily = seedanceFamily && spec.Seedance
	}
	return &TaskBilling{
		Context: func(body map[string]any) TaskContext {
			model, _ := body["model"].(string)
			spec, known := models[model]
			resolution, _ := body["resolution"].(string)
			if resolution == "" {
				resolution = spec.Resolution
			}
			resolution = strings.ToLower(resolution)
			c := TaskContext{Model: model, Resolution: resolution, Known: known, Variant: spec.Variant, PricingBlocked: spec.PricingBlocked}
			for _, key := range []string{"video_urls", "reference_video_urls"} {
				if a, ok := body[key].([]any); ok && len(a) > 0 {
					c.HasVideo = true
				}
			}
			if v, ok := body["video_url"].(string); ok && v != "" {
				c.HasVideo = true
			}
			if spec.Seedance {
				return c
			}
			audio := spec.AudioDefault
			if v, ok := body["generate_audio"].(bool); ok {
				audio = v
			}
			c.Variant = strings.ReplaceAll(c.Variant, "{resolution}", resolution)
			av := "v"
			if audio {
				av = "av"
			}
			c.Variant = strings.ReplaceAll(c.Variant, "{audio}", av)
			ref := "norefv"
			if c.HasVideo {
				ref = "refv"
			}
			c.Variant = strings.ReplaceAll(c.Variant, "{ref}", ref)
			voice := "novoice"
			if a, ok := body["voice_ids"].([]any); ok && len(a) > 0 {
				voice = "voice"
			}
			sound := ref + "_v"
			if audio {
				sound = voice + "_av"
			}
			c.Variant = strings.ReplaceAll(c.Variant, "{sound}", sound)
			if strings.HasPrefix(model, "fal-ai/veo3.1") && resolution == "4k" {
				c.Variant = "4k_" + av + "_duration"
			}
			return c
		},
		Usage: func(doc map[string]any, c TaskContext) map[string]any {
			result := falResult(doc)
			if result == nil {
				return nil
			}
			spec, known := models[c.Model]
			if spec.Seedance || seedanceFamily {
				normalized := map[string]any{"status": "succeeded", "usage": result["usage"], "resolution": result["resolution"], "tool_usage": result["tool_usage"]}
				return seedanceUsage(normalized, c)
			}
			video, _ := result["video"].(map[string]any)
			seconds, _ := video["duration"].(float64)
			u, _ := result["usage"].(map[string]any)
			if n, ok := u["output_seconds"].(float64); ok {
				seconds = n
			}
			if seconds <= 0 || math.IsNaN(seconds) || math.IsInf(seconds, 0) {
				return nil
			}
			variant := c.Variant
			if !known || !c.Known || variant == "" {
				variant = "unknown"
			}
			out := map[string]any{"seconds": seconds, "output_variant": variant}
			// 即使当前目录不能表达复合计价，也保留实测数量供后续补价和审计。
			for _, key := range []string{"input_seconds", "input_image_count"} {
				if u[key] != nil {
					out[key] = u[key]
				}
			}
			if c.PricingBlocked != "" {
				out["pricing_blocked"] = c.PricingBlocked
			}
			return out
		},
	}
}

// falResult 从队列包装或直接结果中提取已成功生成的视频。
// 参数 doc（map[string]any）：供应商响应。返回 map[string]any：有视频 URL 的完成结果，未完成或失败返回 nil。
// COMPLETED 也可能携带生成错误，因此必须同时检查外层、内层错误和产物。
// 调用：FalBilling.Usage。测试：qiniu/billing_test.go。
func falResult(doc map[string]any) map[string]any {
	if doc == nil || doc["error"] != nil || doc["detail"] != nil {
		return nil
	}
	if status, _ := doc["status"].(string); status != "" && status != "COMPLETED" {
		return nil
	}
	result := doc
	if r, ok := doc["result"].(map[string]any); ok {
		result = r
	}
	if result["error"] != nil || result["detail"] != nil {
		return nil
	}
	video, _ := result["video"].(map[string]any)
	url, _ := video["url"].(string)
	if strings.TrimSpace(url) == "" {
		return nil
	}
	return result
}
