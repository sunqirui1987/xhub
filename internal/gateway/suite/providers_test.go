package suite

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/llm"
	"github.com/sunqirui1987/xhub/internal/llm/protocol"
)

func catalogProviderPackages(t *testing.T) []string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "docs", "_inventory", "catalog.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cat struct {
		Providers []string `json:"providers"`
	}
	if err := json.Unmarshal(raw, &cat); err != nil {
		t.Fatal(err)
	}
	if len(cat.Providers) != 136 {
		t.Fatalf("catalog providers %d want 136", len(cat.Providers))
	}
	return cat.Providers
}

func TestAllCatalogProvidersHaveAdapter(t *testing.T) {
	names := catalogProviderPackages(t)
	s, master := testEnv(t)
	h := s.Handler()
	gen := doJSON(t, h, "POST", "/key/generate", master, map[string]any{"key_type": "llm_api"})
	sk := decodeBody(t, gen.Body.Bytes())["key"].(string)
	base := s.Cfg.ModelList[0].LiteLLMParams["api_base"]

	if _, ok := protocol.ProtocolGroup("not-a-provider"); ok {
		t.Fatal("unknown provider must not be treated as an adapter")
	}
	for _, pkg := range names {
		if _, ok := protocol.ProtocolGroup(pkg); !ok {
			t.Fatalf("ProtocolGroup(%q) missing", pkg)
		}
		if !llm.ChatProbeUsesFixture(pkg) {
			up, err := llm.Build(t.Context(), llm.Request{
				Op: "chat", Provider: pkg, APIBase: "https://upstream.example", APIKey: "sk-fake", Model: "m",
				Body: map[string]any{"messages": []any{map[string]any{"role": "user", "content": "hi"}}},
			})
			if pkg == "deprecated_providers" || pkg == "base_llm" || pkg == "custom_httpx" || pkg == "pass_through" {
				if err == nil {
					t.Fatalf("%s should not build an openai chat url", pkg)
				}
				continue
			}
			if err != nil {
				t.Fatalf("%s build: %v", pkg, err)
			}
			if strings.HasSuffix(up.URL, "/chat/completions") {
				t.Fatalf("%s must not post OpenAI chat JSON to /chat/completions (%s)", pkg, up.URL)
			}
			continue
		}
		alias := "prov-" + pkg
		s.LockModels()
		s.Cfg.ModelList = append(s.Cfg.ModelList, config.ModelEntry{
			ModelName: alias,
			LiteLLMParams: map[string]any{
				"model":               pkg + "/e2e-model",
				"custom_llm_provider": pkg,
				"api_key":             "sk-fake",
				"api_base":            base,
			},
		})
		s.UnlockModels()
		rec := doJSON(t, h, "POST", "/v1/chat/completions", sk, map[string]any{
			"model": alias,
			"messages": []any{
				map[string]any{"role": "user", "content": "hi"},
			},
		})
		if rec.Code == 400 && strings.Contains(rec.Body.String(), "provider_not_implemented") {
			t.Fatalf("%s provider_not_implemented %s", pkg, rec.Body.String())
		}
		if rec.Code != 200 {
			t.Fatalf("%s want 200 got %d %s", pkg, rec.Code, rec.Body.String())
		}
		raw := rec.Body.String()
		if !strings.Contains(raw, "choices") && !strings.Contains(raw, "content") && !strings.Contains(raw, "candidates") {
			t.Fatalf("%s empty dataplane body %s", pkg, raw)
		}
	}
}
