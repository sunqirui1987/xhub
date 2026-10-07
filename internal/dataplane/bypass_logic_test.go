package dataplane

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sunqirui1987/xhub/internal/auth"
	"github.com/sunqirui1987/xhub/internal/cache"
	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/hooks"
	"github.com/sunqirui1987/xhub/internal/iam"
	"github.com/sunqirui1987/xhub/internal/live"
	"github.com/sunqirui1987/xhub/internal/plugin"
	"github.com/sunqirui1987/xhub/internal/provider"
	"github.com/sunqirui1987/xhub/internal/router"

	_ "github.com/sunqirui1987/xhub/internal/provider/all"
)

// TestEndpointAndModelLogic walks one request from the selected endpoint type
// to the upstream call. Chat, Volcengine Seedance, Qiniu Seedance, and a custom
// Suno bypass are the four deployments.
func TestEndpointAndModelLogic(t *testing.T) {
	var mu sync.Mutex
	var seen []captured
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		seen = append(seen, captured{r.Method, r.URL.Path, r.URL.RawQuery, r.Header.Get("Authorization"), string(body)})
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v3/contents/generations/tasks":
			_, _ = w.Write([]byte(`{"id":"cgt-1"}`))
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v3/contents/generations/tasks/"):
			_, _ = w.Write([]byte(`{"id":"cgt-1","usage":{"completion_tokens":12}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v3/contents/generations/tasks":
			_, _ = w.Write([]byte(`{"items":[]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v3/contents/generations/tasks":
			_, _ = w.Write([]byte(`{"id":"qvideo-1"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/generate":
			_, _ = w.Write([]byte(`{"data":{"taskId":"suno-1"}}`))
		default:
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	defer upstream.Close()

	ark := deployment("volcengine/doubao-seedance-2-0-260128", "volcengine/doubao-seedance-2-0-260128", "ark-key", upstream.URL, "ark_contents_generation", nil)
	arkFast := deployment("volcengine/doubao-seedance-2-0-fast-260128", "volcengine/doubao-seedance-2-0-fast-260128", "ark-key-2", upstream.URL, "ark_contents_generation", nil)
	qiniu := deployment("qiniu/bytedance/doubao-seedance-2-0-260128", "qiniu/bytedance/doubao-seedance-2-0-260128", "qiniu-key", upstream.URL, "qiniu_contents_generation", nil)
	suno := deployment("V6", "V6", "suno-key", upstream.URL, "custom", map[string]any{
		"endpoint": map[string]any{
			"kind": "bypass", "api_base": upstream.URL, "model_field": "model", "task_id": "data.taskId",
			"actions": []any{
				map[string]any{"name": "create", "method": "POST", "public_path": "/api/v1/generate", "upstream_path": "/api/v1/generate"},
				map[string]any{"name": "get", "method": "GET", "public_path": "/api/v1/generate/record-info", "upstream_path": "/api/v1/generate/record-info", "task_query": "taskId"},
			},
		},
	})
	gpt := config.ModelEntry{
		ModelName: "gpt-4o",
		ModelInfo: map[string]any{"endpoint_types": []any{"chat", "completion"}},
	}
	host := &logicHost{
		cfg:    &config.Config{RouterSettings: config.RouterSettings{RoutingStrategy: "simple-shuffle"}},
		client: upstream.Client(),
		models: []config.ModelEntry{gpt, ark, arkFast, qiniu, suno},
		pins:   map[string]string{},
		billed: map[string]bool{},
	}

	gptBound := provider.BoundTypes(gpt)
	if len(gptBound) != 2 || gptBound[0].Actions[0].PublicPath != "/v1/chat/completions" || gptBound[1].Actions[0].PublicPath != "/v1/completions" {
		t.Fatalf("gpt endpoints %+v", gptBound)
	}
	arkBound := provider.BoundTypes(ark)
	if len(arkBound) != 1 || arkBound[0].ID != "ark_contents_generation" || len(arkBound[0].Actions) != 3 {
		t.Fatalf("ark endpoints %+v", arkBound)
	}

	if _, ok := provider.Match(http.MethodPost, "/v1/chat/completions", host.models); ok {
		t.Fatal("chat completions was claimed by bypass")
	}

	rec := host.call(t, http.MethodPost, "/api/v3/contents/generations/tasks", `{"model":"volcengine/doubao-seedance-2-0-260128","prompt":"street"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("ark create %d %s", rec.Code, rec.Body.String())
	}
	got := lastCall(t, &mu, seen)
	if got.auth != "Bearer ark-key" || !strings.Contains(got.body, `"model":"doubao-seedance-2-0-260128"`) || !strings.Contains(got.body, `"prompt":"street"`) {
		t.Fatalf("ark upstream %+v", got)
	}
	if host.spend[0].op != "ark_contents_generation:create" || host.spend[0].usage != nil || host.spend[0].depID == "" {
		t.Fatalf("create spend %+v", host.spend)
	}
	if len(host.notes) == 0 || host.notes[0].Provider != "volcengine" || host.notes[0].TTFTMs == nil || host.notes[0].DeploymentID == "" {
		t.Fatalf("create note %+v", host.notes)
	}

	rec = host.call(t, http.MethodGet, "/api/v3/contents/generations/tasks/cgt-1", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("ark get %d %s", rec.Code, rec.Body.String())
	}
	if host.spend[1].op != "ark_contents_generation:get" || host.spend[1].usage["completion_tokens"] == nil {
		t.Fatalf("first poll spend %+v", host.spend[1])
	}
	rec = host.call(t, http.MethodGet, "/api/v3/contents/generations/tasks/cgt-1", "")
	if host.spend[2].usage != nil {
		t.Fatalf("second poll charged again %+v", host.spend[2])
	}

	rec = host.call(t, http.MethodGet, "/v3/contents/generations/tasks/cgt-1", "")
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "unknown task") {
		t.Fatalf("qiniu path accepted a volcengine task %d %s", rec.Code, rec.Body.String())
	}

	delete(host.pins, "cgt-1")
	rec = host.call(t, http.MethodGet, "/api/v3/contents/generations/tasks/cgt-1", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expired pin %d", rec.Code)
	}

	before := len(snapshot(&mu, seen))
	rec = host.call(t, http.MethodGet, "/api/v3/contents/generations/tasks?page_size=2", "")
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), ark.ModelName) || !strings.Contains(rec.Body.String(), arkFast.ModelName) {
		t.Fatalf("two-key list %d %s", rec.Code, rec.Body.String())
	}
	if len(snapshot(&mu, seen)) != before {
		t.Fatal("two-key list called upstream")
	}
	host.models = []config.ModelEntry{ark}
	rec = host.call(t, http.MethodGet, "/api/v3/contents/generations/tasks?page_size=2", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("one-key list %d %s", rec.Code, rec.Body.String())
	}
	if lastCall(t, &mu, seen).query != "page_size=2" {
		t.Fatalf("query dropped %+v", lastCall(t, &mu, seen))
	}
	host.models = []config.ModelEntry{gpt, ark, arkFast, qiniu, suno}

	rec = host.call(t, http.MethodPost, "/v3/contents/generations/tasks", `{"model":"qiniu/bytedance/doubao-seedance-2-0-260128"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("qiniu create %d %s", rec.Code, rec.Body.String())
	}
	got = lastCall(t, &mu, seen)
	if got.auth != "Bearer qiniu-key" || !strings.Contains(got.body, `"model":"bytedance/doubao-seedance-2-0-260128"`) {
		t.Fatalf("qiniu upstream %+v", got)
	}

	rec = host.call(t, http.MethodPost, "/api/v1/generate", `{"model":"V6","prompt":"folk"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("suno create %d %s", rec.Code, rec.Body.String())
	}
	rec = host.call(t, http.MethodGet, "/api/v1/generate/record-info?taskId=suno-1", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("suno get %d %s", rec.Code, rec.Body.String())
	}
	got = lastCall(t, &mu, seen)
	if got.auth != "Bearer suno-key" || got.query != "taskId=suno-1" || host.spend[len(host.spend)-1].op != "custom:get" {
		t.Fatalf("suno follow %+v spend %+v", got, host.spend[len(host.spend)-1])
	}
}

type captured struct {
	method, path, query, auth, body string
}

func deployment(name, model, key, base, typeID string, extra map[string]any) config.ModelEntry {
	params := map[string]any{"model": model, "api_key": key, "api_base": base}
	for k, v := range extra {
		params[k] = v
	}
	return config.ModelEntry{
		ModelName:     name,
		LiteLLMParams: params,
		ModelInfo:     map[string]any{"mode": typeID, "endpoint_types": []any{typeID}},
	}
}

func snapshot(mu *sync.Mutex, seen []captured) []captured {
	mu.Lock()
	defer mu.Unlock()
	out := make([]captured, len(seen))
	copy(out, seen)
	return out
}

func lastCall(t *testing.T, mu *sync.Mutex, seen []captured) captured {
	t.Helper()
	got := snapshot(mu, seen)
	if len(got) == 0 {
		t.Fatal("upstream was not called")
	}
	return got[len(got)-1]
}

type spendNote struct {
	op, depID string
	usage     map[string]any
}

type logicHost struct {
	cfg    *config.Config
	client *http.Client
	models []config.ModelEntry
	pins   map[string]string
	billed map[string]bool
	spend  []spendNote
	notes  []CallNote
}

func (h *logicHost) call(t *testing.T, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	hit, ok := provider.Match(method, req.URL.Path, h.models)
	if !ok {
		t.Fatalf("no endpoint for %s %s", method, target)
	}
	rec := httptest.NewRecorder()
	ServeBypass(h, rec, req, hit)
	return rec
}

func (h *logicHost) RequireLLMPrincipal(http.ResponseWriter, *http.Request) *auth.Principal {
	return &auth.Principal{Kind: "session"}
}
func (h *logicHost) ResolveRequest(*http.Request) (*auth.Principal, error) {
	return &auth.Principal{Kind: "session"}, nil
}
func (h *logicHost) GatewayConfig() *config.Config   { return h.cfg }
func (h *logicHost) HTTPClient() *http.Client        { return h.client }
func (h *logicHost) ResponseCache() *cache.DualCache { return cache.New() }
func (h *logicHost) HookEngine() *hooks.Engine       { return hooks.New() }
func (h *logicHost) Extensions() *plugin.Registry    { return plugin.New() }
func (h *logicHost) RouteState() router.State        { return router.State{} }
func (h *logicHost) RouterDocument() map[string]any  { return nil }
func (h *logicHost) GuardrailBlocks(string, map[string]any) (bool, string) {
	return false, ""
}
func (h *logicHost) AttachCredential(dep config.ModelEntry) (config.ModelEntry, error) {
	return dep, nil
}
func (h *logicHost) IncBusy(string)                                                      {}
func (h *logicHost) DecBusy(string)                                                      {}
func (h *logicHost) NoteFailure(string)                                                  {}
func (h *logicHost) NoteLatency(string, float64)                                         {}
func (h *logicHost) SetChatHeaders(http.ResponseWriter, *auth.Principal, string, string) {}
func (h *logicHost) RecordSpend(_ http.ResponseWriter, _ *auth.Principal, _, _, op string, usage map[string]any, _ time.Time, _ bool, _ int, depID string) {
	h.spend = append(h.spend, spendNote{op: op, depID: depID, usage: usage})
}
func (h *logicHost) RememberExchange(string, *http.Request, []byte, []byte) {}
func (h *logicHost) PlanRoute(*http.Request, string, map[string]any, *auth.Principal) RoutePlan {
	return RoutePlan{}
}
func (h *logicHost) CommitRoute(RoutePlan, string, string) {}
func (h *logicHost) AnnotateCall(_ string, note CallNote)  { h.notes = append(h.notes, note) }
func (h *logicHost) PinnedDeployment(string) string        { return "" }
func (h *logicHost) FindDeployment(id string) (config.ModelEntry, bool) {
	for _, m := range h.models {
		if router.DeploymentID(m) == id {
			return m, true
		}
	}
	return config.ModelEntry{}, false
}
func (h *logicHost) PinOfficial(taskID, deploymentID string) { h.pins[taskID] = deploymentID }
func (h *logicHost) OfficialDeployment(taskID string) string { return h.pins[taskID] }
func (h *logicHost) OfficialBilled(taskID string) bool       { return h.billed[taskID] }
func (h *logicHost) MarkOfficialBilled(taskID string)        { h.billed[taskID] = true }
func (h *logicHost) WriteCacheHit(http.ResponseWriter, *auth.Principal, string, string, string, string, []byte, time.Time) {
}
func (h *logicHost) WriteChatJSON(http.ResponseWriter, *auth.Principal, string, string, string, string, string, []byte, int, time.Time, string) {
}
func (h *logicHost) EnforceIdentityLimits(http.ResponseWriter, string, *auth.Principal, string, int) bool {
	return true
}
func (h *logicHost) Redis() *live.Client         { return nil }
func (h *logicHost) Models() []config.ModelEntry { return h.models }
func (h *logicHost) BusyMap() map[string]int     { return map[string]int{} }
func (h *logicHost) Identity() *iam.DB           { return nil }
