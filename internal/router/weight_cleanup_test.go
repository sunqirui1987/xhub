package router

import (
	"github.com/sunqirui1987/xhub/internal/config"
	"reflect"
	"testing"
)

// TestCleanAllocations 验证删除、改名及单部署清理，不因禁用或零权重改变仍有效的分配。
// 前置为内存目录；正常、空目录、悬空及全零输入均验证，函数无外部资源，无需清理。
func TestCleanAllocations(t *testing.T) {
	directory := []config.ModelEntry{
		{ModelName: "shared", ModelInfo: map[string]any{"id": "a"}},
		{ModelName: "shared", ModelInfo: map[string]any{"id": "b", "disabled": true}},
		{ModelName: "other", ModelInfo: map[string]any{"id": "c"}},
	}
	for _, tc := range []struct {
		name  string
		rows  []Allocation
		names []string
		dir   []config.ModelEntry
		want  []Allocation
	}{
		{"保持有效零权重", []Allocation{{"a", 0}, {"b", 2}, {"gone", 9}}, []string{"shared"}, directory, []Allocation{{"a", 0}, {"b", 2}}},
		{"完整全零恢复默认", []Allocation{{"a", 0}, {"b", 0}}, []string{"shared"}, directory, nil},
		{"未配置部署仍默认一", []Allocation{{"a", 0}}, []string{"shared"}, directory, []Allocation{{"a", 0}}},
		{"单部署清空", []Allocation{{"a", 0}, {"b", 1}}, []string{"shared"}, directory[:1], nil},
		{"空目录清空", []Allocation{{"gone", 1}}, []string{"shared"}, nil, nil},
		{"改名清除旧引用", []Allocation{{"a", 1}, {"c", 2}}, []string{"shared"}, directory, []Allocation{{"a", 1}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := CleanAllocations(tc.rows, tc.names, tc.dir); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("清理分配=%v，期望%v", got, tc.want)
			}
		})
	}
}

// TestCleanTemplateWeights 验证模板与组独立复制、字段保留以及损坏结构返回错误而非崩溃。
// 前置为纯内存文档及单部署目录；覆盖正常和失败输入，原文档不变，无外部资源。
func TestCleanTemplateWeights(t *testing.T) {
	directory := []config.ModelEntry{{ModelName: "shared", ModelInfo: map[string]any{"id": "a"}}}
	allocation := []any{map[string]any{"deployment_id": "gone", "weight": 1}}
	doc := map[string]any{"model_routes": []any{map[string]any{"model": "shared", "strategy": "traffic-split", "allocations": allocation}}, "routing_groups": []any{map[string]any{"models": []any{"shared"}, "routing_strategy_args": map[string]any{"allocations": allocation}}}, "retry_policy": map[string]any{"max_attempts": 2}}
	clean, err := CleanTemplateWeights(doc, directory)
	if err != nil {
		t.Fatal(err)
	}
	if clean["model_routes"].([]any)[0].(map[string]any)["allocations"] != nil {
		t.Fatal("单部署模板仍保留权重")
	}
	if clean["routing_groups"].([]any)[0].(map[string]any)["routing_strategy_args"].(map[string]any)["allocations"] != nil {
		t.Fatal("组仍引用已删部署")
	}
	if doc["model_routes"].([]any)[0].(map[string]any)["allocations"] == nil {
		t.Fatal("清理修改了原文档")
	}
	if !reflect.DeepEqual(clean["retry_policy"], map[string]any{"max_attempts": float64(2)}) {
		t.Fatal("清理改变重试设置")
	}
	for _, bad := range []map[string]any{
		{"model_routes": "bad"}, {"model_routes": []any{nil}},
		{"routing_groups": []any{map[string]any{"models": nil}}},
		{"routing_groups": []any{map[string]any{"models": []any{7}}}},
		{"model_routes": []any{map[string]any{"model": "shared", "allocations": "bad"}}},
		{"unsupported": func() {}},
	} {
		if _, err := CleanTemplateWeights(bad, directory); err == nil {
			t.Fatalf("损坏正文未报错: %v", bad)
		}
	}
}
