package suite

import (
	"github.com/sunqirui1987/xhub/internal/gateway"
	"strings"
	"testing"

	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/store"
)

func TestModelCRUDAlignment(t *testing.T) {
	s, master := testEnv(t)
	h := s.Handler()
	created := doJSON(t, h, "POST", "/model/new", master, map[string]any{
		"model_name":     "alias-1",
		"litellm_params": map[string]any{"model": "openai/gpt-4o-mini", "api_key": "k", "rpm": 10},
		"model_info":     map[string]any{"mode": "chat"},
	})
	if created.Code != 200 {
		t.Fatal(created.Body.String())
	}
	cm := decodeBody(t, created.Body.Bytes())
	mustKeys(t, cm, "model_name", "litellm_params", "model_info", "blocked")
	params0, _ := cm["litellm_params"].(map[string]any)
	if params0["api_key"] == "k" {
		t.Fatal("model/new leaked api_key")
	}
	info, _ := cm["model_info"].(map[string]any)
	id := str(info["id"])
	if id == "" {
		t.Fatal("model_info.id missing")
	}

	upd := doJSON(t, h, "POST", "/model/update", master, map[string]any{
		"model_info":     map[string]any{"id": id},
		"litellm_params": map[string]any{"rpm": 99},
	})
	if upd.Code != 200 {
		t.Fatal(upd.Body.String())
	}
	um := decodeBody(t, upd.Body.Bytes())
	params, _ := um["litellm_params"].(map[string]any)
	if params["rpm"] != float64(99) {
		t.Fatalf("rpm %v", params)
	}

	blk := doJSON(t, h, "POST", "/model/block", master, map[string]any{"id": id})
	if decodeBody(t, blk.Body.Bytes())["blocked"] != true {
		t.Fatalf("block %s", blk.Body.String())
	}
	un := doJSON(t, h, "POST", "/model/unblock", master, map[string]any{"id": id})
	if decodeBody(t, un.Body.Bytes())["blocked"] != false {
		t.Fatalf("unblock %s", un.Body.String())
	}

	grp := doJSON(t, h, "GET", "/model_group/info", master, nil)
	if grp.Code != 200 {
		t.Fatal(grp.Body.String())
	}
	gm := decodeBody(t, grp.Body.Bytes())
	if _, ok := gm["data"]; !ok {
		t.Fatalf("model_group %v", gm)
	}

	del := doJSON(t, h, "POST", "/model/delete", master, map[string]any{"id": id})
	if del.Code != 200 {
		t.Fatal(del.Body.String())
	}
	if decodeBody(t, del.Body.Bytes())["deleted"] != true {
		t.Fatalf("delete %s", del.Body.String())
	}
	lst := doJSON(t, h, "GET", "/model/info", master, nil)
	if strings.Contains(lst.Body.String(), "alias-1") {
		t.Fatalf("model still listed %s", lst.Body.String())
	}
}

func TestConfigModelCannotDeleteAndAddedModelReloads(t *testing.T) {
	db := testDatabaseURL(t)
	yamlModel := config.ModelEntry{
		ModelName:     "gpt-4o-mini",
		LiteLLMParams: map[string]any{"model": "openai/gpt-4o-mini", "api_key": "sk-file"},
		ModelInfo:     map[string]any{"id": "cfg-gpt"},
	}
	open := func() *gateway.Server {
		cfg := &config.Config{
			ModelList:       []config.ModelEntry{yamlModel},
			RouterSettings:  config.RouterSettings{RoutingStrategy: "simple-shuffle", NumRetries: 1, Timeout: 5},
			GeneralSettings: config.GeneralSettings{MasterKey: "sk-master", DatabaseURL: db, StoreModelInDB: true},
		}
		st, err := store.Open(cfg.GeneralSettings.DatabaseURL)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = st.DB.Close() })
		return gateway.New(cfg, st)
	}
	s := open()
	h := s.Handler()
	denied := doJSON(t, h, "POST", "/model/delete", "sk-master", map[string]any{"id": "cfg-gpt"})
	if denied.Code != 400 {
		t.Fatalf("config delete %d %s", denied.Code, denied.Body.String())
	}
	created := doJSON(t, h, "POST", "/model/new", "sk-master", map[string]any{
		"model_name":     "added-model",
		"litellm_params": map[string]any{"model": "openai/gpt-4o", "api_key": "sk-added"},
	})
	if created.Code != 200 {
		t.Fatal(created.Body.String())
	}
	info, _ := decodeBody(t, created.Body.Bytes())["model_info"].(map[string]any)
	if info["db_model"] != true {
		t.Fatalf("new model db_model %v", info["db_model"])
	}
	_ = s.Store.DB.Close()

	s2 := open()
	listed := doJSON(t, s2.Handler(), "GET", "/model/info", "sk-master", nil)
	if !strings.Contains(listed.Body.String(), "added-model") {
		t.Fatalf("added model missing after reload %s", listed.Body.String())
	}
	if !strings.Contains(listed.Body.String(), `"db_model":false`) && !strings.Contains(listed.Body.String(), `"db_model": false`) {
		t.Fatalf("config model should stay non-db %s", listed.Body.String())
	}
}

func TestSpendActivityAndCalculate(t *testing.T) {
	s, master := testEnv(t)
	h := s.Handler()
	sk := mintLLM(t, s, master)
	chat := doJSON(t, h, "POST", "/v1/chat/completions", sk, map[string]any{
		"model": "gpt-4o-mini", "messages": []any{map[string]any{"role": "user", "content": "hi"}},
	})
	if chat.Code != 200 {
		t.Fatal(chat.Body.String())
	}

	act := doJSON(t, h, "GET", "/global/activity", master, nil)
	if act.Code != 200 {
		t.Fatal(act.Body.String())
	}
	am := decodeBody(t, act.Body.Bytes())
	mustKeys(t, am, "daily_data", "sum_api_requests", "sum_total_tokens")
	if asInt(am["sum_api_requests"]) < 1 {
		t.Fatalf("activity %v", am)
	}

	logs := doJSON(t, h, "GET", "/global/spend/logs", master, nil)
	if logs.Code != 200 {
		t.Fatal(logs.Body.String())
	}
	keys := doJSON(t, h, "GET", "/global/spend/keys", master, nil)
	if keys.Code != 200 {
		t.Fatal(keys.Body.String())
	}
	models := doJSON(t, h, "GET", "/global/spend/models", master, nil)
	if models.Code != 200 {
		t.Fatal(models.Body.String())
	}

	calc := doJSON(t, h, "POST", "/spend/calculate", master, map[string]any{
		"model": "gpt-4o-mini",
		"completion_response": map[string]any{
			"model": "gpt-4o-mini",
			"usage": map[string]any{"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15},
		},
	})
	if calc.Code != 200 {
		t.Fatal(calc.Body.String())
	}
	cm := decodeBody(t, calc.Body.Bytes())
	if _, ok := cm["cost"]; !ok {
		t.Fatalf("calculate %v", cm)
	}
	if cm["cost"].(float64) <= 0 {
		t.Fatalf("expected non-zero cost %v", cm)
	}

	cache := doJSON(t, h, "GET", "/global/activity/cache_hits", master, nil)
	if cache.Code != 200 {
		t.Fatal(cache.Body.String())
	}
	ch := decodeBody(t, cache.Body.Bytes())
	mustKeys(t, ch, "groups", "totals", "filter_options")

	tc := doJSON(t, h, "POST", "/health/test_connection", master, map[string]any{
		"mode": "chat", "litellm_params": map[string]any{"model": "gpt-4o-mini"},
	})
	if tc.Code != 200 {
		t.Fatal(tc.Body.String())
	}
	tm := decodeBody(t, tc.Body.Bytes())
	if tm["status"] != "success" {
		t.Fatalf("test_connection %v", tm)
	}
}
