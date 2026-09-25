package suite

import (
	"encoding/json"
	"github.com/sunqirui1987/xhub/internal/gateway"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/gateway/prefs"
	"github.com/sunqirui1987/xhub/internal/router"
	"github.com/sunqirui1987/xhub/internal/store"
)

func TestOpenRejectsSQLiteAndExampleIsPostgres(t *testing.T) {
	if _, err := store.Open("sqlite://./xhub.db"); err == nil || !strings.Contains(err.Error(), "sqlite") {
		t.Fatalf("sqlite open err %v", err)
	}
	raw, err := os.ReadFile("../../../configs/config.example.yaml")
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if strings.Contains(text, "sqlite://") || !strings.Contains(text, "postgres://") {
		t.Fatalf("example config is not postgres:\n%s", text)
	}
}

func TestRouterSettingsRoundTripPostgres(t *testing.T) {
	url := testDatabaseURL(t)
	yamlGroup := map[string]any{"group_name": "yaml-group", "models": []any{"gpt-4o"}, "routing_strategy": "simple-shuffle"}
	cfg := func() *config.Config {
		return &config.Config{
			RouterSettings: config.RouterSettings{RoutingStrategy: "simple-shuffle", NumRetries: 2, Timeout: 60},
			RouterRaw: map[string]any{
				"routing_strategy": "simple-shuffle",
				"routing_groups":   []any{yamlGroup},
				"num_retries":      2,
			},
			GeneralSettings: config.GeneralSettings{MasterKey: "sk-master", DatabaseURL: url},
		}
	}
	open := func() *gateway.Server {
		t.Helper()
		st, err := store.Open(url)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = st.DB.Close() })
		return gateway.New(cfg(), st)
	}
	s := open()
	h := s.Handler()
	before := doJSON(t, h, "GET", "/router/settings", "sk-master", nil)
	if before.Code != 200 || !strings.Contains(before.Body.String(), "yaml-group") {
		t.Fatalf("yaml group missing %d %s", before.Code, before.Body.String())
	}

	created := doJSON(t, h, "POST", "/config/update", "sk-master", map[string]any{
		"router_settings": map[string]any{
			"routing_groups": []any{map[string]any{
				"group_name": "prod-group", "models": []any{"gpt-4o", "claude-sonnet"}, "routing_strategy": "least-busy",
			}},
		},
	})
	if created.Code != 200 {
		t.Fatalf("update %d %s", created.Code, created.Body.String())
	}
	assertGroup := func(body string) {
		t.Helper()
		if !strings.Contains(body, "prod-group") || !strings.Contains(body, "claude-sonnet") || !strings.Contains(body, "least-busy") {
			t.Fatalf("group missing %s", body)
		}
		if strings.Contains(body, "yaml-group") {
			t.Fatalf("yaml group was not overridden %s", body)
		}
	}
	settings := doJSON(t, h, "GET", "/router/settings", "sk-master", nil)
	assertGroup(settings.Body.String())
	if !strings.Contains(settings.Body.String(), `"current_values"`) {
		t.Fatal(settings.Body.String())
	}
	cb := doJSON(t, h, "GET", "/get/config/callbacks", "sk-master", nil)
	assertGroup(cb.Body.String())

	partial := doJSON(t, h, "POST", "/config/update", "sk-master", map[string]any{
		"router_settings": map[string]any{"num_retries": 4, "routing_strategy": "latency-based-routing"},
	})
	if partial.Code != 200 {
		t.Fatal(partial.Body.String())
	}
	after := doJSON(t, h, "GET", "/router/settings", "sk-master", nil)
	assertGroup(after.Body.String())
	if !strings.Contains(after.Body.String(), `"num_retries":4`) && !strings.Contains(after.Body.String(), `"num_retries": 4`) {
		t.Fatalf("retries %s", after.Body.String())
	}

	fb := doJSON(t, h, "POST", "/config/update", "sk-master", map[string]any{
		"router_settings": map[string]any{"fallbacks": []any{map[string]any{"gpt-4o": []any{"gpt-4o-mini"}}}},
	})
	if fb.Code != 200 || !strings.Contains(doJSON(t, h, "GET", "/get/config/callbacks", "sk-master", nil).Body.String(), "gpt-4o-mini") {
		t.Fatalf("fallbacks %s", fb.Body.String())
	}
	if !strings.Contains(doJSON(t, h, "GET", "/router/settings", "sk-master", nil).Body.String(), "prod-group") {
		t.Fatal("group lost after fallback update")
	}

	upd := doJSON(t, h, "POST", "/config/field/update", "sk-master", map[string]any{
		"field_name": "enable_anthropic_prompt_caching", "field_value": true, "config_type": "general_settings",
	})
	if upd.Code != 200 {
		t.Fatal(upd.Body.String())
	}
	list := doJSON(t, h, "GET", "/config/list?config_type=general_settings", "sk-master", nil)
	if !strings.Contains(list.Body.String(), `"field_name":"enable_anthropic_prompt_caching"`) && !strings.Contains(list.Body.String(), `"field_name": "enable_anthropic_prompt_caching"`) {
		t.Fatalf("list %s", list.Body.String())
	}
	if !strings.Contains(list.Body.String(), `"stored_in_db":true`) && !strings.Contains(list.Body.String(), `"stored_in_db": true`) {
		t.Fatalf("stored_in_db %s", list.Body.String())
	}
	del := doJSON(t, h, "POST", "/config/field/delete", "sk-master", map[string]any{
		"field_name": "enable_anthropic_prompt_caching", "config_type": "general_settings",
	})
	if del.Code != 200 {
		t.Fatal(del.Body.String())
	}
	reset := doJSON(t, h, "GET", "/config/list?config_type=general_settings", "sk-master", nil)
	if strings.Contains(reset.Body.String(), `"field_name":"enable_anthropic_prompt_caching"`) {
		// stored_in_db for that row must be null, not true
	}
	if strings.Contains(reset.Body.String(), "enable_anthropic_prompt_caching") && strings.Count(reset.Body.String(), `"stored_in_db":true`) > 0 && !strings.Contains(reset.Body.String(), `"field_value":false`) {
		// checked below more carefully
	}
	rows := decodeList(t, reset.Body.Bytes())
	var found bool
	for _, row := range rows {
		if row["field_name"] != "enable_anthropic_prompt_caching" {
			continue
		}
		found = true
		if row["stored_in_db"] != nil {
			t.Fatalf("reset stored_in_db %#v", row["stored_in_db"])
		}
	}
	if !found {
		t.Fatal("prompt caching field missing")
	}

	s2 := open()
	again := doJSON(t, s2.Handler(), "GET", "/router/settings", "sk-master", nil)
	assertGroup(again.Body.String())
}

func decodeList(t *testing.T, raw []byte) []map[string]any {
	t.Helper()
	var rows []map[string]any
	if err := json.Unmarshal(raw, &rows); err != nil {
		t.Fatal(err)
	}
	return rows
}

func TestRedisRouterAndSpendVisibleToSecondProcess(t *testing.T) {
	redisURL := os.Getenv("XHUB_TEST_REDIS_URL")
	if redisURL == "" {
		redisURL = "redis://127.0.0.1:6379/1"
	}
	url := testDatabaseURL(t)
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		model, _ := body["model"].(string)
		if model == "a" {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"down"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "ok", "object": "chat.completion", "model": model,
			"choices": []any{map[string]any{"index": 0, "message": map[string]any{"role": "assistant", "content": "ok"}, "finish_reason": "stop"}},
			"usage":   map[string]any{"prompt_tokens": 8, "completion_tokens": 2, "total_tokens": 10},
		})
	}))
	t.Cleanup(up.Close)
	mk := func() *gateway.Server {
		t.Helper()
		st, err := store.Open(url)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = st.DB.Close() })
		cfg := &config.Config{
			ModelList: []config.ModelEntry{
				{ModelName: "gpt-4o-mini", LiteLLMParams: map[string]any{"model": "openai/a", "api_base": up.URL, "api_key": "k"}},
				{ModelName: "gpt-4o-mini", LiteLLMParams: map[string]any{"model": "openai/b", "api_base": up.URL, "api_key": "k", "latency_ms": 5}},
			},
			RouterSettings:  config.RouterSettings{RoutingStrategy: "latency-based-routing", NumRetries: 3, Timeout: 5},
			GeneralSettings: config.GeneralSettings{MasterKey: "sk-master", DatabaseURL: url, RedisURL: redisURL},
		}
		s := gateway.New(cfg, st)
		if s.Live == nil {
			t.Fatal("redis client was not opened")
		}
		if asInt(prefs.MergedRouter(s)["allowed_fails"]) != 3 {
			t.Fatalf("shipped allowed_fails default %v", prefs.MergedRouter(s)["allowed_fails"])
		}
		t.Cleanup(func() { _ = s.Live.Close() })
		return s
	}
	a := mk()
	b := mk()
	failedID := router.DeploymentID(a.Cfg.ModelList[0])
	okID := router.DeploymentID(a.Cfg.ModelList[1])
	gen := doJSON(t, a.Handler(), "POST", "/key/generate", "sk-master", map[string]any{"key_type": "llm_api", "models": []any{"gpt-4o-mini"}})
	if gen.Code != 200 {
		t.Fatalf("key %d %s", gen.Code, gen.Body.String())
	}
	plain := str(decodeBody(t, gen.Body.Bytes())["key"])
	rec := doJSON(t, a.Handler(), "POST", "/v1/chat/completions", plain, map[string]any{
		"model": "gpt-4o-mini", "messages": []any{map[string]any{"role": "user", "content": "hi"}},
	})
	if rec.Code != 200 {
		t.Fatalf("chat %d %s", rec.Code, rec.Body.String())
	}
	if !b.RouteState().Cooldown[failedID] {
		t.Fatal("second process did not see cooldown from the request path")
	}
	picked := router.Pick(b.Cfg.ModelList, "gpt-4o-mini", "latency-based-routing", b.RouteState())
	if picked == nil || router.DeploymentID(*picked) == failedID {
		t.Fatalf("cooled deployment still picked %#v", picked)
	}
	if b.RouteState().Usage[okID] != 10 {
		t.Fatalf("second process usage %v", b.RouteState().Usage)
	}
	if _, ok := b.RouteState().Latency[okID]; !ok {
		t.Fatal("second process did not see latency from the request path")
	}
	hash := store.HashKey(plain)
	if b.Live.HotSpend(hash) <= 0 {
		t.Fatalf("hot spend %v", b.Live.HotSpend(hash))
	}
	b.FlushSpend()
	logs, err := b.Store.ListSpendLogs()
	if err != nil {
		t.Fatal(err)
	}
	seen := false
	for _, row := range logs {
		if row["model"] == "gpt-4o-mini" && row["api_key"] == hash {
			seen = true
		}
	}
	if !seen {
		t.Fatalf("flushed log missing %#v", logs)
	}
}
