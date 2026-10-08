package models

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sunqirui1987/xhub/internal/auth"
	"github.com/sunqirui1987/xhub/internal/authz"
	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/iam"
	"github.com/sunqirui1987/xhub/internal/store"
)

type modelTestHost struct {
	models []config.ModelEntry
}

func (h *modelTestHost) RequireManage(http.ResponseWriter, *http.Request) *auth.Principal {
	return &auth.Principal{Kind: authz.KindSession, Role: iam.RoleAdmin, UserID: "admin"}
}
func (h *modelTestHost) RecordStore() *store.Store { return nil }
func (h *modelTestHost) Identity() *iam.DB         { return nil }
func (h *modelTestHost) LockModels()               {}
func (h *modelTestHost) UnlockModels()             {}
func (h *modelTestHost) ModelTable() *[]config.ModelEntry {
	return &h.models
}
func (h *modelTestHost) Resolve(*http.Request) (*auth.Principal, error) {
	return &auth.Principal{Kind: authz.KindSession, Role: iam.RoleAdmin, UserID: "admin"}, nil
}
func (h *modelTestHost) AllowLLM(*auth.Principal) bool { return true }

func TestFailedModelUpdateDoesNotMutateStoredEntry(t *testing.T) {
	h := &modelTestHost{models: []config.ModelEntry{{
		ModelName:     "public-name",
		LiteLLMParams: map[string]any{"model": "upstream-name", "input_cost_per_token": 1.0},
		ModelInfo:     map[string]any{"id": "model-1", "db_model": true, "pricing_source": "manual"},
	}}}
	body := []byte(`{"model_name":"mutated-name","litellm_params":{"input_cost_per_token":-1},"model_info":{"base_model":"missing"}}`)
	r := httptest.NewRequest(http.MethodPost, "/model/update/model-1", bytes.NewReader(body))
	r.SetPathValue("model_id", "model-1")
	w := httptest.NewRecorder()
	Update(h, w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	got := h.models[0]
	if got.ModelName != "public-name" || got.LiteLLMParams["input_cost_per_token"] != 1.0 || got.ModelInfo["base_model"] != nil {
		t.Fatalf("failed update mutated live entry: %#v", got)
	}
}

func TestModelIDSearchFiltersBeforePagination(t *testing.T) {
	h := &modelTestHost{}
	for i := 0; i < 55; i++ {
		id := "ordinary"
		if i == 54 {
			id = "target-deployment-id"
		}
		h.models = append(h.models, config.ModelEntry{
			ModelName:     "model-" + string(rune('a'+i%26)),
			LiteLLMParams: map[string]any{"model": "upstream"},
			ModelInfo:     map[string]any{"id": id + string(rune('A'+i)), "db_model": true},
		})
	}
	targetID := h.models[54].ModelInfo["id"].(string)
	for _, query := range []string{"modelId=" + targetID, "search=target-deployment-id"} {
		r := httptest.NewRequest(http.MethodGet, "/v2/model/info?page=1&size=10&"+query, nil)
		w := httptest.NewRecorder()
		Info(h, w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("%s status=%d body=%s", query, w.Code, w.Body.String())
		}
		var out struct {
			Data       []map[string]any `json:"data"`
			TotalCount int              `json:"total_count"`
			TotalPages int              `json:"total_pages"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		if out.TotalCount != 1 || out.TotalPages != 1 || len(out.Data) != 1 || out.Data[0]["id"] != targetID {
			t.Fatalf("%s returned %#v", query, out)
		}
	}
}

func TestPublicDefaultsEnabledAndReportsDisabled(t *testing.T) {
	got := Public(config.ModelEntry{ModelName: "m", ModelInfo: map[string]any{"disabled": true}})
	if _, ok := got["disabled"]; ok {
		t.Fatalf("top-level disabled must be absent: %#v", got)
	}
	info := got["model_info"].(map[string]any)
	if info["disabled"] != true {
		t.Fatalf("model_info=%#v", info)
	}
	enabledInfo := Public(config.ModelEntry{ModelName: "enabled"})["model_info"].(map[string]any)
	if enabledInfo["disabled"] != false {
		t.Fatal("missing disabled flag must default to enabled")
	}
}

func TestStripModelOwnership(t *testing.T) {
	info := map[string]any{
		"team_id": "t", "teamId": "t2", "organization_id": "o",
		"organizationId": "o2", "org_id": "o3", "user_id": "u",
		"access_groups": []string{"legacy"}, "model_access_group": "legacy", "mode": "chat",
	}
	stripModelOwnership(info)
	if len(info) != 1 || info["mode"] != "chat" {
		t.Fatalf("ownership fields remained: %#v", info)
	}
}

func TestManagementInfoIgnoresTeamFilterAndShowsDisabled(t *testing.T) {
	h := &modelTestHost{models: []config.ModelEntry{{
		ModelName: "global", LiteLLMParams: map[string]any{"model": "upstream"},
		ModelInfo: map[string]any{"id": "deployment-1", "db_model": true, "disabled": true},
	}}}
	r := httptest.NewRequest(http.MethodGet, "/v2/model/info?team_id=unrelated", nil)
	w := httptest.NewRecorder()
	Info(h, w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var out struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Data) != 1 {
		t.Fatalf("rows=%#v", out.Data)
	}
	if _, ok := out.Data[0]["disabled"]; ok {
		t.Fatalf("top-level disabled must be absent: %#v", out.Data[0])
	}
	info, ok := out.Data[0]["model_info"].(map[string]any)
	if !ok || info["disabled"] != true {
		t.Fatalf("rows=%#v", out.Data)
	}
}
