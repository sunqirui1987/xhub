package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestCatalogPathSweepNo404(t *testing.T) {
	s, master := testEnv(t)
	raw, err := os.ReadFile("routes.json")
	if err != nil {
		t.Fatal(err)
	}
	var routes []catRoute
	if err := json.Unmarshal(raw, &routes); err != nil {
		t.Fatal(err)
	}
	notFound := 0
	var samples []string
	for _, rt := range routes {
		path := fillPath(rt.P)
		var rec *httptest.ResponseRecorder
		if rt.M == http.MethodGet || rt.M == http.MethodHead || rt.M == "GET" || rt.M == "HEAD" {
			rec = doJSON(t, s.Handler(), rt.M, path, master, nil)
		} else {
			rec = doJSON(t, s.Handler(), rt.M, path, master, map[string]any{"model": "gpt-4o-mini"})
		}
		if removedColumnRoute(path) {
			if rec.Code != 404 {
				t.Fatalf("%s %s removed column want 404 got %d", rt.M, path, rec.Code)
			}
			continue
		}
		if rec.Code == 404 && isTypedResource404(rec) {
			continue
		}
		if rec.Code == 404 {
			notFound++
			if len(samples) < 8 {
				samples = append(samples, fmt.Sprintf("%s %s -> %d %s", rt.M, path, rec.Code, rec.Body.String()))
			}
		}
	}
	if notFound != 0 {
		t.Fatalf("not_found=%d examples=%v total=%d", notFound, samples, len(routes))
	}
}

func TestDataPlaneCatalogStubScan(t *testing.T) {
	s, master := testEnv(t)
	sk := mintLLM(t, s, master)
	raw, err := os.ReadFile("routes.json")
	if err != nil {
		t.Fatal(err)
	}
	var routes []catRoute
	if err := json.Unmarshal(raw, &routes); err != nil {
		t.Fatal(err)
	}
	var stubs []string
	checked := 0
	for _, rt := range routes {
		if !strings.EqualFold(rt.M, http.MethodPost) {
			continue
		}
		path := fillPath(rt.P)
		if !isDataPlanePath(path) {
			continue
		}
		checked++
		rec := doJSON(t, s.Handler(), rt.M, path, sk, map[string]any{
			"model": "gpt-4o-mini", "prompt": "a cat", "input": "hi", "query": "q",
			"documents": []any{"a"}, "messages": []any{map[string]any{"role": "user", "content": "hi"}},
			"purpose": "batch",
		})
		if rec.Code != 200 {
			continue
		}
		if isGenericStub(rec.Body.String()) {
			stubs = append(stubs, rt.M+" "+path+" "+rec.Body.String())
		}
	}
	if len(stubs) != 0 {
		n := 8
		if len(stubs) < n {
			n = len(stubs)
		}
		t.Fatalf("stub_bodies=%d examples=%v checked=%d", len(stubs), stubs[:n], checked)
	}
	t.Logf("data-plane POST 200 non-stub checked=%d", checked)
}

func fillPath(p string) string {
	p = strings.ReplaceAll(p, "{model:path}", "gpt-4o-mini")
	p = strings.ReplaceAll(p, "{model}", "gpt-4o-mini")
	p = strings.ReplaceAll(p, "{key:path}", "sk-abc")
	p = strings.ReplaceAll(p, "{key}", "sk-abc")
	p = strings.ReplaceAll(p, "{id}", "id1")
	p = strings.ReplaceAll(p, "{file_id}", "file1")
	p = strings.ReplaceAll(p, "{batch_id}", "batch1")
	p = strings.ReplaceAll(p, "{agent_id}", "agent1")
	p = strings.ReplaceAll(p, "{guardrail_id}", "g1")
	p = strings.ReplaceAll(p, "{policy_id}", "p1")
	p = strings.ReplaceAll(p, "{server_id}", "s1")
	p = strings.ReplaceAll(p, "{vector_store_id}", "vs1")
	p = strings.ReplaceAll(p, "{run_id}", "run1")
	p = strings.ReplaceAll(p, "{thread_id}", "th1")
	p = strings.ReplaceAll(p, "{assistant_id}", "as1")
	p = strings.ReplaceAll(p, "{container_id}", "c1")
	p = strings.ReplaceAll(p, "{video_id}", "v1")
	p = strings.ReplaceAll(p, "{response_id}", "r1")
	p = strings.ReplaceAll(p, "{eval_id}", "e1")
	p = strings.ReplaceAll(p, "{skill_id}", "sk1")
	p = strings.ReplaceAll(p, "{tool_name}", "tool1")
	p = strings.ReplaceAll(p, "{mcp_server_name}", "mcp1")
	p = strings.ReplaceAll(p, "{plugin_name}", "plug1")
	p = strings.ReplaceAll(p, "{access_group}", "ag1")
	p = strings.ReplaceAll(p, "{organization_id}", "o1")
	p = strings.ReplaceAll(p, "{team_id}", "t1")
	p = strings.ReplaceAll(p, "{user_id}", "u1")
	p = strings.ReplaceAll(p, "{project_id}", "pr1")
	p = strings.ReplaceAll(p, "{budget_id}", "b1")
	p = strings.ReplaceAll(p, "{credential_name}", "cred1")
	p = strings.ReplaceAll(p, "{prompt_id}", "pmt1")
	p = strings.ReplaceAll(p, "{search_tool_id}", "st1")
	p = strings.ReplaceAll(p, "{job_id}", "j1")
	p = strings.ReplaceAll(p, "{name}", "n1")
	p = strings.ReplaceAll(p, "{endpoint}", "chat")
	p = strings.ReplaceAll(p, "{provider}", "openai")
	// leftover braces
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
