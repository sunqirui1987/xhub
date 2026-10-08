// 对已经在听的 e2e 网关逐条请求 catalog.json，不经过进程内 handler。
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/sunqirui1987/xhub/internal/catalog"
	"github.com/sunqirui1987/xhub/internal/gateway"
)

type httpRoute struct {
	Method string `json:"method"`
	Path   string `json:"path"`
}

func main() {
	base := strings.TrimRight(getenv("E2E_GATEWAY", "http://127.0.0.1:4000"), "/")
	master := getenv("E2E_MASTER_KEY", "sk-e2e-master")
	catPath := getenv("CATALOG_JSON", "docs/testdata/catalog.json")
	raw, err := os.ReadFile(catPath)
	if err != nil {
		fail(err)
	}
	var doc struct {
		Baseline struct {
			Unique int `json:"unique_http_routes"`
		} `json:"baseline"`
		Families []struct {
			ID        string   `json:"id"`
			HTTPPaths []string `json:"http_paths"`
		} `json:"families"`
		Routes []httpRoute `json:"http_routes"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		fail(err)
	}
	if doc.Baseline.Unique != len(doc.Routes) {
		fail(fmt.Errorf("unique_http_routes=%d http_routes=%d", doc.Baseline.Unique, len(doc.Routes)))
	}
	famPaths := map[string][]string{}
	for _, f := range doc.Families {
		famPaths[f.ID] = f.HTTPPaths
	}
	client := &http.Client{Timeout: 20 * time.Second}
	code, _, loginBody := call(client, base, http.MethodPost, "/v2/login", "", map[string]any{"username": "admin", "password": master})
	var login map[string]any
	_ = json.Unmarshal([]byte(loginBody), &login)
	admin, _ := login["key"].(string)
	if admin == "" {
		admin, _ = login["token"].(string)
	}
	if code != 200 || admin == "" {
		fail(fmt.Errorf("sweep login %d", code))
	}
	sk := mint(client, base, admin)
	var bad []string
	var lines []string
	checked := 0
	for _, rt := range doc.Routes {
		method := rt.Method
		path := fillPath(rt.Path)
		tok := admin
		if catalog.IsPublicPath(method, path) {
			tok = ""
		} else if catalog.IsDataPlanePath(path) {
			tok = sk
		}
		var body any
		if method != http.MethodGet && method != http.MethodHead {
			body = catalogBody(path)
		}
		code, hdr, resp := call(client, base, method, path, tok, body)
		checked++
		before := len(bad)
		record := func() {
			outcome := "pass"
			if len(bad) > before {
				outcome = "fail"
			}
			lines = append(lines, fmt.Sprintf("route %s %s %s %d", outcome, method, rt.Path, code))
		}
		if gateway.IsRemovedColumn(path) {
			if code != 404 {
				bad = append(bad, fmt.Sprintf("%s %s removed column want 404 got %d", method, path, code))
			}
			record()
			continue
		}
		if code == 404 && typed404(resp) {
			if hdr.Get("x-litellm-call-id") == "" {
				bad = append(bad, fmt.Sprintf("%s %s typed 404 missing call-id", method, path))
			}
			record()
			continue
		}
		if code == 0 || code == 404 || code >= 500 {
			bad = append(bad, fmt.Sprintf("%s %s -> %d %s", method, path, code, truncate(resp, 180)))
			record()
			continue
		}
		if hdr.Get("x-litellm-call-id") == "" {
			bad = append(bad, fmt.Sprintf("%s %s missing call-id", method, path))
		}
		if hdr.Get("x-litellm-version") == "" {
			bad = append(bad, fmt.Sprintf("%s %s missing version", method, path))
		}
		if code == 200 {
			ct := hdr.Get("Content-Type")
			if ct == "" {
				bad = append(bad, fmt.Sprintf("%s %s missing content-type", method, path))
			}
			if strings.Contains(ct, "json") && genericStub(resp) {
				bad = append(bad, fmt.Sprintf("%s %s generic stub %s", method, path, truncate(resp, 120)))
			}
			if (method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch) && strings.Contains(ct, "json") {
				var m map[string]any
				if json.Unmarshal([]byte(resp), &m) == nil && shouldCheckFrozen(path, m) {
					keys := frozenRespKeys(path, famPaths)
					if miss := missingKeys(m, keys); len(miss) > 0 {
						bad = append(bad, fmt.Sprintf("%s %s missing %v in %s", method, path, miss, truncate(resp, 160)))
					}
				}
			}
		}
		if code == 401 && tok == sk && catalog.IsDataPlanePath(path) {
			bad = append(bad, fmt.Sprintf("%s %s llm_api 401 %s", method, path, truncate(resp, 120)))
		}
		record()
	}
	if report := os.Getenv("E2E_ROUTE_LINES"); report != "" {
		_ = os.WriteFile(report, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
	}
	if len(bad) != 0 {
		n := 12
		if len(bad) < n {
			n = len(bad)
		}
		fmt.Printf("checked=%d misaligned=%d unique_http_routes=%d examples=%v\n", checked, len(bad), doc.Baseline.Unique, bad[:n])
		os.Exit(1)
	}
	fmt.Printf("checked=%d misaligned=0 unique_http_routes=%d\n", checked, doc.Baseline.Unique)
}

func mint(client *http.Client, base, master string) string {
	orgCode, _, orgBody := call(client, base, http.MethodPost, "/organization/new", master, map[string]any{"organization_alias": "e2e-sweep-org"})
	var org map[string]any
	_ = json.Unmarshal([]byte(orgBody), &org)
	if orgCode != 200 {
		fail(fmt.Errorf("sweep organization %d", orgCode))
	}
	// Master has no user identity, so create a first team administrator explicitly.
	adminCode, _, _ := call(client, base, http.MethodPost, "/user/new", master, map[string]any{"user_id": "e2e-sweep-admin", "user_email": "e2e-sweep-admin@example.com", "user_role": "user"})
	if adminCode != 200 {
		fail(fmt.Errorf("sweep administrator %d", adminCode))
	}
	teamCode, _, teamBody := call(client, base, http.MethodPost, "/team/new", master, map[string]any{"organization_id": org["organization_id"], "team_alias": "e2e-sweep-team", "admin_user_id": "e2e-sweep-admin"})
	var team map[string]any
	_ = json.Unmarshal([]byte(teamBody), &team)
	if teamCode != 200 {
		fail(fmt.Errorf("sweep team %d %s", teamCode, truncate(teamBody, 200)))
	}
	ownerCode, _, ownerBody := call(client, base, http.MethodPost, "/user/new", master, map[string]any{
		"user_id": "e2e-sweep-owner", "user_email": "e2e-sweep@example.com",
		"password": "e2e-sweep-password", "user_role": "user", "team_id": team["team_id"], "team_role": "user",
	})
	if ownerCode != 200 {
		fail(fmt.Errorf("create sweep owner %d %s", ownerCode, truncate(ownerBody, 200)))
	}
	code, _, body := call(client, base, http.MethodPost, "/key/generate", master, map[string]any{"key_type": "llm_api", "user_id": "e2e-sweep-owner", "team_id": team["team_id"]})
	var g map[string]any
	_ = json.Unmarshal([]byte(body), &g)
	sk, _ := g["key"].(string)
	if code != 200 || !strings.HasPrefix(sk, "sk-") {
		fail(fmt.Errorf("mint key %d %s", code, truncate(body, 200)))
	}
	return sk
}

func call(client *http.Client, base, method, path, token string, body any) (int, http.Header, string) {
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, base+path, rdr)
	if err != nil {
		return 0, http.Header{}, err.Error()
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := client.Do(req)
	if err != nil {
		return 0, http.Header{}, err.Error()
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	return res.StatusCode, res.Header, string(b)
}

func typed404(raw string) bool {
	var m map[string]any
	if json.Unmarshal([]byte(raw), &m) != nil {
		return false
	}
	errObj, _ := m["error"].(map[string]any)
	if errObj == nil || errObj["message"] == nil {
		return false
	}
	return errObj["type"] != nil || errObj["code"] != nil
}

func genericStub(raw string) bool {
	var m map[string]any
	if json.Unmarshal([]byte(raw), &m) != nil {
		return false
	}
	_, hasStatus := m["status"]
	_, hasID := m["id"]
	_, hasData := m["data"]
	if hasStatus && hasID && len(m) <= 3 && !hasData {
		st, _ := m["status"].(string)
		return st == "ok"
	}
	if hasStatus && hasData && len(m) <= 3 {
		st, _ := m["status"].(string)
		data, _ := m["data"].([]any)
		return st == "ok" && len(data) == 0
	}
	return false
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
	if strings.Contains(path, "/audio/speech") || strings.HasPrefix(path, "/tools") || strings.HasPrefix(path, "/toolset") {
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
	case "data.chat", "data.completions":
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
	case "data.files", "data.vector_stores":
		if fam == "data.files" {
			return []string{"id", "object", "filename", "purpose", "status"}
		}
		return []string{"id", "object", "name", "status", "file_counts"}
	case "data.batches":
		return []string{"id", "object", "status", "request_counts"}
	case "data.assistants_threads":
		return []string{"id", "object", "created_at"}
	case "data.fine_tuning":
		return []string{"id", "object", "status", "model"}
	case "data.containers":
		return []string{"id", "object", "name", "status"}
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

func fillPath(p string) string {
	repl := []struct{ a, b string }{
		{"{model:path}", "gpt-4o-mini"}, {"{model}", "gpt-4o-mini"},
		{"{key:path}", "sk-abc"}, {"{key}", "sk-abc"},
		{"{id}", "id1"}, {"{file_id}", "file1"}, {"{batch_id}", "batch1"},
		{"{agent_id}", "agent1"}, {"{guardrail_id}", "g1"}, {"{policy_id}", "p1"},
		{"{server_id}", "s1"}, {"{vector_store_id}", "vs1"}, {"{run_id}", "run1"},
		{"{thread_id}", "th1"}, {"{assistant_id}", "as1"}, {"{container_id}", "c1"},
		{"{video_id}", "v1"}, {"{response_id}", "r1"}, {"{eval_id}", "e1"},
		{"{skill_id}", "sk1"}, {"{tool_name}", "tool1"}, {"{mcp_server_name}", "mcp1"},
		{"{plugin_name}", "plug1"}, {"{access_group}", "ag1"}, {"{organization_id}", "o1"},
		{"{team_id}", "t1"}, {"{user_id}", "u1"}, {"{project_id}", "pr1"},
		{"{budget_id}", "b1"}, {"{credential_name}", "cred1"}, {"{prompt_id}", "pmt1"},
		{"{search_tool_id}", "st1"}, {"{job_id}", "j1"}, {"{name}", "n1"},
		{"{endpoint}", "chat"}, {"{provider}", "openai"},
	}
	for _, r := range repl {
		p = strings.ReplaceAll(p, r.a, r.b)
	}
	for strings.Contains(p, "{") {
		i := strings.Index(p, "{")
		j := strings.Index(p, "}")
		if j < 0 {
			break
		}
		p = p[:i] + "x" + p[j+1:]
	}
	if p == "" {
		p = "/"
	}
	return p
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
