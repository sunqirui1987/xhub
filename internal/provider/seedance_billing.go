package provider

import (
	"strings"

	"github.com/sunqirui1987/xhub/internal/logx"
)

// SeedanceBilling 按供应商报告的视频 token 结算，需要成功结果与创建时保存的参考视频事实。
// 参数：无。
// 返回 *TaskBilling：创建请求上下文与轮询结果计费提取器。
// 调用：volcengine 的 Seedance transport 注册。
// 测试：volcengine/seedance_test.go。
func SeedanceBilling() *TaskBilling {
	logx.Trace("provider Seedance task billing configured")
	return &TaskBilling{Context: seedanceContext, Usage: seedanceUsage}
}

// seedanceContext 保存请求模型、分辨率以及是否包含参考视频。
// 参数 body（map[string]any）：提交给供应商的任务请求。
// 返回 TaskContext：后续轮询结算所需的原始请求事实。
// 调用：SeedanceBilling 返回的 Context 回调。
// 测试：volcengine/seedance_test.go。
func seedanceContext(body map[string]any) TaskContext {
	model, _ := body["model"].(string)
	resolution, _ := body["resolution"].(string)
	if resolution == "" {
		resolution = "720p"
	}
	c := TaskContext{Model: model, Resolution: resolution, Known: true}
	content, _ := body["content"].([]any)
	for _, part := range content {
		item, _ := part.(map[string]any)
		if item["type"] == "video_url" {
			c.HasVideo = true
		}
	}
	return c
}

// seedanceUsage 只从成功任务中提取实测计价量。
// 参数 doc（map[string]any）：供应商轮询响应；c（TaskContext）：创建任务时保存的请求事实。
// 返回 map[string]any：可计价的完成 token、搜索次数及变体；失败或缺少用量时为空。
// 调用：SeedanceBilling 返回的 Usage 回调。
// 测试：volcengine/seedance_test.go。
func seedanceUsage(doc map[string]any, c TaskContext) map[string]any {
	if doc["status"] != "succeeded" || doc["error"] != nil {
		return nil
	}
	u, _ := doc["usage"].(map[string]any)
	if len(u) == 0 {
		return nil
	}
	// 只复制实测计价量。total_tokens 和请求时长不能代替 completion_tokens；此处不对输入 token 计价。
	out := map[string]any{"completion_tokens": u["completion_tokens"]}
	if tools, ok := doc["tool_usage"].(map[string]any); ok {
		out["searches"] = tools["web_search"]
	}
	// 缺少上下文时保持未定价，不能猜测任务没有参考视频。
	variant := "unknown"
	if c.Known {
		resolution := c.Resolution
		if actual, ok := doc["resolution"].(string); ok && actual != "" {
			resolution = actual
		}
		band := "woiv"
		if c.HasVideo {
			band = "wiv"
		}
		switch strings.ToLower(resolution) {
		case "480p", "720p":
			variant = band
		case "1080p", "4k":
			variant = strings.ToLower(resolution) + "_" + band + "_v"
		}
	}
	out["output_variant"] = variant
	return out
}
