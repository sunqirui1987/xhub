package gateway

import (
	"bytes"
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
	{"guardrails-monitor", "/guardrails/usage/overview"},
	{"teams", "/v2/team/list?page=1&page_size=10"},
	{"projects", "/project/list"},
	{"users", "/user/list"},
	{"organizations", "/organization/list"},
	{"access-groups", "/access_group/list"},
	{"router-settings", "/router/settings"},
	{"logging-and-alerts", "/get/config/callbacks"},
	{"cost-tracking", "/config/list?config_type=general_settings"},
	{"admin-panel", "/get/sso_settings"},
	{"ui-settings", "/get/ui_settings"},
	{"spend-tags", "/spend/tags"},
	{"tool-spend", "/v1/tool/spend"},
	{"budget-list", "/budget/list"},
	{"tag-list", "/tag/list"},
}

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}

func TestLiveChains(t *testing.T) {
	srv, base := bootGateway(t)
	defer srv.Close()
	var key string
	t.Run("login", func(t *testing.T) {
		key = loginAdmin(t, base)
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

	t.Run("team-info", func(t *testing.T) {
		status, body := authed(t, base, key, http.MethodGet, "/v2/team/list?page=1&page_size=10", nil)
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
	srv, base := bootGateway(t)
	defer srv.Close()
	key := loginAdmin(t, base)
	client := &http.Client{Timeout: 90 * time.Second}
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
	if resp.StatusCode == http.StatusUnauthorized {
		t.Fatalf("local auth rejection: %s", trim(body))
	}
	var parsed struct {
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	_ = json.Unmarshal(body, &parsed)
	if parsed.Error.Type == "authentication_error" {
		t.Fatalf("local authentication_error: %s", parsed.Error.Message)
	}
	if resp.StatusCode == http.StatusOK {
		return
	}
	if resp.StatusCode == http.StatusBadGateway && parsed.Error.Type == "upstream_error" && strings.Contains(parsed.Error.Message, "api.openai.com") {
		return
	}
	t.Fatalf("status %d type %s body %s", resp.StatusCode, parsed.Error.Type, trim(body))
}

func bootGateway(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	cfg, err := config.Load(configPath(t))
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(cfg.GeneralSettings.DatabaseURL)
	if err != nil {
		t.Fatal(err)
	}
	gw := New(cfg, st)
	srv := httptest.NewServer(gw.Handler())
	return srv, srv.URL
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

func loginAdmin(t *testing.T, base string) string {
	t.Helper()
	cfg, err := config.Load(configPath(t))
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(map[string]string{"username": "admin", "password": cfg.GeneralSettings.MasterKey})
	resp, err := http.Post(base+"/login", "application/json", bytes.NewReader(payload))
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
