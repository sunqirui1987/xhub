package provider_test

import (
	"testing"

	"github.com/sunqirui1987/xhub/internal/catalog"
	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/provider"

	_ "github.com/sunqirui1987/xhub/internal/provider/all"
)

func TestBypassPathsStayOnTheirProviders(t *testing.T) {
	qiniu, ok := provider.Match("POST", "/v3/contents/generations/tasks", nil)
	if !ok || qiniu.Transport.ID != "qiniu_contents_generation" || qiniu.Transport.ModelField != "model" {
		t.Fatalf("qiniu create %+v %v", qiniu.Transport.ID, ok)
	}
	ark, ok := provider.Match("POST", "/api/v3/contents/generations/tasks", nil)
	if !ok || ark.Transport.ID != "ark_contents_generation" || ark.Transport.TaskID != "id" {
		t.Fatalf("ark create %+v", ark.Transport.ID)
	}
	got, ok := provider.Match("GET", "/api/v3/contents/generations/tasks/cgt-1", nil)
	if !ok || got.Names["id"] != "cgt-1" || got.Action.Name != "get" {
		t.Fatalf("ark poll %+v %+v", got.Action, got.Names)
	}
	if _, ok := provider.Match("POST", "/v1/chat/completions", nil); ok {
		t.Fatal("adapted chat must not be claimed by bypass")
	}
}

// TestRegisteredTransportsAreTheOnlyBypassSource 钉住 bypass 的来源只有一处：
// 后台登记的转发方式。
//
// 部署上自带的路径表不再参与匹配。这段代码原来覆盖的是一条部署自带文档的自定义
// bypass，现在那条路关掉了，所以要断言它匹配不上。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestRegisteredTransportsAreTheOnlyBypassSource(t *testing.T) {
	dep := config.ModelEntry{
		ModelName: "tripo-text",
		LiteLLMParams: map[string]any{
			"endpoint": map[string]any{
				"kind":        "bypass",
				"model_field": "type",
				"task_id":     "data.task_id",
				"actions": []any{
					map[string]any{"name": "create", "method": "POST", "public_path": "/v2/openapi/task", "upstream_path": "/v2/openapi/task"},
					map[string]any{"name": "get", "method": "GET", "public_path": "/api/v1/generate/record-info", "upstream_path": "/api/v1/generate/record-info", "task_query": "taskId"},
				},
			},
		},
	}
	if _, ok := provider.Match("GET", "/api/v1/generate/record-info", []config.ModelEntry{dep}); ok {
		t.Fatal("a deployment document was accepted as a bypass source")
	}
	if _, ok := provider.Match("POST", "/v2/openapi/task", []config.ModelEntry{dep}); ok {
		t.Fatal("a deployment document was accepted as a bypass source")
	}

	if got := provider.OfficialID("qiniu", "qiniu/bytedance/doubao-seedance-2-0-260128"); got != "bytedance/doubao-seedance-2-0-260128" {
		t.Fatalf("official id %s", got)
	}
	in, out, ok := catalog.TokenRates("volcengine/doubao-seedance-2-0-260128")
	if !ok || in != 7.0/1_000_000 || out != in {
		t.Fatalf("rates %v %v %v", in, out, ok)
	}
	if provider.ModelEndpoints()["volcengine/doubao-seedance-2-0-260128"] != "ark_contents_generation" {
		t.Fatal("model did not preselect the ark endpoint type")
	}
}

func TestReadTaskID(t *testing.T) {
	doc := map[string]any{"data": map[string]any{"taskId": "suno-1"}}
	if got := provider.ReadTaskID(doc, "data.taskId"); got != "suno-1" {
		t.Fatalf("task %s", got)
	}
}
