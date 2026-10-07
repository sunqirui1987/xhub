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
	if !ok || qiniu.Type.ID != "qiniu_contents_generation" || qiniu.Type.ModelField != "model" {
		t.Fatalf("qiniu create %+v %v", qiniu.Type.ID, ok)
	}
	ark, ok := provider.Match("POST", "/api/v3/contents/generations/tasks", nil)
	if !ok || ark.Type.ID != "ark_contents_generation" || ark.Type.TaskID != "id" {
		t.Fatalf("ark create %+v", ark.Type.ID)
	}
	got, ok := provider.Match("GET", "/api/v3/contents/generations/tasks/cgt-1", nil)
	if !ok || got.Names["id"] != "cgt-1" || got.Action.Name != "get" {
		t.Fatalf("ark poll %+v %+v", got.Action, got.Names)
	}
	if _, ok := provider.Match("POST", "/v1/chat/completions", nil); ok {
		t.Fatal("adapted chat must not be claimed by bypass")
	}
}

func TestCustomEndpointMatchesOnlyItsDeployment(t *testing.T) {
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
	hit, ok := provider.Match("GET", "/api/v1/generate/record-info", []config.ModelEntry{dep})
	if !ok || hit.DeploymentName != "tripo-text" || hit.Action.TaskQuery != "taskId" {
		t.Fatalf("custom get %+v %v", hit, ok)
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
