package gateway

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
)

// catalogRoute is one method and path from the shipped route inventory.
type catalogRoute struct {
	Method string `json:"method"`
	Path   string `json:"path"`
}

// TestCatalogReads calls every catalog GET through the shipped gateway handler.
// A removed column must stay 404. A mounted read must return JSON, either 200 without an error object or the handler's 400 contract.
func TestCatalogReads(t *testing.T) {
	srv, base := bootGateway(t)
	defer srv.Close()
	key := loginAdmin(t, base)
	for _, rt := range catalogGets(t) {
		rt := rt
		t.Run(rt.Method+" "+rt.Path, func(t *testing.T) {
			path := fillCatalogPath(rt.Path)
			status, body := authed(t, base, key, http.MethodGet, path, nil)
			if IsRemovedColumn(rt.Path) || IsRemovedColumn(path) {
				if status != http.StatusNotFound {
					t.Fatalf("removed status %d %s", status, trim(body))
				}
				return
			}
			switch status {
			case http.StatusOK:
				if !json.Valid(body) {
					t.Fatalf("body is not json: %s", trim(body))
				}
				var probe map[string]any
				if json.Unmarshal(body, &probe) == nil {
					if _, ok := probe["error"].(map[string]any); ok {
						t.Fatalf("error contract %s", trim(body))
					}
				}
			case http.StatusBadRequest:
				var probe map[string]any
				if json.Unmarshal(body, &probe) != nil {
					t.Fatalf("400 is not json: %s", trim(body))
				}
				if _, ok := probe["error"].(map[string]any); !ok {
					t.Fatalf("400 missing error contract: %s", trim(body))
				}
			case http.StatusNotFound:
				var probe map[string]any
				if json.Unmarshal(body, &probe) != nil {
					t.Fatalf("404 is not json: %s", trim(body))
				}
				errObj, _ := probe["error"].(map[string]any)
				if errObj["type"] != "not_found" {
					t.Fatalf("404 contract %s", trim(body))
				}
			default:
				t.Fatalf("status %d %s", status, trim(body))
			}
		})
	}
}

func catalogGets(t *testing.T) []catalogRoute {
	t.Helper()
	var raw []byte
	var err error
	for _, rel := range []string{"docs/catalog.json", "../../docs/catalog.json"} {
		raw, err = os.ReadFile(rel)
		if err == nil {
			break
		}
	}
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Routes []catalogRoute `json:"http_routes"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	out := make([]catalogRoute, 0, len(doc.Routes))
	for _, rt := range doc.Routes {
		if rt.Method != http.MethodGet || rt.Path == "" {
			continue
		}
		if seen[rt.Path] {
			continue
		}
		seen[rt.Path] = true
		out = append(out, rt)
	}
	if len(out) == 0 {
		t.Fatal("catalog has no GET routes")
	}
	return out
}

func fillCatalogPath(p string) string {
	repl := []struct{ a, b string }{
		{"{model:path}", "gpt-4o-mini"}, {"{model}", "gpt-4o-mini"},
		{"{key:path}", "sk-abc"}, {"{key}", "sk-abc"},
		{"{id}", "id1"}, {"{file_id}", "file1"}, {"{batch_id}", "batch1"},
		{"{agent_id}", "agent1"}, {"{guardrail_id}", "g1"}, {"{policy_id}", "p1"},
		{"{server_id}", "s1"}, {"{vector_store_id}", "vs1"}, {"{run_id}", "run1"},
		{"{thread_id}", "th1"}, {"{assistant_id}", "as1"}, {"{container_id}", "c1"},
		{"{video_id}", "v1"}, {"{response_id}", "r1"}, {"{eval_id}", "e1"},
		{"{skill_id}", "sk1"}, {"{tool_name}", "tool1"}, {"{mcp_server_name}", "mcp1"},
		{"{plugin_name}", "plug1"}, {"{access_group}", "ag1"}, {"{access_group_id}", "ag1"},
		{"{organization_id}", "o1"}, {"{team_id:path}", "t1"}, {"{team_id}", "t1"},
		{"{user_id}", "u1"}, {"{project_id}", "pr1"}, {"{budget_id}", "b1"},
		{"{credential_name:path}", "cred1"}, {"{credential_name}", "cred1"},
		{"{prompt_id}", "pmt1"}, {"{search_tool_id}", "st1"}, {"{job_id}", "j1"},
		{"{name}", "n1"}, {"{endpoint}", "chat"}, {"{provider}", "openai"},
		{"{model_id}", "gpt-4o-mini"}, {"{model_name:path}", "gpt-4o-mini"},
		{"{file_id:path}", "file1"}, {"{fine_tuning_job_id:path}", "ft1"},
		{"{batch_id:path}", "batch1"}, {"{request_id}", "req1"},
		{"{interaction_id}", "ix1"}, {"{character_id}", "ch1"},
		{"{category_name}", "cat1"}, {"{key_id}", "kid1"}, {"{facet}", "info"},
	}
	for _, r := range repl {
		p = strings.ReplaceAll(p, r.a, r.b)
	}
	for strings.Contains(p, "{") {
		i := strings.Index(p, "{")
		j := strings.Index(p, "}")
		if j < i {
			break
		}
		p = p[:i] + "x" + p[j+1:]
	}
	if p == "" {
		return "/"
	}
	return p
}
