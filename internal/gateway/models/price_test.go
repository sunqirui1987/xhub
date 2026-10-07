package models

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sunqirui1987/xhub/internal/auth"
	"github.com/sunqirui1987/xhub/internal/catalog"
	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/iam"
	"github.com/sunqirui1987/xhub/internal/store"
	"github.com/sunqirui1987/xhub/internal/testsupport"
)

// priceHost is the process surface the price handlers need: a real store, plus
// a switch for whether the caller may manage the catalog. The methods the price
// handlers never call are stubs.
type priceHost struct {
	store *store.Store
	allow bool
}

func (h *priceHost) RequireManage(w http.ResponseWriter, r *http.Request) *auth.Principal {
	if !h.allow {
		w.WriteHeader(http.StatusUnauthorized)
		return nil
	}
	return &auth.Principal{UserID: "admin"}
}

func (h *priceHost) RecordStore() *store.Store { return h.store }

func (h *priceHost) Identity() *iam.DB { return nil }

func (h *priceHost) LockModels() {}

func (h *priceHost) UnlockModels() {}

func (h *priceHost) ModelTable() *[]config.ModelEntry { return &[]config.ModelEntry{} }

func (h *priceHost) Resolve(r *http.Request) (*auth.Principal, error) { return nil, nil }

func (h *priceHost) AllowLLM(p *auth.Principal) bool { return true }

func openPriceStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(testsupport.Postgres(t, "price"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() {
		if st.Engine != nil {
			_ = st.Engine.Close()
		}
		// The price map is package state shared by every test in the process, so
		// a row this test wrote must not leak into the next one.
		catalog.RemoveModel("acme-chat-v1")
		if baseline, ok := catalog.BaselineModel("claude-4.1-opus"); ok {
			catalog.SetModel("claude-4.1-opus", baseline)
		}
	})
	return st
}

func call(h *priceHost, method, path string, body any) (int, map[string]any) {
	raw, _ := json.Marshal(body)
	r := httptest.NewRequest(method, path, bytes.NewReader(raw))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	switch {
	case path == "/price/model" && method == http.MethodPost:
		UpsertPriceModel(h, w, r)
	case path == "/price/model" && method == http.MethodDelete:
		DeletePriceModel(h, w, r)
	case path == "/price/model/reset":
		ResetPriceModel(h, w, r)
	case path == "/price/provider" && method == http.MethodPost:
		UpsertPriceProvider(h, w, r)
	case path == "/price/provider" && method == http.MethodDelete:
		DeletePriceProvider(h, w, r)
	case path == "/price/catalog":
		PriceList(h, w, r)
	default:
		panic("unhandled path " + method + " " + path)
	}
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out
}

// priceRow finds one model in the list response.
func priceRow(t *testing.T, body map[string]any, id string) map[string]any {
	t.Helper()
	rows, _ := body["models"].([]any)
	for _, raw := range rows {
		row, _ := raw.(map[string]any)
		if row["id"] == id {
			return row
		}
	}
	return nil
}

func TestPriceCatalogServesTheEmbeddedBaseline(t *testing.T) {
	h := &priceHost{allow: true}
	status, body := call(h, http.MethodGet, "/price/catalog", nil)
	if status != http.StatusOK {
		t.Fatalf("status %d", status)
	}
	row := priceRow(t, body, "claude-4.1-opus")
	if row == nil {
		t.Fatal("the embedded catalog did not reach the console")
	}
	if row["baseline"] != true || row["overridden"] != false {
		t.Fatalf("flags %#v", row)
	}
	if row["input_cost_per_token"] == nil || row["output_cost_per_token"] == nil {
		t.Fatalf("rates missing %#v", row)
	}
}

func TestPriceModelWriteIsStoredAndSurvivesAReload(t *testing.T) {
	h := &priceHost{store: openPriceStore(t), allow: true}

	status, _ := call(h, http.MethodPost, "/price/model", map[string]any{
		"id": "acme-chat-v1", "litellm_provider": "custom", "display_name": "Acme Chat",
		"mode": "chat", "input_cost_per_token": 0.000002, "output_cost_per_token": 0.000009,
	})
	if status != http.StatusOK {
		t.Fatalf("status %d", status)
	}
	// A hand-added row must be billable immediately, not only after a restart.
	if in, out, ok := catalog.TokenRates("acme-chat-v1"); !ok || in != 0.000002 || out != 0.000009 {
		t.Fatalf("rates after write in=%v out=%v ok=%v", in, out, ok)
	}

	// A fresh process replays the stored rows over the embedded baseline.
	catalog.RemoveModel("acme-chat-v1")
	if _, _, ok := catalog.TokenRates("acme-chat-v1"); ok {
		t.Fatal("the row was not removed")
	}
	LoadPriceOverrides(&priceHost{store: h.store, allow: true})
	if in, out, ok := catalog.TokenRates("acme-chat-v1"); !ok || in != 0.000002 || out != 0.000009 {
		t.Fatalf("rates after reload in=%v out=%v ok=%v", in, out, ok)
	}

	status, body := call(h, http.MethodGet, "/price/catalog", nil)
	if status != http.StatusOK {
		t.Fatalf("status %d", status)
	}
	row := priceRow(t, body, "acme-chat-v1")
	if row == nil || row["baseline"] != false || row["overridden"] != true {
		t.Fatalf("row flags %#v", row)
	}
}

func TestPriceModelEditOverridesTheBaselineAndResetRestoresIt(t *testing.T) {
	h := &priceHost{store: openPriceStore(t), allow: true}
	_, before := call(h, http.MethodGet, "/price/catalog", nil)
	original := priceRow(t, before, "claude-4.1-opus")["input_cost_per_token"]

	status, _ := call(h, http.MethodPost, "/price/model", map[string]any{
		"id": "claude-4.1-opus", "litellm_provider": "anthropic",
		"input_cost_per_token": 0.00002, "output_cost_per_token": nil,
	})
	if status != http.StatusOK {
		t.Fatalf("status %d", status)
	}
	in, out, ok := catalog.TokenRates("claude-4.1-opus")
	if !ok || in != 0.00002 {
		t.Fatalf("override did not take effect in=%v ok=%v", in, ok)
	}
	// A cleared side stops being billable. It must not become a free side, and
	// the baseline's other rates must survive the edit.
	if out != 0 {
		t.Fatalf("cleared output side is %v", out)
	}
	if _, ok := catalog.BaselineModel("claude-4.1-opus"); !ok {
		t.Fatal("editing a baseline row must not remove it from the baseline")
	}

	status, body := call(h, http.MethodPost, "/price/model/reset", map[string]any{"id": "claude-4.1-opus"})
	if status != http.StatusOK || body["restored"] != true {
		t.Fatalf("reset status=%d body=%#v", status, body)
	}
	if in, _, _ := catalog.TokenRates("claude-4.1-opus"); in != original {
		t.Fatalf("reset left input at %v, want %v", in, original)
	}
}

func TestPriceModelDeleteOfABaselineRowIsRemembered(t *testing.T) {
	h := &priceHost{store: openPriceStore(t), allow: true}

	status, _ := call(h, http.MethodDelete, "/price/model", map[string]any{"id": "kimi-k2"})
	if status != http.StatusOK {
		t.Fatalf("status %d", status)
	}
	if _, _, ok := catalog.TokenRates("kimi-k2"); ok {
		t.Fatal("the deleted row is still priced")
	}

	// The deletion survives a restart, and the console can still see the id so
	// the action is reversible.
	LoadPriceOverrides(&priceHost{store: h.store, allow: true})
	if _, _, ok := catalog.TokenRates("kimi-k2"); ok {
		t.Fatal("the deleted row came back after a reload")
	}
	status, body := call(h, http.MethodGet, "/price/catalog", nil)
	if status != http.StatusOK {
		t.Fatalf("status %d", status)
	}
	row := priceRow(t, body, "kimi-k2")
	if row == nil || row["removed"] != true {
		t.Fatalf("removed row %#v", row)
	}

	status, reset := call(h, http.MethodPost, "/price/model/reset", map[string]any{"id": "kimi-k2"})
	if status != http.StatusOK || reset["restored"] != true {
		t.Fatalf("reset status=%d body=%#v", status, reset)
	}
	if _, _, ok := catalog.TokenRates("kimi-k2"); !ok {
		t.Fatal("the restored row has no rates")
	}
}

func TestPriceModelRejectsAMissingIdOrProvider(t *testing.T) {
	h := &priceHost{store: openPriceStore(t), allow: true}
	if status, _ := call(h, http.MethodPost, "/price/model", map[string]any{"litellm_provider": "custom"}); status != http.StatusBadRequest {
		t.Fatalf("a model with no id was accepted: %d", status)
	}
	if status, _ := call(h, http.MethodPost, "/price/model", map[string]any{"id": "x-one"}); status != http.StatusBadRequest {
		t.Fatalf("a model with no provider was accepted: %d", status)
	}
	if status, _ := call(h, http.MethodDelete, "/price/model", map[string]any{}); status != http.StatusBadRequest {
		t.Fatalf("a delete with no id was accepted: %d", status)
	}
}

func TestPriceWritesNeedManage(t *testing.T) {
	h := &priceHost{store: openPriceStore(t), allow: false}
	cases := []struct {
		method, path string
		body         any
	}{
		{http.MethodGet, "/price/catalog", nil},
		{http.MethodPost, "/price/model", map[string]any{"id": "a", "litellm_provider": "custom"}},
		{http.MethodDelete, "/price/model", map[string]any{"id": "a"}},
		{http.MethodPost, "/price/provider", map[string]any{"litellm_provider": "a"}},
		{http.MethodDelete, "/price/provider", map[string]any{"litellm_provider": "a"}},
	}
	for _, tc := range cases {
		if status, _ := call(h, tc.method, tc.path, tc.body); status != http.StatusUnauthorized {
			t.Fatalf("%s %s answered %d without manage", tc.method, tc.path, status)
		}
	}
}

func TestPriceProviderAddEditAndDeleteReachesTheDropdown(t *testing.T) {
	h := &priceHost{store: openPriceStore(t), allow: true}

	status, _ := call(h, http.MethodPost, "/price/provider", map[string]any{
		"litellm_provider": "acme", "provider_display_name": "Acme",
		"default_api_base": "https://api.acme.test",
	})
	if status != http.StatusOK {
		t.Fatalf("status %d", status)
	}
	found := func(body map[string]any, slug string) map[string]any {
		rows, _ := body["providers"].([]any)
		for _, raw := range rows {
			row, _ := raw.(map[string]any)
			if row["litellm_provider"] == slug {
				return row
			}
		}
		return nil
	}
	_, body := call(h, http.MethodGet, "/price/catalog", nil)
	row := found(body, "acme")
	if row == nil || row["baseline"] != false || row["overridden"] != true {
		t.Fatalf("provider row %#v", row)
	}
	// A hand-added supplier must be selectable in the add-model form, which
	// reads the public provider list rather than the console one.
	if !catalog.KnownProvider("acme") {
		t.Fatal("the added supplier is not a known provider")
	}

	// The addition survives a restart.
	catalog.RemoveProvider("acme")
	LoadPriceOverrides(&priceHost{store: h.store, allow: true})
	_, body = call(h, http.MethodGet, "/price/catalog", nil)
	if found(body, "acme") == nil {
		t.Fatal("the added supplier did not come back after a reload")
	}

	if status, _ := call(h, http.MethodDelete, "/price/provider", map[string]any{"litellm_provider": "acme"}); status != http.StatusOK {
		t.Fatalf("delete status %d", status)
	}
	_, body = call(h, http.MethodGet, "/price/catalog", nil)
	if found(body, "acme") != nil {
		t.Fatal("the deleted supplier is still listed")
	}
	LoadPriceOverrides(&priceHost{store: h.store, allow: true})
	_, body = call(h, http.MethodGet, "/price/catalog", nil)
	if found(body, "acme") != nil {
		t.Fatal("the deleted supplier came back after a reload")
	}
}

func TestPriceProviderDeleteOfABaselineSupplierIsRemembered(t *testing.T) {
	h := &priceHost{store: openPriceStore(t), allow: true}
	if status, _ := call(h, http.MethodDelete, "/price/provider", map[string]any{"litellm_provider": "anthropic"}); status != http.StatusOK {
		t.Fatalf("status %d", status)
	}
	LoadPriceOverrides(&priceHost{store: h.store, allow: true})
	_, body := call(h, http.MethodGet, "/price/catalog", nil)
	rows, _ := body["providers"].([]any)
	for _, raw := range rows {
		row, _ := raw.(map[string]any)
		if row["litellm_provider"] == "anthropic" {
			t.Fatal("a deleted baseline supplier came back after a reload")
		}
	}
	// Put it back so the shared process state is unchanged for later tests.
	catalog.SetProvider("anthropic", map[string]any{
		"provider": "Anthropic", "provider_display_name": "Anthropic", "litellm_provider": "anthropic",
	})
}
