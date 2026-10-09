package dataplane

import (
	"github.com/sunqirui1987/xhub/internal/auth"
	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/provider"
	"github.com/sunqirui1987/xhub/internal/router"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func officialHost(up *httptest.Server, models ...config.ModelEntry) *logicHost {
	return &logicHost{cfg: &config.Config{}, client: up.Client(), models: models, pins: map[string]string{}}
}

func TestOfficialTemplateWeightsAndCredentialPin(t *testing.T) {
	var seen []string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Header.Get("Authorization"))
		io.WriteString(w, `{"id":"task-1"}`)
	}))
	defer up.Close()
	a := deployment("video", "volcengine/model", "key-a", up.URL, "ark_contents_generation", map[string]any{"litellm_credential_name": "a", "weight": 0.0})
	b := deployment("video", "volcengine/model", "key-b", up.URL, "ark_contents_generation", map[string]any{"litellm_credential_name": "b", "weight": 1.0})
	h := officialHost(up, a, b)
	h.settings = map[string]any{"routing_strategy": "weighted-split"}
	if rec := h.call(t, http.MethodPost, "/api/v3/contents/generations/tasks", `{"model":"video"}`); rec.Code != 200 {
		t.Fatal(rec.Code, rec.Body.String())
	}
	if h.spend[0].depID != router.CooldownID(b) {
		t.Fatalf("wrong credential recorded: %+v", h.spend)
	}
	h.settings = map[string]any{"routing_strategy": "simple-shuffle"}
	if rec := h.call(t, http.MethodGet, "/api/v3/contents/generations/tasks/task-1", ""); rec.Code != 200 {
		t.Fatal(rec.Code, rec.Body.String())
	}
	if len(seen) != 2 || seen[0] != "Bearer key-b" || seen[1] != "Bearer key-b" {
		t.Fatalf("task credential changed: %v", seen)
	}
}

func TestOfficialUsesModelRoutingOverride(t *testing.T) {
	var seen []string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Header.Get("Authorization"))
		io.WriteString(w, "{\"id\":\"task-model-policy\"}")
	}))
	defer up.Close()
	a := deployment("video", "provider/model-a", "key-a", up.URL, "ark_contents_generation", map[string]any{"weight": 9.0, "deployment_id": "model-a"})
	b := deployment("video", "provider/model-b", "key-b", up.URL, "ark_contents_generation", map[string]any{"weight": 1.0, "deployment_id": "model-b"})
	h := officialHost(up, a, b)
	h.settings = map[string]any{
		"routing_strategy":      "simple-shuffle",
		"routing_strategy_args": map[string]any{"weights": map[string]any{"deployment:model-a": 1.0}},
		"model_routing": []any{map[string]any{
			"model_name": "video", "routing_strategy": "weighted-split",
			"routing_strategy_args": map[string]any{"weights": map[string]any{
				"deployment:model-a": 0.0, "deployment:model-b": 1.0,
			}},
		}},
	}
	rec := h.call(t, http.MethodPost, "/api/v3/contents/generations/tasks", "{\"model\":\"video\"}")
	if rec.Code != http.StatusOK || len(seen) != 1 || seen[0] != "Bearer key-b" {
		t.Fatalf("official model policy was not applied: status=%d seen=%v body=%s", rec.Code, seen, rec.Body.String())
	}
}

func TestOfficialTemplateRetriesHTTPFailure(t *testing.T) {
	var calls atomic.Int32
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(429)
			return
		}
		io.WriteString(w, `{"id":"task-1"}`)
	}))
	defer up.Close()
	h := officialHost(up, deployment("video", "volcengine/model", "key", up.URL, "ark_contents_generation", nil))
	h.settings = map[string]any{"num_retries": 2}
	rec := h.call(t, http.MethodPost, "/api/v3/contents/generations/tasks", `{"model":"video"}`)
	if rec.Code != 200 || calls.Load() != 2 || len(h.failures) != 1 {
		t.Fatalf("status=%d calls=%d failures=%v", rec.Code, calls.Load(), h.failures)
	}
}

func TestOfficialTimeoutDoesNotReplayAmbiguousCreate(t *testing.T) {
	var calls atomic.Int32
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		select {
		case <-r.Context().Done():
		case <-time.After(150 * time.Millisecond):
		}
	}))
	defer up.Close()
	h := officialHost(up, deployment("video", "volcengine/model", "key", up.URL, "ark_contents_generation", nil))
	h.settings = map[string]any{"num_retries": 3, "timeout": 0.03}
	rec := h.call(t, http.MethodPost, "/api/v3/contents/generations/tasks", `{"model":"video"}`)
	if rec.Code != 502 || calls.Load() != 1 {
		t.Fatalf("ambiguous create replayed: status=%d calls=%d", rec.Code, calls.Load())
	}
}

func TestOfficialTaskScopesAndPendingUsage(t *testing.T) {
	p := &auth.Principal{Hash: "a"}
	key := officialTaskScope(p, "ark", "same")
	if key == officialTaskScope(&auth.Principal{Hash: "b"}, "ark", "same") || key == officialTaskScope(p, "qiniu", "same") {
		t.Fatal("task scope collision")
	}
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"status":"running","usage":{"completion_tokens":12}}`)
	}))
	defer up.Close()
	dep := deployment("video", "volcengine/model", "key", up.URL, "ark_contents_generation", nil)
	h := officialHost(up, dep)
	h.pins[officialTaskScope(&auth.Principal{UserID: "test-user"}, "ark_contents_generation", "task")] = router.CooldownID(dep)
	h.call(t, http.MethodGet, "/api/v3/contents/generations/tasks/task", "")
	if len(h.spend) != 1 || h.spend[0].usage != nil {
		t.Fatalf("in-progress task billed: %+v", h.spend)
	}
}

func TestOfficialForwardDoesNotLeakGatewayCredentials(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, name := range []string{"Cookie", "X-Api-Key", "Api-Key", "X-Litellm-Api-Key", "X-Private"} {
			if r.Header.Get(name) != "" {
				t.Errorf("leaked header %s", name)
			}
		}
		if r.Header.Get("Authorization") != "Bearer provider-key" {
			t.Error("missing provider credential")
		}
		io.WriteString(w, `{}`)
	}))
	defer up.Close()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, name := range []string{"Cookie", "X-Api-Key", "Api-Key", "X-Litellm-Api-Key", "X-Private"} {
		r.Header.Set(name, "gateway-secret")
	}
	r.Header.Set("Connection", "X-Private")
	response, err := forwardOfficial(officialHost(up), r, http.MethodGet, up.URL, "provider-key", nil, r.Header, provider.Transport{Auth: provider.AuthConfig{Header: "Authorization", Prefix: "Bearer"}}, nil)
	status := response.StatusCode
	if err != nil || status != 200 {
		t.Fatal(status, err)
	}
}
