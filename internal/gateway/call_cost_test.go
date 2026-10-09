package gateway

import (
	"testing"
	"time"

	"github.com/sunqirui1987/xhub/internal/catalog"
	"github.com/sunqirui1987/xhub/internal/config"
)

func billingPeak() time.Time {
	return time.Date(2026, 3, 2, 10, 0, 0, 0, time.FixedZone("CST", 8*60*60))
}

func billingOffpeak() time.Time {
	return time.Date(2026, 3, 2, 20, 0, 0, 0, time.FixedZone("CST", 8*60*60))
}

func TestDeploymentCostUsesWindowsCacheImagesAndSeconds(t *testing.T) {
	dep := config.ModelEntry{LiteLLMParams: map[string]any{
		"model":                           "upstream",
		"input_cost_per_token":            1.0,
		"output_cost_per_token":           2.0,
		"input_cost_per_token_peak":       3.0,
		"output_cost_per_token_peak":      4.0,
		"cache_read_input_token_cost":     0.25,
		"cache_creation_input_token_cost": 0.5,
		"output_cost_per_image":           5.0,
		"output_cost_per_second":          6.0,
	}}
	usage := catalog.Usage{PromptTokens: 10, CompletionTokens: 2, CachedTokens: 2, CacheWriteTokens: 1, Images: 1, Seconds: 2}
	off, ok := deploymentCost(dep, usage, billingOffpeak())
	if !ok || off.Total != 8+0.5+0.5+4+5+12 {
		t.Fatalf("offpeak charge %#v ok=%v", off, ok)
	}
	peak, ok := deploymentCost(dep, usage, billingPeak())
	if !ok || peak.Total != 24+0.5+0.5+8+5+12 {
		t.Fatalf("peak charge %#v ok=%v", peak, ok)
	}
}

func TestCallCostUsesCatalogBaseModelAndManualDoesNotFallThrough(t *testing.T) {
	const catalogID = "call-cost-base-model"
	catalog.SetModel(catalogID, map[string]any{
		"rates": []catalog.Rate{{Measure: "second", UnitSize: 1, Side: "output", Window: "all", USD: 2}},
	})
	t.Cleanup(func() { catalog.RemoveModel(catalogID) })

	dep := config.ModelEntry{
		ModelName:     "public",
		LiteLLMParams: map[string]any{"model": "upstream"},
		ModelInfo:     map[string]any{"pricing_source": "catalog", "base_model": catalogID},
	}
	s := &Server{Cfg: &config.Config{ModelList: []config.ModelEntry{dep}}}
	total, _, output, ok, charge := s.callCost("public", "|upstream", catalog.Usage{Seconds: 3}, billingPeak())
	if !ok || total != 6 || output != 6 || len(charge.Applied) != 1 {
		t.Fatalf("catalog base model total=%v output=%v ok=%v charge=%#v", total, output, ok, charge)
	}

	dep.ModelInfo["pricing_source"] = "manual"
	dep.LiteLLMParams["input_cost_per_token"] = 1.0
	s.Cfg.ModelList[0] = dep
	if total, _, _, ok, _ := s.callCost("public", "|upstream", catalog.Usage{Seconds: 3}, billingPeak()); ok || total != 0 {
		t.Fatalf("manual deployment fell through to catalog total=%v ok=%v", total, ok)
	}
}

func TestMeasuredVideoBandDoesNotFallThroughExplicitRates(t *testing.T) {
	dep := config.ModelEntry{ModelName: "video", LiteLLMParams: map[string]any{
		"model":                 "qiniu/bytedance/doubao-seedance-2-0-260128",
		"rates":                 []catalog.Rate{{Measure: "token", Side: "output", Variant: "wiv", Window: "all", USD: 1e-6}},
		"output_cost_per_token": 99.0,
	}}
	s := &Server{Cfg: &config.Config{ModelList: []config.ModelEntry{dep}}}
	u := catalog.Usage{CompletionTokens: 1000, OutputVariant: "woiv"}
	if _, ok := deploymentCost(dep, u, billingPeak()); ok {
		t.Fatal("incomplete table fell back to flat price")
	}
	if _, _, _, ok, _ := s.callCost("video", "|qiniu/bytedance/doubao-seedance-2-0-260128", u, billingPeak()); ok {
		t.Fatal("incomplete explicit rates fell through to catalog")
	}
}

func TestCallCostKeepsOriginalTaskModelAfterDeploymentEdit(t *testing.T) {
	const original = "seedance-original-task-model"
	const changed = "seedance-edited-deployment-model"
	catalog.SetModel(original, map[string]any{"rates": []catalog.Rate{{Measure: "token", Side: "output", Variant: "woiv", Window: "all", USD: 0.001}}})
	catalog.SetModel(changed, map[string]any{"rates": []catalog.Rate{{Measure: "token", Side: "output", Variant: "woiv", Window: "all", USD: 0.009}}})
	t.Cleanup(func() { catalog.RemoveModel(original); catalog.RemoveModel(changed) })
	dep := config.ModelEntry{ModelName: "task", LiteLLMParams: map[string]any{"model": changed}}
	s := &Server{Cfg: &config.Config{ModelList: []config.ModelEntry{dep}}}
	total, _, _, ok, _ := s.callCost("task", "|"+changed, catalog.Usage{CompletionTokens: 1000, OutputVariant: "woiv", PricingModel: original}, billingPeak())
	if !ok || total != 1 {
		t.Fatalf("task billed as edited deployment: %v %v", total, ok)
	}
}
