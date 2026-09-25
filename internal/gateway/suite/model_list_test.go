package suite

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sunqirui1987/xhub/internal/catalog"
	"github.com/sunqirui1987/xhub/internal/config"
)

func TestOpenAICatalogMatchesLiteLLMPriceMap(t *testing.T) {
	models := catalog.ProviderModels("openai")
	if len(models) < 200 {
		t.Fatalf("openai catalog too small: %d", len(models))
	}
	got := map[string]struct{}{}
	for _, m := range models {
		got[m] = struct{}{}
	}
	// Chat finetune price rows are pricing-only and stay out of openai/*.
	// text-completion-openai rows such as ft:babbage-002 stay in, matching LiteLLM.
	if _, ok := got["ft:gpt-4o-2024-08-06"]; ok {
		t.Fatal("chat finetune leaked into openai set")
	}
	if _, ok := got["ft:babbage-002"]; !ok {
		t.Fatal("text-completion finetune missing from openai set")
	}
	for _, want := range []string{"gpt-4o", "gpt-4.1", "o3", "gpt-5", "gpt-5.4", "text-embedding-3-large", "dall-e-3"} {
		if _, ok := got[want]; !ok {
			t.Fatalf("missing %s", want)
		}
	}
	raw, _ := catalog.Raw().(map[string]any)
	if len(raw) < 3000 {
		t.Fatalf("cost map keys %d", len(raw))
	}
}

func TestModelCostMapSource(t *testing.T) {
	s, master := testEnv(t)
	h := s.Handler()

	denied := doJSON(t, h, "GET", "/model/cost_map/source", "", nil)
	if denied.Code != 401 {
		t.Fatalf("unauth %d %s", denied.Code, denied.Body.String())
	}

	rec := doJSON(t, h, "GET", "/model/cost_map/source", master, nil)
	if rec.Code != 200 {
		t.Fatalf("source %d %s", rec.Code, rec.Body.String())
	}
	body := decodeBody(t, rec.Body.Bytes())
	if body["source"] != "local" {
		t.Fatalf("source %v", body["source"])
	}
	if body["url"] != nil || body["etag"] != nil || body["source_revision"] != nil || body["fallback_reason"] != nil {
		t.Fatalf("unexpected provenance %s", rec.Body.String())
	}
	if body["is_env_forced"] != false {
		t.Fatalf("is_env_forced %v", body["is_env_forced"])
	}
	loadedAt, _ := body["loaded_at"].(string)
	if _, err := time.Parse(time.RFC3339, loadedAt); err != nil {
		t.Fatalf("loaded_at %q", loadedAt)
	}
	n, ok := body["model_count"].(float64)
	raw, _ := catalog.Raw().(map[string]any)
	want := len(raw)
	if _, hasSpec := raw["sample_spec"]; hasSpec {
		want--
	}
	if !ok || int(n) != want || want < 3000 {
		t.Fatalf("model_count %v want %d", body["model_count"], want)
	}

	t.Setenv("LITELLM_LOCAL_MODEL_COST_MAP", "True")
	forced := doJSON(t, h, "GET", "/model/cost_map/source", master, nil)
	if decodeBody(t, forced.Body.Bytes())["is_env_forced"] != true {
		t.Fatalf("forced %s", forced.Body.String())
	}
}

func TestModelCostMapReloadAndSchedule(t *testing.T) {
	s, master := testEnv(t)
	h := s.Handler()

	denied := doJSON(t, h, "POST", "/reload/model_cost_map", "", nil)
	if denied.Code != 401 {
		t.Fatalf("unauth reload %d %s", denied.Code, denied.Body.String())
	}

	rec := doJSON(t, h, "POST", "/reload/model_cost_map", master, nil)
	if rec.Code != 200 {
		t.Fatalf("reload %d %s", rec.Code, rec.Body.String())
	}
	body := decodeBody(t, rec.Body.Bytes())
	if body["status"] != "success" {
		t.Fatalf("reload status %v", body["status"])
	}
	n, _ := body["models_count"].(float64)
	if int(n) != catalog.Count() || catalog.Count() < 3000 {
		t.Fatalf("models_count %v want %d", body["models_count"], catalog.Count())
	}

	status := decodeBody(t, must200(t, h, "GET", "/schedule/model_cost_map_reload/status", master).Body.Bytes())
	if status["scheduled"] != false || status["last_run"] == nil {
		t.Fatalf("status after reload %v", status)
	}

	bad := doJSON(t, h, "POST", "/schedule/model_cost_map_reload?hours=0", master, nil)
	if bad.Code != 400 {
		t.Fatalf("bad hours %d %s", bad.Code, bad.Body.String())
	}

	ok := doJSON(t, h, "POST", "/schedule/model_cost_map_reload?hours=6", master, nil)
	if ok.Code != 200 || decodeBody(t, ok.Body.Bytes())["status"] != "success" {
		t.Fatalf("schedule %d %s", ok.Code, ok.Body.String())
	}
	scheduled := decodeBody(t, must200(t, h, "GET", "/schedule/model_cost_map_reload/status", master).Body.Bytes())
	if scheduled["scheduled"] != true || scheduled["interval_hours"] != float64(6) || scheduled["next_run"] == nil {
		t.Fatalf("scheduled %v", scheduled)
	}

	cancelled := doJSON(t, h, "DELETE", "/schedule/model_cost_map_reload", master, nil)
	if cancelled.Code != 200 || decodeBody(t, cancelled.Body.Bytes())["status"] != "success" {
		t.Fatalf("cancel %d %s", cancelled.Code, cancelled.Body.String())
	}
	after := decodeBody(t, must200(t, h, "GET", "/schedule/model_cost_map_reload/status", master).Body.Bytes())
	if after["scheduled"] != false || after["interval_hours"] != nil || after["last_run"] == nil {
		t.Fatalf("after cancel %v", after)
	}
}

func must200(t *testing.T, h http.Handler, method, path, token string) *httptest.ResponseRecorder {
	t.Helper()
	rec := doJSON(t, h, method, path, token, nil)
	if rec.Code != 200 {
		t.Fatalf("%s %s %d %s", method, path, rec.Code, rec.Body.String())
	}
	return rec
}

func TestModelsWildcardExpandsOpenAIAndAccessGroups(t *testing.T) {
	s, master := testEnv(t)
	base := s.Cfg.ModelList[0].LiteLLMParams["api_base"]
	s.Cfg.ModelList = append(s.Cfg.ModelList, config.ModelEntry{
		ModelName: "openai/*",
		LiteLLMParams: map[string]any{
			"model": "openai/*", "api_key": "sk-upstream", "api_base": base,
		},
		ModelInfo: map[string]any{"access_groups": []any{"beta-models"}},
	})
	s.Cfg.ModelList = append(s.Cfg.ModelList, config.ModelEntry{
		ModelName:     "gpt-4o-mini",
		LiteLLMParams: map[string]any{"model": "openai/gpt-4o-mini", "api_key": "k"},
		ModelInfo:     map[string]any{"access_groups": []any{"beta-models"}},
	})
	h := s.Handler()
	sk := mintLLM(t, s, master)

	all := doJSON(t, h, "GET", "/models", sk, nil)
	if all.Code != 200 {
		t.Fatal(all.Body.String())
	}
	body := all.Body.String()
	if !strings.Contains(body, `"id":"openai/gpt-4o"`) && !strings.Contains(body, `"id": "openai/gpt-4o"`) {
		t.Fatalf("wildcard did not expand openai/gpt-4o: %s", body[:min(400, len(body))])
	}
	if strings.Contains(body, `"id":"openai/*"`) || strings.Contains(body, `"id": "openai/*"`) {
		t.Fatal("wildcard route leaked without return_wildcard_routes")
	}
	if strings.Count(body, `"object":"model"`)+strings.Count(body, `"object": "model"`) < 200 {
		t.Fatalf("expanded list too small")
	}

	wild := doJSON(t, h, "GET", "/v1/models?return_wildcard_routes=True", sk, nil)
	if !strings.Contains(wild.Body.String(), "openai/*") {
		t.Fatalf("return_wildcard_routes missing openai/*: %s", wild.Body.String()[:min(300, wild.Body.Len())])
	}

	only := doJSON(t, h, "GET", "/models?include_model_access_groups=True&only_model_access_groups=True", sk, nil)
	if only.Code != 200 {
		t.Fatal(only.Body.String())
	}
	om := decodeBody(t, only.Body.Bytes())
	data, _ := om["data"].([]any)
	if len(data) != 1 {
		t.Fatalf("only access groups want [beta-models] got %s", only.Body.String())
	}
	row, _ := data[0].(map[string]any)
	if row["id"] != "beta-models" {
		t.Fatalf("access group id %v", row["id"])
	}

	withGroups := doJSON(t, h, "GET", "/models?include_model_access_groups=True", sk, nil)
	if !strings.Contains(withGroups.Body.String(), "beta-models") || !strings.Contains(withGroups.Body.String(), "openai/gpt-5") {
		t.Fatal("include_model_access_groups dropped group or openai models")
	}

	bad := doJSON(t, h, "GET", "/models?scope=nope", sk, nil)
	if bad.Code != 400 || !strings.Contains(bad.Body.String(), "expand") {
		t.Fatalf("bad scope %d %s", bad.Code, bad.Body.String())
	}

	chat := doJSON(t, h, "POST", "/v1/chat/completions", sk, map[string]any{
		"model": "openai/gpt-4.1", "messages": []any{map[string]any{"role": "user", "content": "hi"}},
	})
	if chat.Code != 200 {
		t.Fatalf("wildcard route chat %d %s", chat.Code, chat.Body.String())
	}
}

func TestKeyAllowlistStillHidesOtherModels(t *testing.T) {
	s, master := testEnv(t)
	s.Cfg.ModelList = append(s.Cfg.ModelList, config.ModelEntry{
		ModelName: "openai/*",
		LiteLLMParams: map[string]any{
			"model": "openai/*", "api_key": "sk-upstream", "api_base": "http://upstream.invalid",
		},
	})
	h := s.Handler()
	gen := doJSON(t, h, "POST", "/key/generate", master, map[string]any{
		"key_type": "llm_api", "models": []string{"gpt-4o-mini"},
	})
	sk := decodeBody(t, gen.Body.Bytes())["key"].(string)
	lst := doJSON(t, h, "GET", "/v1/models", sk, nil)
	body := lst.Body.String()
	if !strings.Contains(body, "gpt-4o-mini") || strings.Contains(body, "openai/gpt-4o") || strings.Contains(body, "dall-e-3") {
		t.Fatalf("allowlist leaked %s", body[:min(500, len(body))])
	}
}
