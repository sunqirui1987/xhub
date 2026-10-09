package openai

import (
	"encoding/json"
	"github.com/sunqirui1987/xhub/internal/provider"
	"math"
	"testing"
)

// TestVideoUsageOnlyCompleted 验证视频仅按成功实测秒数结算；参数 t 为上下文，覆盖失败和缺失，离线无数据清理。
func TestVideoUsageOnlyCompleted(t *testing.T) {
	for _, d := range []map[string]any{{"status": "queued", "seconds": 8}, {"status": "failed", "seconds": 8}, {"status": "completed"}, {"status": "completed", "seconds": 8, "error": "failed"}} {
		if videoUsage(d, provider.TaskContext{}) != nil {
			t.Fatalf("未成功任务产生账单: %v", d)
		}
	}
	for _, seconds := range []any{8, 8.0, "8", json.Number("8")} {
		if u := videoUsage(map[string]any{"status": "completed", "seconds": seconds}, provider.TaskContext{}); u["seconds"] != 8.0 {
			t.Fatalf("合法实测秒数 %v 未归一化: %v", seconds, u)
		}
	}
	for _, seconds := range []any{-1, "bad", "NaN", "Infinity", true, math.Inf(1)} {
		if u := videoUsage(map[string]any{"status": "completed", "seconds": seconds}, provider.TaskContext{}); u != nil {
			t.Fatalf("非法秒数 %v 产生用量: %v", seconds, u)
		}
	}
	if nativePublicPath("image_edit", "openai", "/v1/images/edits") != "/v1/images/edits" || nativePublicPath("bypass:openai-images", "openai", "/v1/images/generations") != "/bypass/openai/v1/images/generations" {
		t.Fatal("端点路径混淆")
	}
}
