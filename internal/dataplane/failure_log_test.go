package dataplane

import (
	"bytes"
	"encoding/json"
	"errors"
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
	"github.com/sunqirui1987/xhub/internal/provider"
	"github.com/sunqirui1987/xhub/internal/router"
)

// logHost is the gateway surface Serve needs. Failure paths return before spend and Redis are used.
type logHost struct {
	cfg       *config.Config
	client    *http.Client
	cache     *cache.DualCache
	hooks     *hooks.Engine
	ext       *plugin.Registry
	spends    []capturedSpend
	exchanges map[string][]byte
}

// capturedSpend 保存夹具观察到的最终状态；测试用它验证失败确实进入日志链，无外部副作用。
type capturedSpend struct {
	callID string
	status int
}

func newLogHost(cfg *config.Config, client *http.Client) *logHost {
	if client == nil {
		client = http.DefaultClient
	}
	return &logHost{
		cfg:       cfg,
		client:    client,
		cache:     cache.New(),
		hooks:     hooks.New(),
		ext:       plugin.New(),
		exchanges: map[string][]byte{},
	}
}

func (h *logHost) RequireLLMPrincipal(http.ResponseWriter, *http.Request) *auth.Principal {
	return &auth.Principal{Kind: "session", UserID: "log-user"}
}
func (h *logHost) ResolveRequest(*http.Request) (*auth.Principal, error) {
	return &auth.Principal{Kind: "session", UserID: "log-user"}, nil
}
func (h *logHost) RouteSettingsFor(*auth.Principal) prefs.RouteSettings {
	return prefs.BuiltinSettings()
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
func (h *logHost) RecordSpend(_ http.ResponseWriter, _ *auth.Principal, callID, _ string, _ string, _ map[string]any, _ time.Time, _ bool, status int, _ string) {
	h.spends = append(h.spends, capturedSpend{callID: callID, status: status})
}
func (h *logHost) RememberExchange(callID string, _ *http.Request, _ []byte, response []byte) {
	h.exchanges[callID] = append([]byte(nil), response...)
}
func (h *logHost) PlanRoute(*http.Request, string, map[string]any, *auth.Principal) RoutePlan {
	return RoutePlan{}
}
func (h *logHost) CommitRoute(RoutePlan, string, string)           {}
func (h *logHost) AnnotateCall(string, CallNote)                   {}
func (h *logHost) PinnedDeployment(string) string                  { return "" }
func (h *logHost) FindDeployment(string) (config.ModelEntry, bool) { return config.ModelEntry{}, false }
func (h *logHost) PinOfficial(string, string)                      {}
func (h *logHost) OfficialDeployment(string) string                { return "" }
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
	for i := range entries {
		if entries[i].ModelInfo == nil {
			entries[i].ModelInfo = map[string]any{}
		}
		entries[i].ModelInfo["transport"] = "bypass_openai_chat"
		if entries[i].ParamString("custom_llm_provider", "") == "" {
			entries[i].LiteLLMParams["custom_llm_provider"] = "custom-test"
		}
		entries[i].ModelInfo["endpoint_types"] = []string{"chat"}
	}
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

// TestServePersistsCompleteUpstreamFailure 验证重试耗尽后保留上游完整 500 正文。
// 前置条件是本地假上游固定失败；结果应记录客户端 502 和原始 JSON；本地服务由 Cleanup 关闭。
func TestServePersistsCompleteUpstreamFailure(t *testing.T) {
	rawError := []byte("{\"error\":{\"message\":\"complete upstream failure\",\"detail\":\"原始正文\"}}")
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write(rawError)
	}))
	t.Cleanup(up.Close)
	h := newLogHost(chatConfig(config.ModelEntry{ModelName: "failed-model", LiteLLMParams: map[string]any{
		"model": "openai/failed-model", "api_key": "test-key", "api_base": up.URL + "/v1",
	}}), up.Client())
	rec := httptest.NewRecorder()
	Serve(h, rec, chatRequest(t, "failed-model", false), "chat")
	if rec.Code != http.StatusBadGateway || len(h.spends) != 1 || h.spends[0].status != http.StatusBadGateway {
		t.Fatalf("失败日志状态不完整: http=%d spends=%+v", rec.Code, h.spends)
	}
	if got := h.exchanges[h.spends[0].callID]; !bytes.Equal(got, rawError) {
		t.Fatalf("上游错误正文被改写: got=%q want=%q", got, rawError)
	}
}

// TestServePersistsNetworkFailure 验证普通数据面网络错误也写入 502 日志。
// 前置条件是自定义 RoundTripper 返回固定错误；结果应保存安全错误文本；不访问外网且无需清理数据。
func TestServePersistsNetworkFailure(t *testing.T) {
	client := &http.Client{Transport: roundTripError{err: errors.New("dial tcp: connection refused")}}
	h := newLogHost(chatConfig(config.ModelEntry{ModelName: "network-model", LiteLLMParams: map[string]any{
		"model": "openai/network-model", "api_key": "test-key", "api_base": "https://example.invalid/v1",
	}}), client)
	rec := httptest.NewRecorder()
	Serve(h, rec, chatRequest(t, "network-model", false), "chat")
	if rec.Code != http.StatusBadGateway || len(h.spends) != 1 {
		t.Fatalf("网络失败未形成一条日志: http=%d spends=%+v", rec.Code, h.spends)
	}
	if got := string(h.exchanges[h.spends[0].callID]); !strings.Contains(got, "connection refused") {
		t.Fatalf("网络错误正文缺失: %q", got)
	}
}

// roundTripError 是离线网络失败夹具；参数请求不读取，返回固定错误且无响应。
type roundTripError struct{ err error }

// RoundTrip 返回固定网络错误；参数请求仅用于满足接口，响应始终为空。
func (r roundTripError) RoundTrip(*http.Request) (*http.Response, error) { return nil, r.err }

// TestServeLogsBuildSkipAndTerminalAuth 验证任意供应商按显式协议调用且型号原样传递。
// 参数 t：测试上下文；本地上游检查请求及日志，不依赖外网，服务和日志 writer 自动恢复。
func TestServeLogsBuildSkipAndTerminalAuth(t *testing.T) {
	buf := captureLog(t)
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var doc map[string]any
		_ = json.NewDecoder(r.Body).Decode(&doc)
		if doc["model"] != "base_llm/some-model" {
			t.Errorf("上游型号被改写: %v", doc)
		}
		io.WriteString(w, `{"id":"local","choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`)
	}))
	defer up.Close()
	h := newLogHost(chatConfig(config.ModelEntry{ModelName: "custom", LiteLLMParams: map[string]any{"model": "base_llm/some-model", "api_key": "sk-local-master", "api_base": up.URL}}), up.Client())
	rec := httptest.NewRecorder()
	Serve(h, rec, chatRequest(t, "custom", false), "chat")
	if rec.Code != 200 {
		t.Fatalf("自定义供应商调用失败: %d %s", rec.Code, rec.Body.String())
	}
	assertNoSecrets(t, buf.String())
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
		"debug skip deployment path=/v1/chat/completions provider=custom-test model=openai/gpt-4o-mini reason=missing api key or api base",
		"error dataplane path=/v1/chat/completions status=401 code=authentication_error provider=custom-test",
	} {
		if !strings.Contains(line, want) {
			t.Fatalf("log missing %q\n%s", want, line)
		}
	}
	assertNoSecrets(t, line)
}

// TestServeLogsUnregisteredTransport 验证未注册的显式 transport 在联网前被拒绝。
// 参数 t：测试上下文；供应商名称只是用户数据，不参与能力判断；无外部调用，日志 writer 由 cleanup 恢复。
func TestServeLogsUnregisteredTransport(t *testing.T) {
	buf := captureLog(t)
	cfg := chatConfig(config.ModelEntry{ModelName: "ghost", LiteLLMParams: map[string]any{"model": "openai/gpt", "api_key": "sk-local-master", "api_base": "https://example.invalid"}})
	cfg.ModelList[0].ModelInfo["transport"] = "unregistered_transport"
	rec := httptest.NewRecorder()
	Serve(newLogHost(cfg, nil), rec, chatRequest(t, "ghost", false), "chat")
	if rec.Code != 400 {
		t.Fatalf("未注册 transport 未被拒绝: %d %s", rec.Code, rec.Body.String())
	}
	assertNoSecrets(t, buf.String())
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
			"error upstream stream path=/v1/chat/completions provider=custom-test model=openai/gpt-4o-mini",
			"err=upstream stream ended without completion",
			"error dataplane path=/v1/chat/completions status=502 code=upstream_error detail=upstream stream ended without completion",
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
			"error upstream status path=/v1/chat/completions provider=custom-test model=openai/gpt-4o-mini",
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
	scope, err := json.Marshal(map[string]any{"models": h.cfg.ModelList, "router": h.RouteSettingsFor(nil).Settings, "session": "", "query": ""})
	if err != nil {
		t.Fatal(err)
	}
	h.cache.Set(cache.Key("user:log-user", "chat", "gpt-4o-mini", string(raw), string(scope)), []byte(`{"usage":{"prompt_tokens":11,"completion_tokens":7,"total_tokens":18}}`))

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
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":5,\"completion_tokens\":4,\"total_tokens\":9}}\n\ndata: [DONE]\n\n")
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

func (h *logHost) PinOfficialContext(string, provider.TaskContext) {}
func (h *logHost) OfficialContext(string) provider.TaskContext     { return provider.TaskContext{} }
