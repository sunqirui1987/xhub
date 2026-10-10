package models

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/sunqirui1987/xhub/internal/config"
)

func TestProxyModelNamesSkipsProviderShells(t *testing.T) {
	names := proxyModelNames([]config.ModelEntry{
		{ModelName: "qiniu", ModelInfo: map[string]any{"role": "provider"}},
		{ModelName: "fennoai", ModelInfo: map[string]any{"role": "provider"}},
		{ModelName: "gpt-5.5", ModelInfo: map[string]any{"role": "model"}},
	})
	if len(names) != 1 || names[0] != "gpt-5.5" {
		t.Fatalf("provider shells leaked into the model list: %v", names)
	}
}

func TestProxyModelNamesDoesNotInferRolesFromNames(t *testing.T) {
	names := proxyModelNames([]config.ModelEntry{
		{ModelName: "qiniu"},
		{ModelName: "fennoai"},
		{ModelName: "GPT-5.5"},
	})
	if len(names) != 3 {
		t.Fatalf("model names incorrectly inferred provider roles: %v", names)
	}
}

func TestNonModelEntryKeepsOrdinaryModels(t *testing.T) {
	if nonModelEntry(config.ModelEntry{ModelName: "gpt-5.5"}) {
		t.Fatal("ordinary model was classified as a provider entry")
	}
}

func TestProxyModelNamesOmitsOnlyFullyDisabledNames(t *testing.T) {
	names := proxyModelNames([]config.ModelEntry{
		{ModelName: "mixed", ModelInfo: map[string]any{"disabled": true}},
		{ModelName: "mixed", ModelInfo: map[string]any{"disabled": false}},
		{ModelName: "off", ModelInfo: map[string]any{"disabled": true}},
	})
	if len(names) != 1 || names[0] != "mixed" {
		t.Fatalf("disabled deployments leaked into public names: %v", names)
	}
}

// TestPublicModelListShape 验证公开响应的正常与空列表边界；参数为测试对象，无返回。
// 前置仅模型名称，断言标准字段且没有凭证，不写数据库，内存响应自动清理。
func TestPublicModelListShape(t *testing.T) {
	for _, names := range [][]string{nil, {"public-model"}} {
		w := httptest.NewRecorder()
		writeModelList(w, names)
		var got struct{ Data []map[string]any }
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if got.Data == nil || len(got.Data) != len(names) {
			t.Fatalf("公开列表空数组或数量错误: %s", w.Body.String())
		}
		for _, row := range got.Data {
			if len(row) != 4 || row["id"] != names[0] {
				t.Fatalf("公开目录泄漏字段或名称错误: %v", row)
			}
		}
	}
}
