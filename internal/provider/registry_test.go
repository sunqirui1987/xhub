package provider_test

import (
	"testing"

	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/provider"

	_ "github.com/sunqirui1987/xhub/internal/provider/all"
)

// TestSelectedCapabilitiesUsesDeclaredIDs 验证部署能力只来自显式 endpoint_types。
// 参数 t（*testing.T）：当前测试上下文。
// 返回：无。失败时说明能力声明被错误推断或放宽。
func TestSelectedCapabilitiesUsesDeclaredIDs(t *testing.T) {
	newRow := config.ModelEntry{ModelInfo: map[string]any{"endpoint_types": []any{"chat", "embedding"}}}
	if got := provider.SelectedCapabilities(newRow); len(got) != 2 || got[0] != "chat" || got[1] != "embedding" {
		t.Fatalf("new spelling selected %v", got)
	}

	unknown := config.ModelEntry{ModelInfo: map[string]any{"endpoint_types": []any{"image"}}}
	if got := provider.SelectedCapabilities(unknown); len(got) != 0 {
		t.Fatalf("unknown capability selected %v", got)
	}
	modeOnly := config.ModelEntry{ModelInfo: map[string]any{"mode": "embedding"}}
	if got := provider.SelectedCapabilities(modeOnly); len(got) != 0 {
		t.Fatalf("mode-only row selected %v", got)
	}
	// 空配置不声明任何能力；新系统不从缺失字段推断对话能力。
	plain := config.ModelEntry{}
	if got := provider.SelectedCapabilities(plain); len(got) != 0 {
		t.Fatalf("an empty model info selected %v, want no capabilities", got)
	}
}

func TestTransportRequiresExplicitRegisteredID(t *testing.T) {
	row := config.ModelEntry{ModelInfo: map[string]any{"mode": "ark_contents_generation", "endpoint_types": []string{"ark_contents_generation"}}}
	if got := provider.SelectedTransport(row); got != "" {
		t.Fatalf("unselected transport=%q", got)
	}
	row.ModelInfo["transport"] = "ark_contents_generation"
	if got := provider.SelectedTransport(row); got != "ark_contents_generation" {
		t.Fatalf("explicit transport=%q", got)
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

		}
	}
}
