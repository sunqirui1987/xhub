package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
)

func TestEveryCatalogAPI(t *testing.T) {
	s, master := testEnv(t)
	h := s.Handler()
	sk := mintLLM(t, s, master)
	raw, err := os.ReadFile("routes.json")
	if err != nil {
		t.Fatal(err)
	}
	var routes []catRoute
	if err := json.Unmarshal(raw, &routes); err != nil {
		t.Fatal(err)
	}
	famPaths := loadFamilyPaths(t)
	var bad []string
	checked := 0
	for _, rt := range routes {
		method := rt.M
		path := fillPath(rt.P)
		tok := master
		if isPublicPath(method, path) {
			tok = ""
		} else if isDataPlanePath(path) && !isMixedPath(path) {
			tok = sk
		} else if isMixedPath(path) {
			tok = sk
		}
		body := catalogBody(path)
		rec := doJSON(t, h, method, path, tok, body)
		checked++
		if removedColumnRoute(path) {
			if rec.Code != 404 {
				bad = append(bad, fmt.Sprintf("%s %s removed column want 404 got %d", method, path, rec.Code))
			}
			continue
		}
		if rec.Code == 404 && isTypedResource404(rec) {
			if rec.Result().Header.Get("x-litellm-call-id") == "" {
				bad = append(bad, fmt.Sprintf("%s %s typed 404 missing call-id", method, path))
			}
			continue
		}
		if rec.Code == 404 || rec.Code >= 500 {
			bad = append(bad, fmt.Sprintf("%s %s -> %d %s", method, path, rec.Code, truncate(rec.Body.String(), 180)))
			continue
		}
		if rec.Result().Header.Get("x-litellm-call-id") == "" {
			bad = append(bad, fmt.Sprintf("%s %s missing call-id", method, path))
		}
		if rec.Result().Header.Get("x-litellm-version") == "" {
			bad = append(bad, fmt.Sprintf("%s %s missing version", method, path))
		}
		if rec.Code == 200 {
			ct := rec.Result().Header.Get("Content-Type")
			if ct == "" {
				bad = append(bad, fmt.Sprintf("%s %s missing content-type", method, path))
			}
			if strings.Contains(ct, "json") && isGenericStub(rec.Body.String()) {
				bad = append(bad, fmt.Sprintf("%s %s generic stub %s", method, path, truncate(rec.Body.String(), 120)))
			}
			if (method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch) && strings.Contains(ct, "json") {
				var m map[string]any
				if json.Unmarshal(rec.Body.Bytes(), &m) == nil && shouldCheckFrozen(path, m) {
					keys := frozenRespKeys(path, famPaths)
					if len(keys) > 0 {
						miss := missingKeys(m, keys)
						if len(miss) > 0 {
							bad = append(bad, fmt.Sprintf("%s %s missing %v in %s", method, path, miss, truncate(rec.Body.String(), 160)))
						}
					}
				}
			}
		}
		if rec.Code == 401 && tok == sk && isDataPlanePath(path) {
			bad = append(bad, fmt.Sprintf("%s %s llm_api 401 %s", method, path, truncate(rec.Body.String(), 120)))
		}
	}
	if len(bad) != 0 {
		n := 12
		if len(bad) < n {
			n = len(bad)
		}
		t.Fatalf("misaligned=%d/%d examples=%v", len(bad), checked, bad[:n])
	}
	t.Logf("aligned catalog routes=%d", checked)
}

func loadFamilyPaths(t *testing.T) map[string][]string {
	t.Helper()
	raw, err := os.ReadFile("../../docs/_inventory/catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	var cat struct {
		Families []struct {
			ID        string   `json:"id"`
			HTTPPaths []string `json:"http_paths"`
		} `json:"families"`
	}
	if err := json.Unmarshal(raw, &cat); err != nil {
		t.Fatal(err)
	}
	out := map[string][]string{}
	for _, f := range cat.Families {
		out[f.ID] = f.HTTPPaths
	}
	return out
}

func familyOf(path string, famPaths map[string][]string) string {
	best, n := "", -1
	for id, paths := range famPaths {
		if !strings.HasPrefix(id, "data.") && !strings.HasPrefix(id, "mgmt.") {
			continue
		}
		for _, p := range paths {
			p = strings.TrimSuffix(p, "/")
			if p == "" {
				continue
			}
			if path == p || strings.HasPrefix(path, p+"/") {
				if len(p) > n {
					best, n = id, len(p)
				}
			}
		}
	}
	return best
}

func frozenRespKeys(path string, famPaths map[string][]string) []string {
	if strings.Contains(path, "/key/generate") || strings.Contains(path, "/key/service-account") || strings.Contains(path, "/regenerate") {
		return []string{"key", "key_name", "key_alias", "expires", "token_id", "user_id", "team_id", "models", "max_budget", "spend"}
	}
	if strings.Contains(path, "/spend/calculate") {
		return []string{"cost"}
	}
	if strings.HasSuffix(path, "/health/test_connection") {
		return []string{"status", "mode"}
	}
	if strings.Contains(path, "apply_guardrail") {
		return []string{"status", "action", "blocked"}
	}
	if strings.Contains(path, "/audio/speech") {
		return nil
	}
	if strings.HasPrefix(path, "/tools") || strings.HasPrefix(path, "/toolset") {
		return nil
	}
	if strings.Contains(path, "/count_tokens") {
		return []string{"input_tokens"}
	}
	if strings.Contains(path, "/key/health") || strings.Contains(path, "/key/aliases") || strings.Contains(path, "/reset_spend") {
		return nil
	}
	if strings.Contains(path, "/vector_stores") && strings.Contains(path, "/files") {
		return []string{"id", "object", "filename", "purpose", "status"}
	}
	fam := familyOf(path, famPaths)
	switch fam {
	case "data.chat":
		return []string{"id", "object", "choices", "usage", "model"}
	case "data.completions":
		return []string{"id", "object", "choices", "usage", "model"}
	case "data.messages":
		return []string{"id", "type", "role", "content", "model", "stop_reason", "usage"}
	case "data.responses":
		return []string{"id", "object", "status", "output", "usage", "model"}
	case "data.embeddings":
		return []string{"object", "data", "model"}
	case "data.images":
		return []string{"created", "data"}
	case "data.moderations":
		return []string{"id", "model", "results"}
	case "data.rerank":
		return []string{"id", "results", "meta"}
	case "data.files":
		return []string{"id", "object", "filename", "purpose", "status"}
	case "data.batches":
		return []string{"id", "object", "status", "request_counts"}
	case "data.assistants_threads":
		return []string{"id", "object", "created_at"}
	case "data.fine_tuning":
		return []string{"id", "object", "status", "model"}
	case "data.containers":
		return []string{"id", "object", "name", "status"}
	case "data.vector_stores":
		return []string{"id", "object", "name", "status", "file_counts"}
	case "data.videos":
		return []string{"id", "object", "status"}
	case "data.search_ocr_rag":
		return []string{"results", "usage"}
	case "data.evals":
		return []string{"id", "object", "name", "status"}
	case "data.workflows":
		return []string{"run_id", "status", "events", "messages"}
	case "data.agents":
		return []string{"agent_id", "agent_name", "created_at"}
	case "data.interactions":
		return []string{"id", "status", "output"}
	case "data.gemini_v1beta":
		return []string{"candidates", "usageMetadata"}
	case "data.a2a":
		return []string{"id", "status", "artifacts"}
	case "data.access_groups":
		return []string{"access_group_id", "models", "created_at"}
	case "mgmt.prompts":
		return []string{"prompt_id", "version", "created_at"}
	case "mgmt.guardrails":
		return []string{"guardrail_id", "guardrail_name", "created_at"}
	case "mgmt.policies":
		return []string{"policy_id", "policy_name", "version", "status"}
	case "mgmt.tags":
		return []string{"name", "spend", "created_at"}
	case "mgmt.credentials":
		return []string{"credential_name"}
	case "mgmt.customers":
		return []string{"user_id", "spend", "blocked"}
	case "mgmt.invitations":
		return []string{"id", "is_accepted", "expires"}
	case "mgmt.projects":
		return []string{"project_id", "project_alias", "blocked", "created_at"}
	case "mgmt.users":
		return []string{"user_id", "user_email", "user_role", "spend", "teams", "created_at"}
	case "mgmt.teams":
		return []string{"team_id", "team_alias", "members_with_roles"}
	case "mgmt.organizations":
		return []string{"organization_id", "organization_alias"}
	case "mgmt.budgets":
		return []string{"budget_id", "max_budget", "tpm_limit", "rpm_limit", "budget_duration", "budget_reset_at"}
	case "mgmt.keys":
		return []string{"key", "key_name", "spend"}
	case "mgmt.search_tools":
		return []string{"search_tool_id", "search_tool_name"}
	case "mgmt.vector_stores_admin":
		return []string{"vector_store_id", "vector_store_name", "created_at"}
	case "mgmt.mcp_servers":
		return []string{"server_id", "server_name"}
	case "data.mcp":
		return nil
	case "data.realtime":
		return []string{"id"}
	case "data.audio":
		if strings.Contains(path, "transcription") || strings.Contains(path, "translation") {
			return []string{"text"}
		}
		return nil
	default:
		return nil
	}
}

func shouldCheckFrozen(path string, m map[string]any) bool {
	if strings.Contains(path, "/delete") || strings.Contains(path, "/info") || strings.Contains(path, "/list") {
		return false
	}
	if strings.Contains(path, "/token") {
		return false
	}
	if _, ok := m["deleted"]; ok {
		return false
	}
	if _, ok := m["data"]; ok {
		if _, hasID := m["id"]; !hasID {
			if _, hasGID := m["guardrail_id"]; !hasGID {
				return false
			}
		}
	}
	return true
}

func missingKeys(m map[string]any, keys []string) []string {
	var miss []string
	for _, k := range keys {
		if _, ok := m[k]; !ok {
			miss = append(miss, k)
		}
	}
	return miss
}

func catalogBody(path string) map[string]any {
	b := map[string]any{
		"model": "gpt-4o-mini", "prompt": "hi", "input": "hi", "query": "q",
		"documents": []any{"a"}, "messages": []any{map[string]any{"role": "user", "content": "hi"}},
		"purpose": "batch", "name": "n1", "max_tokens": 16,
	}
	switch {
	case strings.Contains(path, "/user"):
		b["user_email"] = "all@x"
		b["user_role"] = "internal_user"
		b["password"] = "pw"
	case strings.Contains(path, "/team"):
		b["team_alias"] = "t"
	case strings.Contains(path, "/organization"):
		b["organization_alias"] = "o"
	case strings.Contains(path, "/project"):
		b["project_alias"] = "p"
	case strings.Contains(path, "/budget"):
		b["max_budget"] = 1.0
	case strings.Contains(path, "/guardrail"):
		b["guardrail_name"] = "g"
		b["litellm_params"] = map[string]any{"guardrail": "presidio"}
	case strings.Contains(path, "/prompt"):
		b["prompt_id"] = "pr1"
	case strings.Contains(path, "/tag"):
		b["name"] = "tag1"
	case strings.Contains(path, "/customer"), strings.Contains(path, "/end_user"):
		b["user_id"] = "end1"
	case strings.Contains(path, "/invitation"):
		b["user_email"] = "i@x"
	case strings.Contains(path, "/login"):
		b["username"] = "admin"
		b["password"] = "not-checked-here"
	}
	return b
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
