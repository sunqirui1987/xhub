package models

import (
	"testing"

	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/store"
)

func TestAddedModelMatchesHandAdded(t *testing.T) {
	entry := addedModel("fennoai", "gpt-5.6-terra")
	if entry.ModelName != "gpt-5.6-terra" || entry.LiteLLMParams["model"] != "gpt-5.6-terra" {
		t.Fatalf("name %#v", entry)
	}
	if entry.LiteLLMParams["litellm_credential_name"] != "fennoai" || entry.LiteLLMParams["custom_llm_provider"] != "openai" {
		t.Fatalf("params %#v", entry.LiteLLMParams)
	}
	if _, ok := entry.LiteLLMParams["api_base"]; ok {
		t.Fatal("api_base belongs on the credential")
	}
	if _, ok := entry.ModelInfo["mode"]; ok || entry.ModelInfo["role"] != nil || entry.ModelInfo["builtin"] != nil {
		t.Fatalf("info %#v", entry.ModelInfo)
	}
	id, _ := entry.ModelInfo["id"].(string)
	if len(id) < len("model_") || id[:len("model_")] != "model_" {
		t.Fatal(id)
	}
}

func TestClearCopiedMode(t *testing.T) {
	row := store.ProxyModel{Info: map[string]any{"builtin": "fennoai", "mode": "responses"}}
	if !clearCopiedMode(&row) || row.Info["mode"] != nil {
		t.Fatal("copied mode stayed on the model")
	}
	row.Info["mode"] = "embedding"
	if clearCopiedMode(&row) || row.Info["mode"] != "embedding" {
		t.Fatal("an explicit mode was removed")
	}
	plain := store.ProxyModel{Info: map[string]any{"mode": "responses"}}
	if clearCopiedMode(&plain) {
		t.Fatal("a hand-added mode was treated as copied")
	}
}

func TestPlaygroundGroupSkipsProviderShellsAndUsesChat(t *testing.T) {
	list := []config.ModelEntry{
		{ModelName: "fennoai", ModelInfo: map[string]any{"role": "provider", "builtin": "fennoai", "wire_api": "responses", "mode": "responses"}},
		{ModelName: "qiniu", ModelInfo: map[string]any{"role": "provider", "builtin": "qiniu", "wire_api": "responses"}},
		{ModelName: "gpt-5.6-terra", LiteLLMParams: map[string]any{"custom_llm_provider": "openai", "litellm_credential_name": "fennoai"}},
		{ModelName: "gpt-5.6-luna", LiteLLMParams: map[string]any{"custom_llm_provider": "openai"}, ModelInfo: map[string]any{"id": "model_5e85e4e8398a"}},
	}
	got := playgroundGroups(list)
	if len(got) != 2 {
		t.Fatalf("groups %#v", got)
	}
	byName := map[string]string{}
	for _, row := range got {
		byName[row["model_group"].(string)] = row["mode"].(string)
	}
	if byName["gpt-5.6-terra"] != "chat" || byName["gpt-5.6-luna"] != "chat" {
		t.Fatalf("modes %#v", byName)
	}
	if _, ok := byName["fennoai"]; ok {
		t.Fatal("provider shell fennoai was listed")
	}
	if _, ok := byName["qiniu"]; ok {
		t.Fatal("provider shell qiniu was listed")
	}
}

func TestParseModelIDs(t *testing.T) {
	ids := ParseModelIDs([]byte(`{"data":[{"id":"gpt-4o"},{"id":""},{"id":"gpt-4o"},{"id":"qwen"}]}`))
	if len(ids) != 2 || ids[0] != "gpt-4o" || ids[1] != "qwen" {
		t.Fatalf("ids %#v", ids)
	}
	if ParseModelIDs([]byte(`{"data":[]}`)) != nil {
		t.Fatal("empty list should stay empty")
	}
}

func TestBuiltinsEnabled(t *testing.T) {
	t.Setenv("XHUB_BUILTIN_PROVIDERS", "")
	if !BuiltinsEnabled() {
		t.Fatal("unset switch should install builtins")
	}
	t.Setenv("XHUB_BUILTIN_PROVIDERS", "off")
	if BuiltinsEnabled() {
		t.Fatal("off should disable builtins")
	}
}

func TestParseCatalogKeepsCategoryAndPrices(t *testing.T) {
	body := []byte(`{"data":[{"id":"gpt-vision","category":"vision","input_price":1.25,"output_price":10},{"id":"deepseek/deepseek-v3","pricing":{"prompt":0.2,"completion":0.8}}]}`)
	items := ParseCatalog(body)
	if len(items) != 2 {
		t.Fatalf("%#v", items)
	}
	if items[0].Category != "vision" || items[0].InputPrice == nil || *items[0].InputPrice != 1.25 || items[0].OutputPrice == nil || *items[0].OutputPrice != 10 {
		t.Fatalf("vision card %#v", items[0])
	}
	if items[1].Category != "deepseek" || items[1].InputPrice == nil || *items[1].InputPrice != 0.2 || items[1].OutputPrice == nil || *items[1].OutputPrice != 0.8 {
		t.Fatalf("priced card %#v", items[1])
	}
	plain := ParseCatalog([]byte(`{"data":[{"id":"gpt-4o"}]}`))
	if len(plain) != 1 || plain[0].InputPrice != nil || plain[0].OutputPrice != nil || plain[0].Category != "llm" {
		t.Fatalf("missing price must stay empty %#v", plain)
	}
}

func TestFillFromCostMapUsesPriceData(t *testing.T) {
	item := CatalogModel{ID: "gpt-oss-120b", Category: "llm"}
	fillFromCostMap(&item)
	if item.InputPrice == nil || item.OutputPrice == nil || *item.InputPrice <= 0 || *item.OutputPrice <= 0 {
		t.Fatalf("price data missing for gpt-oss-120b in=%v out=%v", item.InputPrice, item.OutputPrice)
	}
	kept := 1.25
	item.InputPrice = &kept
	fillFromCostMap(&item)
	if *item.InputPrice != 1.25 {
		t.Fatalf("payload price was replaced: %v", *item.InputPrice)
	}
}

func TestModelsURL(t *testing.T) {
	if ModelsURL(BuiltinFenno, "https://api.fenno.ai") != "https://api.fenno.ai/v1/models" {
		t.Fatal(ModelsURL(BuiltinFenno, "https://api.fenno.ai"))
	}
	if ModelsURL(BuiltinQiniu, "https://api.qnaigc.com/v1/") != "https://api.qnaigc.com/v1/models" {
		t.Fatal(ModelsURL(BuiltinQiniu, "https://api.qnaigc.com/v1/"))
	}
}

func TestSlashedModelIDStaysTheModelName(t *testing.T) {
	const id = "deepseek/deepseek-v3.2-exp"
	entry := addedModel("qiniu", id)
	if entry.ModelName != id || entry.LiteLLMParams["model"] != id {
		t.Fatalf("%#v", entry)
	}
}
