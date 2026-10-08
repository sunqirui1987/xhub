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
	"github.com/sunqirui1987/xhub/internal/gateway/prefs"
	"github.com/sunqirui1987/xhub/internal/hooks"
	"github.com/sunqirui1987/xhub/internal/iam"
	"github.com/sunqirui1987/xhub/internal/live"
	"github.com/sunqirui1987/xhub/internal/plugin"
	"github.com/sunqirui1987/xhub/internal/router"
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
func (h *logHost) RouteSettingsFor(*auth.Principal) prefs.RouteSettings {
	return prefs.PlatformSettings(nil)
}

func (h *logHost) GatewayConfig() *config.Config   { return h.cfg }
func (h *logHost) HTTPClient() *http.Client        { return h.client }
func (h *logHost) ResponseCache() *cache.DualCache { return h.cache }
func (h *logHost) HookEngine() *hooks.Engine       { return h.hooks }
func (h *logHost) Extensions() *plugin.Registry    { return h.ext }
func (h *logHost) RouteState() router.State        { return router.State{} }
func (h *logHost) RouterDocument() map[string]any  { return nil }
func (h *logHost) GuardrailBlocks(string, map[string]any) (bool, string) {
	return false, ""
}
func (h *logHost) AttachCredential(dep config.ModelEntry) (config.ModelEntry, error) { return dep, nil }
func (h *logHost) IncBusy(string)                                                    {}
func (h *logHost) DecBusy(string)                                                    {}
func (h *logHost) NoteFailure(string, prefs.RouteSettings)                           {}
func (h *logHost) NoteLatency(string, float64)                                       {}
func (h *logHost) SetChatHeaders(http.ResponseWriter, *auth.Principal, string, string) {
}
func (h *logHost) RecordSpend(http.ResponseWriter, *auth.Principal, string, string, string, map[string]any, time.Time, bool, int, string) {
}
func (h *logHost) RememberExchange(string, *http.Request, []byte, []byte) {}
func (h *logHost) PlanRoute(*http.Request, string, map[string]any, *auth.Principal) RoutePlan {
	return RoutePlan{}
}
func (h *logHost) CommitRoute(RoutePlan, string, string)           {}
func (h *logHost) AnnotateCall(string, CallNote)                   {}
func (h *logHost) PinnedDeployment(string) string                  { return "" }
func (h *logHost) FindDeployment(string) (config.ModelEntry, bool) { return config.ModelEntry{}, false }
func (h *logHost) PinOfficial(string, string)                      {}
func (h *logHost) OfficialDeployment(string) string                { return "" }
func (h *logHost) OfficialBilled(string) bool                      { return false }
func (h *logHost) MarkOfficialBilled(string)                       {}
func (h *logHost) WriteCacheHit(http.ResponseWriter, *auth.Principal, string, string, string, string, []byte, time.Time) {
}
func (h *logHost) WriteChatJSON(http.ResponseWriter, *auth.Principal, string, string, string, string, string, []byte, int, time.Time, string) {
}
func (h *logHost) EnforceIdentityLimits(http.ResponseWriter, string, *auth.Principal, string, int) bool {
	return true
}
func (h *logHost) Redis() *live.Client         { return nil }
func (h *logHost) Models() []config.ModelEntry { return h.cfg.ModelList }
func (h *logHost) BusyMap() map[string]int     { return map[string]int{} }
func (h *logHost) Identity() *iam.DB           { return nil }

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
	// base_llm passes the provider check and then has no encoder, so this is a
	// request the gateway cannot express -- not a missing credential. It used to
	// answer 401 "no upstream API key configured" with the key set, which sent an
	// operator looking for a credential that was never the problem.
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	line := buf.String()
	t.Log(line)
	for _, want := range []string{
		"trace dataplane hop path=/v1/chat/completions",
		"error upstream encode path=/v1/chat/completions provider=base_llm model=some-model err=provider_not_implemented",
		"error dataplane path=/v1/chat/completions status=400 code=provider_not_implemented provider=base_llm",
	} {
		if !strings.Contains(line, want) {
			t.Fatalf("log missing %q\n%s", want, line)
		}
	}
	// The body has to say what is wrong, not blame a credential that was set.
	body := rec.Body.String()
	if !strings.Contains(body, "encoded for base_llm") {
		t.Fatalf("the refusal does not name the encoding problem: %s", body)
	}
	if strings.Contains(body, "API key") {
		t.Fatalf("the refusal blames a credential that was configured: %s", body)
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
	t.Log(line)
	for _, want := range []string{
		"trace dataplane hop path=/v1/chat/completions",
		"trace process path=/v1/chat/completions step=start op=chat",
		"debug process path=/v1/chat/completions step=auth ok=true kind=session",
		"debug process path=/v1/chat/completions step=route model=gpt-4o-mini deployments=1 stream=false",
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
	t.Log(line)
	if !strings.Contains(line, "trace dataplane hop path=/v1/chat/completions") || !strings.Contains(line, "error dataplane path=/v1/chat/completions status=400 code=provider_not_implemented") {
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
		t.Log(line)
		for _, want := range []string{
			"trace dataplane hop path=/v1/chat/completions",
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
		t.Log(line)
		for _, want := range []string{
			"trace dataplane hop path=/v1/chat/completions",
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

func TestServeLogsCacheHitAndStreamMetrics(t *testing.T) {
	raw := []byte(`{"messages":[{"content":"hi","role":"user"}],"model":"gpt-4o-mini"}`)
	h := newLogHost(chatConfig(config.ModelEntry{
		ModelName: "gpt-4o-mini",
		LiteLLMParams: map[string]any{
			"model":    "openai/gpt-4o-mini",
			"api_key":  "test-key",
			"api_base": "https://api.openai.com/v1",
		},
	}), nil)
	h.cache.Set(cache.Key("", "chat", "gpt-4o-mini", string(raw)), []byte(`{"usage":{"prompt_tokens":11,"completion_tokens":7,"total_tokens":18}}`))

	buf := captureLog(t)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(raw))
	rec := httptest.NewRecorder()
	Serve(h, rec, req, "chat")
	if rec.Code != http.StatusOK {
		t.Fatalf("cache status %d body %s", rec.Code, rec.Body.String())
	}
	line := buf.String()
	t.Log(line)
	for _, want := range []string{
		"step=cache hit=true model=gpt-4o-mini",
		"step=metrics model=gpt-4o-mini cache_hit=true",
		"ttft=",
		"prompt_tokens=11",
		"completion_tokens=7",
		"total_tokens=18",
		"tokens_per_s=0.00",
	} {
		if !strings.Contains(line, want) {
			t.Fatalf("cache log missing %q\n%s", want, line)
		}
	}

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "data: {\"usage\":{\"prompt_tokens\":5,\"completion_tokens\":4,\"total_tokens\":9}}\n\n")
	}))
	defer upstream.Close()
	streamHost := newLogHost(chatConfig(config.ModelEntry{
		ModelName: "gpt-4o-mini",
		LiteLLMParams: map[string]any{
			"model":    "openai/gpt-4o-mini",
			"api_key":  "test-key",
			"api_base": upstream.URL + "/v1",
		},
	}), upstream.Client())
	streamRaw := []byte(`{"messages":[{"content":"hi","role":"user"}],"model":"gpt-4o-mini","stream":true}`)
	buf = captureLog(t)
	sreq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(streamRaw))
	srec := httptest.NewRecorder()
	Serve(streamHost, srec, sreq, "chat")
	if srec.Code != http.StatusOK {
		t.Fatalf("stream status %d body %s", srec.Code, srec.Body.String())
	}
	sline := buf.String()
	t.Log(sline)
	for _, want := range []string{
		"step=metrics model=gpt-4o-mini cache_hit=false",
		"ttft=",
		"prompt_tokens=5",
		"completion_tokens=4",
		"total_tokens=9",
		"tokens_per_s=",
	} {
		if !strings.Contains(sline, want) {
			t.Fatalf("stream log missing %q\n%s", want, sline)
		}
	}
	if strings.Contains(sline, "http://") || strings.Contains(sline, "https://") {
		t.Fatalf("log included a full upstream url: %s", sline)
	}
}
