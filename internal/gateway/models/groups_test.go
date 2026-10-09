package models

import (
	"encoding/json"
	"github.com/sunqirui1987/xhub/internal/config"
	"strings"
	"testing"
)

// TestGroupedModelWeights 验证按精确公开名称聚合、供应商搜索返回完整组、规范部署 ID 和凭据脱敏。
// 参数 t 为测试上下文；前置纯内存目录和 3:7 默认权重，返回安全完整组，无外部清理。
func TestGroupedModelWeights(t *testing.T) {
	list := []config.ModelEntry{
		{ModelName: "shared", LiteLLMParams: map[string]any{"model": "vendor/full/model", "deployment_id": "a", "api_key": "secret-a", "litellm_credential_name": "account-a"}, ModelInfo: map[string]any{"id": "record-a"}},
		{ModelName: "shared", LiteLLMParams: map[string]any{"model": "different/model", "deployment_id": "b", "api_key": "secret-b", "litellm_credential_name": "account-b"}, ModelInfo: map[string]any{"id": "record-b"}},
		{ModelName: "Shared", LiteLLMParams: map[string]any{"model": "single", "deployment_id": "c"}},
	}
	defaults := map[string]any{"shared": map[string]any{"allocations": []any{map[string]any{"deployment_id": "a", "weight": 3}, map[string]any{"deployment_id": "b", "weight": 7}}}}
	groups, err := groupModels(list, defaults, "account-b")
	if err != nil || len(groups) != 1 || len(groups[0].Deployments) != 2 {
		t.Fatalf("搜索拆散模型组: %+v %v", groups, err)
	}
	if groups[0].Deployments[0]["id"] != "a" || groups[0].Default.Allocations[1].Weight != 7 {
		t.Fatalf("部署身份或默认权重错误: %+v", groups)
	}
	raw, _ := json.Marshal(groups)
	if strings.Contains(string(raw), "secret-") {
		t.Fatal("模型目录泄露凭据")
	}
	if !strings.Contains(string(raw), "vendor/full/model") {
		t.Fatal("型号前缀被修改")
	}
	all, err := groupModels(list, defaults, "")
	if err != nil || len(all) != 2 {
		t.Fatal("大小写不同模型被错误聚合")
	}
	empty, err := groupModels(list, defaults, "missing")
	if err != nil || len(empty) != 0 {
		t.Fatal("无匹配返回非空")
	}
	defaults["shared"] = map[string]any{"allocations": []any{map[string]any{"deployment_id": "a", "weight": -1}}}
	if _, err = groupModels(list, defaults, ""); err == nil {
		t.Fatal("非法默认未返回错误")
	}
}
