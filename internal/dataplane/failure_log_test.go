package dataplane

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sunqirui1987/xhub/internal/auth"
	"github.com/sunqirui1987/xhub/internal/cache"
	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/hooks"
	"github.com/sunqirui1987/xhub/internal/live"
	"github.com/sunqirui1987/xhub/internal/plugin"
	"github.com/sunqirui1987/xhub/internal/router"
	"github.com/sunqirui1987/xhub/internal/store"
)

// logHost is the gateway surface Serve needs. Failure paths return before spend and Redis are used.
type logHost struct {
	cfg    *config.Config
	client *http.Client
	cache  *cache.DualCache
	hooks  *hooks.Engine
	ext    *plugin.Registry
}

func newLogHost(cfg *config.Config, client *http.Client) *logHost {
	if client == nil {
		client = http.DefaultClient
	}
	return &logHost{
		cfg:    cfg,
		client: client,
		cache:  cache.New(),
		hooks:  hooks.New(),
		ext:    plugin.New(),
	}
}

func (h *logHost) RequireLLMPrincipal(http.ResponseWriter, *http.Request) *auth.Principal {
	return &auth.Principal{Kind: "session"}
}
func (h *logHost) ResolveRequest(*http.Request) (*auth.Principal, error) {
	return &auth.Principal{Kind: "session"}, nil
}
func (h *logHost) GatewayConfig() *config.Config   { return h.cfg }
func (h *logHost) HTTPClient() *http.Client        { return h.client }
func (h *logHost) ResponseCache() *cache.DualCache { return h.cache }
func (h *logHost) HookEngine() *hooks.Engine       { return h.hooks }
func (h *logHost) Extensions() *plugin.Registry    { return h.ext }
func (h *logHost) RouteState() router.State        { return router.State{} }
func (h *logHost) RouterDocument() map[string]any  { return nil }
func (h *logHost) GuardrailBlocks(map[string]any) (bool, string) {
	return false, ""
}
func (h *logHost) AttachCredential(dep config.ModelEntry) config.ModelEntry { return dep }
func (h *logHost) IncBusy(string)                                           {}
func (h *logHost) DecBusy(string)                                           {}
func (h *logHost) NoteFailure(string)                                       {}
func (h *logHost) NoteLatency(string, float64)                              {}
func (h *logHost) SetChatHeaders(http.ResponseWriter, *auth.Principal, string, string) {
}
func (h *logHost) RecordSpend(http.ResponseWriter, *auth.Principal, string, string, map[string]any, time.Time, bool, string) {
}
func (h *logHost) WriteCacheHit(http.ResponseWriter, *auth.Principal, string, string, string, []byte, time.Time) {
}
func (h *logHost) WriteChatJSON(http.ResponseWriter, *auth.Principal, string, string, string, string, string, []byte, int, time.Time, string) {
}
func (h *logHost) EnforceIdentityLimits(http.ResponseWriter, string, *auth.Principal, string, int) bool {
	return true
}
func (h *logHost) Redis() *live.Client         { return nil }
func (h *logHost) Models() []config.ModelEntry { return h.cfg.ModelList }
func (h *logHost) BusyMap() map[string]int     { return map[string]int{} }
func (h *logHost) SpendStore() *store.Store    { return nil }

func chatConfig(entries ...config.ModelEntry) *config.Config {
	return &config.Config{
		ModelList: entries,
		RouterSettings: config.RouterSettings{
			RoutingStrategy: "simple-shuffle",
			NumRetries:      1,
		},
	}
}

func chatRequest(t *testing.T, model string, stream bool) *http.Request {
	t.Helper()
	payload := map[string]any{
		"model":    model,
		"messages": []any{map[string]any{"role": "user", "content": "hi"}},
	}
	if stream {
		payload["stream"] = true
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(raw))
	req.Header.Set("Authorization", "Bearer sk-local-master")
	return req
}

func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(prev) })
	return &buf
}

func assertNoSecrets(t *testing.T, line string) {
	t.Helper()
	if strings.Contains(line, "sk-local-master") || strings.Contains(line, "Bearer") || strings.Contains(line, "sk-") {
		t.Fatalf("log leaked credentials: %s", line)
	}
}

func TestServeLogsBuildSkipAndTerminalAuth(t *testing.T) {
	buf := captureLog(t)
	h := newLogHost(chatConfig(config.ModelEntry{
		ModelName: "broken",
		LiteLLMParams: map[string]any{
			"model":    "base_llm/some-model",
			"api_key":  "sk-local-master",
			"api_base": "https://example.invalid/v1",
		},
	}), nil)
	rec := httptest.NewRecorder()
	Serve(h, rec, chatRequest(t, "broken", false), "chat")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	line := buf.String()
	for _, want := range []string{
		"error upstream encode path=/v1/chat/completions provider=base_llm model=some-model err=provider_not_implemented",
		"error dataplane path=/v1/chat/completions status=401 code=authentication_error provider=base_llm",
	} {
		if !strings.Contains(line, want) {
			t.Fatalf("log missing %q\n%s", want, line)
		}
	}
	assertNoSecrets(t, line)
}

func TestServeLogsMissingCredential(t *testing.T) {
	buf := captureLog(t)
	h := newLogHost(chatConfig(config.ModelEntry{
		ModelName: "gpt-4o-mini",
		LiteLLMParams: map[string]any{
			"model":    "openai/gpt-4o-mini",
			"api_base": "https://api.openai.com/v1",
		},
	}), nil)
	rec := httptest.NewRecorder()
	Serve(h, rec, chatRequest(t, "gpt-4o-mini", false), "chat")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	line := buf.String()
	for _, want := range []string{
		"debug skip deployment path=/v1/chat/completions provider=openai model=gpt-4o-mini reason=missing api key or api base",
		"error dataplane path=/v1/chat/completions status=401 code=authentication_error provider=openai",
	} {
		if !strings.Contains(line, want) {
			t.Fatalf("log missing %q\n%s", want, line)
		}
	}
	assertNoSecrets(t, line)
}

func TestServeLogsUnimplementedProvider(t *testing.T) {
	buf := captureLog(t)
	h := newLogHost(chatConfig(config.ModelEntry{
		ModelName: "ghost",
		LiteLLMParams: map[string]any{
			"model":               "openai/gpt-4o-mini",
			"custom_llm_provider": "   ",
			"api_key":             "sk-local-master",
			"api_base":            "https://api.openai.com/v1",
		},
	}), nil)
	rec := httptest.NewRecorder()
	Serve(h, rec, chatRequest(t, "ghost", false), "chat")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	line := buf.String()
	if !strings.Contains(line, "error dataplane path=/v1/chat/completions status=400 code=provider_not_implemented") {
		t.Fatalf("log missing provider_not_implemented: %s", line)
	}
	assertNoSecrets(t, line)
}

func TestServeLogsEmptyStreamAndUpstreamStatus(t *testing.T) {
	empty := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer empty.Close()
	failed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, `{"error":"sk-local-master Bearer sk-local-master"}`)
	}))
	defer failed.Close()

	t.Run("empty stream", func(t *testing.T) {
		buf := captureLog(t)
		h := newLogHost(chatConfig(config.ModelEntry{
			ModelName: "gpt-4o-mini",
			LiteLLMParams: map[string]any{
				"model":    "openai/gpt-4o-mini",
				"api_key":  "sk-local-master",
				"api_base": empty.URL + "/v1",
			},
		}), empty.Client())
		rec := httptest.NewRecorder()
		Serve(h, rec, chatRequest(t, "gpt-4o-mini", true), "chat")
		if rec.Code != http.StatusBadGateway {
			t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
		}
		line := buf.String()
		for _, want := range []string{
			"error upstream stream path=/v1/chat/completions provider=openai model=gpt-4o-mini",
			"err=empty upstream stream",
			"error dataplane path=/v1/chat/completions status=502 code=upstream_error detail=empty upstream stream",
		} {
			if !strings.Contains(line, want) {
				t.Fatalf("log missing %q\n%s", want, line)
			}
		}
		if strings.Contains(line, empty.URL) {
			t.Fatalf("log included full upstream url: %s", line)
		}
		assertNoSecrets(t, line)
	})

	t.Run("upstream 500", func(t *testing.T) {
		buf := captureLog(t)
		h := newLogHost(chatConfig(config.ModelEntry{
			ModelName: "gpt-4o-mini",
			LiteLLMParams: map[string]any{
				"model":    "openai/gpt-4o-mini",
				"api_key":  "sk-local-master",
				"api_base": failed.URL + "/v1",
			},
		}), failed.Client())
		rec := httptest.NewRecorder()
		Serve(h, rec, chatRequest(t, "gpt-4o-mini", false), "chat")
		if rec.Code != http.StatusBadGateway {
			t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
		}
		line := buf.String()
		for _, want := range []string{
			"error upstream status path=/v1/chat/completions provider=openai model=gpt-4o-mini",
			"status=500",
			"error dataplane path=/v1/chat/completions status=502 code=upstream_error detail=upstream 500",
		} {
			if !strings.Contains(line, want) {
				t.Fatalf("log missing %q\n%s", want, line)
			}
		}
		if strings.Contains(line, failed.URL) {
			t.Fatalf("log included full upstream url: %s", line)
		}
		assertNoSecrets(t, line)
	})
}
