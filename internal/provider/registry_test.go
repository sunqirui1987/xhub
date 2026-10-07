package provider_test

import (
	"testing"

	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/provider"

	_ "github.com/sunqirui1987/xhub/internal/provider/all"
)

func TestOneModelCanSelectSeveralEndpointTypes(t *testing.T) {
	gpt := config.ModelEntry{ModelInfo: map[string]any{"endpoint_types": []any{"chat", "completion"}}}
	got := provider.SelectedTypes(gpt)
	if len(got) != 2 || got[0] != "chat" || got[1] != "completion" {
		t.Fatalf("selected %v", got)
	}
	if !provider.Includes(gpt, "chat") || provider.Includes(gpt, "ark_contents_generation") {
		t.Fatal("includes did not follow the selection")
	}
	bound := provider.BoundTypes(gpt)
	if len(bound) != 2 || bound[0].Kind != provider.KindAdapted || bound[1].Actions[0].PublicPath != "/v1/completions" {
		t.Fatalf("bound %+v", bound)
	}
	seedance := config.ModelEntry{ModelInfo: map[string]any{"endpoint_types": []any{"ark_contents_generation"}}}
	paths := provider.BoundTypes(seedance)
	if len(paths) != 1 || paths[0].ID != "ark_contents_generation" || len(paths[0].Actions) != 3 {
		t.Fatalf("seedance bound %+v", paths)
	}
}

func TestBypassTypesStayWithTheirProvider(t *testing.T) {
	for _, typ := range provider.Types() {
		switch typ.ID {
		case "chat":
			if len(typ.Providers) != 0 {
				t.Fatal("adapted chat is available to every provider")
			}
		case "ark_contents_generation":
			if len(typ.Providers) != 1 || typ.Providers[0] != "volcengine" {
				t.Fatalf("ark providers %v", typ.Providers)
			}
		case "qiniu_contents_generation":
			if len(typ.Providers) != 1 || typ.Providers[0] != "qiniu" {
				t.Fatalf("qiniu providers %v", typ.Providers)
			}
		}
	}
}

func TestCustomBypassReadsTheDocumentFields(t *testing.T) {
	tripo := config.ModelEntry{
		ModelName: "text_to_model",
		ModelInfo: map[string]any{"endpoint_types": []any{"custom"}},
		LiteLLMParams: map[string]any{"endpoint": map[string]any{
			"kind": "bypass", "model_field": "type", "task_id": "data.task_id",
			"actions": []any{map[string]any{
				"name": "create", "method": "POST",
				"public_path": "/v2/openapi/task", "upstream_path": "/v2/openapi/task",
			}},
		}},
	}
	hit, ok := provider.Match("POST", "/v2/openapi/task", []config.ModelEntry{tripo})
	if !ok || hit.DeploymentName != "text_to_model" || hit.Type.ModelField != "type" || hit.Type.TaskID != "data.task_id" {
		t.Fatalf("tripo %+v %v", hit, ok)
	}
	if got := provider.ReadTaskID(map[string]any{"data": map[string]any{"task_id": "task-1"}}, hit.Type.TaskID); got != "task-1" {
		t.Fatalf("tripo task %s", got)
	}
	suno := config.ModelEntry{
		ModelName: "V6",
		LiteLLMParams: map[string]any{"endpoint": map[string]any{
			"kind": "bypass", "model_field": "model", "task_id": "data.taskId",
			"actions": []any{map[string]any{
				"name": "get", "method": "GET", "task_query": "taskId",
				"public_path": "/api/v1/generate/record-info", "upstream_path": "/api/v1/generate/record-info",
			}},
		}},
	}
	got, ok := provider.Match("GET", "/api/v1/generate/record-info", []config.ModelEntry{suno})
	if !ok || got.Action.TaskQuery != "taskId" || got.DeploymentName != "V6" {
		t.Fatalf("suno %+v %v", got, ok)
	}
}
