package provider_test

import (
	"testing"

	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/provider"

	_ "github.com/sunqirui1987/xhub/internal/provider/all"
)

func TestSelectedCapabilitiesReadsBothSpellings(t *testing.T) {
	// 新写入：能力 id 列表。
	newRow := config.ModelEntry{ModelInfo: map[string]any{"endpoint_types": []any{"chat", "embedding"}}}
	if got := provider.SelectedCapabilities(newRow); len(got) != 2 || got[0] != "chat" || got[1] != "embedding" {
		t.Fatalf("new spelling selected %v", got)
	}

	// 旧行：只有 mode，值是存量类型 id。image_generation 和 image_edit 都收成 image，
	// 所以要在这里核对映射，而不只是核对透传。
	legacy := config.ModelEntry{ModelInfo: map[string]any{"mode": "image_generation"}}
	if got := provider.SelectedCapabilities(legacy); len(got) != 1 || got[0] != "image" {
		t.Fatalf("legacy spelling selected %v", got)
	}

	// 空配置默认 chat：老的部署和不带端点信息的部署都是这个意思。
	plain := config.ModelEntry{}
	if got := provider.SelectedCapabilities(plain); len(got) != 1 || got[0] != "chat" {
		t.Fatalf("an empty model info selected %v, want chat", got)
	}
}

func TestOneModelCanAnswerSeveralCapabilities(t *testing.T) {
	both := config.ModelEntry{ModelInfo: map[string]any{"endpoint_types": []any{"chat", "embedding"}}}
	if !provider.IncludesCapability(both, "chat") || !provider.IncludesCapability(both, "embedding") {
		t.Fatal("a model with two capabilities did not report both")
	}
	if provider.IncludesCapability(both, "image") {
		t.Fatal("a model answered a capability it never declared")
	}
}

func TestBypassTypesStayWithTheirProvider(t *testing.T) {
	for _, typ := range provider.Transports() {
		switch typ.ID {
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

// TestABypassIsNeverTakenFromADeploymentDocument 钉住这次拆分的一个决定：
// bypass 只认后台登记的形状，不读部署上自带的路径表。
//
// 理由是它根本做不成：一份随手填进去的路径表发不出请求，也就拿不到上游的返回值，
// 那么预选、日志、用量都没有依据。上游跟 chat、image、video 一样必须是固定的，
// 只是它走原样转发。
//
// 旧的实现读 litellm_params.endpoint；这个测试保证那条路已经关掉。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestABypassIsNeverTakenFromADeploymentDocument(t *testing.T) {
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
	if _, ok := provider.Match("POST", "/v2/openapi/task", []config.ModelEntry{tripo}); ok {
		t.Fatal("a bypass path was taken from a deployment document")
	}
	// 而且它不会因此被当成 bypass：它仍旧走协议适配。
	if got := provider.SelectedTransport(tripo); got != provider.AdaptedTransportID {
		t.Fatalf("a deployment with a stale endpoint document reports transport %q", got)
	}
}
