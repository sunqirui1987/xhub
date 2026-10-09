package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/iam"
	"github.com/sunqirui1987/xhub/internal/store"
)

// liveChains are the primary reads the sidebar still uses.
// Each name is the product chain. The path is the shipped HTTP entry.
var liveChains = []struct {
	name string
	path string
}{
	{"api-keys", "/key/list?page=1&size=10&return_full_object=true&include_team_keys=true&include_created_by_keys=true"},
	{"playground-models", "/v1/models"},
	{"models-and-endpoints", "/v2/model/info"},
	{"guardrails", "/guardrails/list"},
	{"usage", "/global/activity"},
	{"logs", "/spend/logs/v2?page=1&page_size=10"},
	{"logs-ui", "/spend/logs/ui?page=1&page_size=10"},
	{"guardrails-monitor", "/guardrails/usage/overview"},
	{"teams", "/v2/team/list?page=1&page_size=10"},
	{"projects", "/project/list"},
	{"users", "/user/list"},
	{"organizations", "/organization/list"},
	{"router-settings", "/router/settings"},
	{"admin-panel", "/config/list?config_type=general_settings"},
	{"ui-settings", "/get/ui_settings"},
	{"spend-tags", "/spend/tags"},
	{"budget-list", "/budget/list"},
	{"tag-list", "/tag/list"},
}

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}

func TestLiveChains(t *testing.T) {
	srv, base, db := bootGateway(t)
	defer srv.Close()
	var key string
	t.Run("login", func(t *testing.T) {
		key = loginAdmin(t, base, db)
	})
	if key == "" {
		t.Fatal("login did not return a session")
	}

	for _, chain := range liveChains {
		t.Run(chain.name, func(t *testing.T) {
			status, body := authed(t, base, key, http.MethodGet, chain.path, nil)
			if status != http.StatusOK {
				t.Fatalf("status %d body %s", status, trim(body))
			}
			if !json.Valid(body) {
				t.Fatalf("body is not json: %s", trim(body))
			}
			var probe map[string]any
			if json.Unmarshal(body, &probe) == nil {
				if errObj, ok := probe["error"].(map[string]any); ok {
					t.Fatalf("error contract %v", errObj["type"])
				}
			}
		})
	}

	t.Run("models-and-endpoints-has-rows", func(t *testing.T) {
		// The page reads this route and nothing else, and its failure mode is an
		// empty table rather than an error: without a handler the request fell
		// through to the catalog's generic store and answered 200 with an empty
		// list, so the status-only assertion above stayed green while the page
		// showed nothing. Assert the envelope the page reads, and that it is not
		// empty.
		status, body := authed(t, base, key, http.MethodGet, "/v2/model/info?page=1&size=50", nil)
		if status != http.StatusOK {
			t.Fatalf("status %d body %s", status, trim(body))
		}
		var page struct {
			Data       []map[string]any `json:"data"`
			TotalCount int              `json:"total_count"`
			TotalPages int              `json:"total_pages"`
			Page       int              `json:"current_page"`
			Size       int              `json:"size"`
		}
		if err := json.Unmarshal(body, &page); err != nil {
			t.Fatalf("decode: %s", trim(body))
		}
		if len(page.Data) == 0 || page.TotalCount == 0 {
			t.Fatalf("the deployments page would render empty: %s", trim(body))
		}
		if page.TotalPages < 1 || page.Page != 1 || page.Size != 50 {
			t.Fatalf("pagination envelope is wrong: %s", trim(body))
		}
		// Each row has to carry the fields the table renders, including the two
		// the page uses to decide whether a row is editable.
		for _, row := range page.Data {
			if row["model_name"] == nil || row["litellm_params"] == nil || row["model_info"] == nil {
				t.Fatalf("row is missing a rendered field: %v", row)
			}
			if _, ok := row["db_model"].(bool); !ok {
				t.Fatalf("row has no db_model flag: %v", row)
			}
		}
		// A provider shell is a credential, not a deployment, so it must not be
		// offered as a model the page can configure.
		for _, row := range page.Data {
			if name, _ := row["model_name"].(string); name == "fennoai" || name == "qiniu" {
				t.Fatalf("provider shell %q leaked into the deployments list", name)
			}
		}
	})

	t.Run("logs-page-has-rows", func(t *testing.T) {
		// The console's Logs page reads /spend/logs/ui, and only /spend/logs/v2
		// was registered. The page's request fell through to the catalog's
		// generic store and answered an empty list, so a deployment with traffic
		// showed no rows. A status check cannot see that, so this asserts the
		// envelope the page reads and that the two routes agree.
		status, uiBody := authed(t, base, key, http.MethodGet, "/spend/logs/ui?page=1&page_size=10", nil)
		if status != http.StatusOK {
			t.Fatalf("ui status %d %s", status, trim(uiBody))
		}
		var ui struct {
			Data  []map[string]any `json:"data"`
			Total int              `json:"total"`
		}
		if err := json.Unmarshal(uiBody, &ui); err != nil {
			t.Fatalf("decode ui: %s", trim(uiBody))
		}
		status, v2Body := authed(t, base, key, http.MethodGet, "/spend/logs/v2?page=1&page_size=10", nil)
		if status != http.StatusOK {
			t.Fatalf("v2 status %d %s", status, trim(v2Body))
		}
		var v2 struct {
			Data  []map[string]any `json:"data"`
			Total int              `json:"total"`
		}
		if err := json.Unmarshal(v2Body, &v2); err != nil {
			t.Fatalf("decode v2: %s", trim(v2Body))
		}
		// Both routes describe the same rows, so they must not disagree: a
		// mismatch means one of them is not reaching the usage tables.
		if ui.Total != v2.Total {
			t.Fatalf("the two log routes disagree: ui total %d, v2 total %d", ui.Total, v2.Total)
		}
		// And the page's own shape has to be usable, whatever the count is.
		if ui.Data == nil {
			t.Fatalf("the logs page would render nothing: %s", trim(uiBody))
		}
	})

	t.Run("team-info", func(t *testing.T) {
		// The fixture starts with no team at all, so the team the assertions
		// below read has to be created first.
		status, body := authed(t, base, key, http.MethodPost, "/organization/new",
			[]byte(`{"organization_alias":"Chain Org"}`))
		if status != http.StatusOK {
			t.Fatalf("create org %d %s", status, trim(body))
		}
		var org struct {
			OrganizationID string `json:"organization_id"`
		}
		if err := json.Unmarshal(body, &org); err != nil || org.OrganizationID == "" {
			t.Fatalf("organization id: %s", trim(body))
		}
		// A team must name its first team_admin, so the administrator created for
		// this test is looked up by the address the login used.
		admin, err := db.UserByEmail(context.Background(), "chain-admin@example.com")
		if err != nil {
			t.Fatalf("read admin: %v", err)
		}
		status, body = authed(t, base, key, http.MethodPost, "/team/new",
			[]byte(`{"team_alias":"Chain Team","organization_id":"`+org.OrganizationID+`","admin_user_id":"`+admin.ID+`"}`))
		if status != http.StatusOK {
			t.Fatalf("create team %d %s", status, trim(body))
		}

		status, body = authed(t, base, key, http.MethodGet, "/v2/team/list?page=1&page_size=10", nil)
		if status != http.StatusOK {
			t.Fatalf("team list %d %s", status, trim(body))
		}
		var page struct {
			Teams []struct {
				TeamID string `json:"team_id"`
			} `json:"teams"`
		}
		if err := json.Unmarshal(body, &page); err != nil || len(page.Teams) == 0 || page.Teams[0].TeamID == "" {
			t.Fatalf("team list has no team: %s", trim(body))
		}
		status, body = authed(t, base, key, http.MethodGet, "/team/info?team_id="+page.Teams[0].TeamID, nil)
		if status != http.StatusOK {
			t.Fatalf("team info %d %s", status, trim(body))
		}
		var info map[string]any
		if err := json.Unmarshal(body, &info); err != nil {
			t.Fatal(err)
		}
		if _, ok := info["team_info"]; !ok {
			t.Fatalf("team_info missing: %s", trim(body))
		}
		if _, ok := info["team_memberships"]; !ok {
			t.Fatalf("team_memberships missing: %s", trim(body))
		}
	})
}

func TestPlaygroundCompletionReachesUpstream(t *testing.T) {
	var calls int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" {
			t.Errorf("upstream request %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer sk-playground-fixture" {
			t.Errorf("upstream authorization %q", r.Header.Get("Authorization"))
		}
		body, _ := io.ReadAll(r.Body)
		if !bytes.Contains(body, []byte(`"model":"openai/gpt-6-astra"`)) || !bytes.Contains(body, []byte(`"content":"ping"`)) {
			t.Errorf("upstream body %s", trim(body))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-playground","object":"chat.completion","created":1,"model":"gpt-6-astra","choices":[{"index":0,"message":{"role":"assistant","content":"pong"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer upstream.Close()
	cfg, err := config.Load(configPath(t))
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(cfg.GeneralSettings.DatabaseURL)
	if err != nil {
		t.Fatal(err)
	}
	ensurePlaygroundModel(t, st, upstream.URL)
	srv, base, db := bootGateway(t)
	defer srv.Close()
	key := loginAdmin(t, base, db)
	client := &http.Client{Timeout: 5 * time.Second}
	payload := []byte(`{"model":"gpt-6-astra","messages":[{"role":"user","content":"ping"}]}`)
	req, err := http.NewRequest(http.MethodPost, base+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || !bytes.Contains(body, []byte(`"content":"pong"`)) {
		t.Fatalf("status %d body %s", resp.StatusCode, trim(body))
	}
	if calls != 1 {
		t.Fatalf("upstream calls %d", calls)
	}
}

func ensurePlaygroundModel(t *testing.T, st *store.Store, upstreamURL string) {
	t.Helper()
	rows, err := st.ListProxyModels()
	if err != nil {
		t.Fatal(err)
	}
	var previous *store.ProxyModel
	for i := range rows {
		if rows[i].ID == "model_gpt6_astra" {
			copy := rows[i]
			previous = &copy
			break
		}
	}
	t.Cleanup(func() {
		if previous != nil {
			_ = st.UpsertProxyModel(*previous)
		} else {
			_ = st.DeleteProxyModel("model_gpt6_astra")
		}
	})
	err = st.UpsertProxyModel(store.ProxyModel{
		ID:        "model_gpt6_astra",
		ModelName: "gpt-6-astra",
		Params: map[string]any{
			"model":               "openai/gpt-6-astra",
			"custom_llm_provider": "openai",
			"api_base":            upstreamURL + "/v1",
			"api_key":             "sk-playground-fixture",
		},
		Info: map[string]any{"id": "model_gpt6_astra", "db_model": true, "transport": "bypass_openai_chat", "endpoint_types": []string{"chat"}},
	})
	if err != nil {
		t.Fatal(err)
	}
}

// bootGateway starts the real handler over a private identity store. The caller
// signs in with loginAdmin, which seeds the administrator it logs in as.
func bootGateway(t *testing.T) (*httptest.Server, string, *iam.DB) {
	t.Helper()
	cfg, err := config.Load(configPath(t))
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(cfg.GeneralSettings.DatabaseURL)
	if err != nil {
		t.Fatal(err)
	}
	db := testIdentityStore(t)
	gw := New(cfg, st, db)
	srv := httptest.NewServer(gw.Handler())
	return srv, srv.URL, db
}

func configPath(t *testing.T) string {
	t.Helper()
	for _, rel := range []string{"configs/config.yaml", "../../configs/config.yaml"} {
		if _, err := os.Stat(rel); err == nil {
			abs, _ := filepath.Abs(rel)
			return abs
		}
	}
	t.Fatal("configs/config.yaml not found")
	return ""
}

// loginAdmin signs in as a platform administrator seeded through the configured
// administrator path, which is how a deployment gets its first account.
//
// It deliberately does not use the master key: that credential reaches the
// bootstrap and emergency routes only, and is not an account password.
func loginAdmin(t *testing.T, base string, db *iam.DB) string {
	t.Helper()
	const (
		email    = "chain-admin@example.com"
		password = "password123"
	)
	// A platform administrator is what makes the management routes reachable;
	// the master key is not an account and does not confer them.
	if _, err := db.EnsureAdmin(context.Background(), email, "Chain Admin", password); err != nil {
		t.Fatalf("seed admin: %v", err)
	}
	payload, _ := json.Marshal(map[string]string{"username": email, "password": password})
	resp, err := http.Post(base+"/v2/login", "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login %d %s", resp.StatusCode, trim(body))
	}
	var parsed struct {
		Key string `json:"key"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil || !strings.HasPrefix(parsed.Key, "sess-") {
		t.Fatalf("login key: %s", trim(body))
	}
	return parsed.Key
}

func authed(t *testing.T, base, key, method, path string, payload []byte) (int, []byte) {
	t.Helper()
	req, err := http.NewRequest(method, base+path, bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+key)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, body
}

func trim(b []byte) string {
	s := string(b)
	if len(s) > 240 {
		return s[:240]
	}
	return s
}
