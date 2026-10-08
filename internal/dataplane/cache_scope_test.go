package dataplane

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sunqirui1987/xhub/internal/auth"
	"github.com/sunqirui1987/xhub/internal/cache"
	"github.com/sunqirui1987/xhub/internal/config"
)

type cacheScopeHost struct {
	*logHost
	session string
	hits    int
}

func (h *cacheScopeHost) PlanRoute(*http.Request, string, map[string]any, *auth.Principal) RoutePlan {
	return RoutePlan{SessionID: h.session}
}
func (h *cacheScopeHost) WriteCacheHit(http.ResponseWriter, *auth.Principal, string, string, string, string, []byte, time.Time) {
	h.hits++
}

func TestCacheScopeChangesWithConfigurationSessionAndQuery(t *testing.T) {
	for _, change := range []string{"unchanged", "disabled", "price", "session", "query"} {
		t.Run(change, func(t *testing.T) {
			attempts := 0
			client := &http.Client{Transport: auditRoundTripper(func(*http.Request) (*http.Response, error) {
				attempts++
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"usage":{"prompt_tokens":1,"completion_tokens":1}}`))}, nil
			})}
			cfg := chatConfig(config.ModelEntry{ModelName: "audit", LiteLLMParams: map[string]any{"model": "openai/audit", "api_base": "https://example.invalid/v1", "api_key": "test-key"}, ModelInfo: map[string]any{}})
			h := &cacheScopeHost{logHost: newLogHost(cfg, client), session: "session-a"}
			request := chatRequest(t, "audit", false)
			raw, _ := io.ReadAll(request.Body)
			var body map[string]any
			_ = json.Unmarshal(raw, &body)
			raw, _ = json.Marshal(body)
			scope, _ := json.Marshal(map[string]any{"models": cfg.ModelList, "router": h.RouteSettingsFor(nil).Settings, "session": h.session, "query": ""})
			h.cache.Set(cache.Key("user:log-user", "chat", "audit", string(raw), string(scope)), []byte(`{"usage":{"prompt_tokens":1,"completion_tokens":1}}`))
			request.Body = io.NopCloser(strings.NewReader(string(raw)))
			switch change {
			case "disabled":
				cfg.ModelList[0].ModelInfo["disabled"] = true
			case "price":
				cfg.ModelList[0].ModelInfo["input_cost_per_token"] = 0.25
			case "session":
				h.session = "session-b"
			case "query":
				request.URL.RawQuery = "version=2"
			}
			rec := httptest.NewRecorder()
			Serve(h, rec, request, "chat")
			if change == "unchanged" {
				if h.hits != 1 || attempts != 0 {
					t.Fatalf("hits=%d attempts=%d", h.hits, attempts)
				}
			} else if h.hits != 0 {
				t.Fatalf("stale cache hit after %s", change)
			} else if change == "disabled" {
				if attempts != 0 || rec.Code != 400 {
					t.Fatalf("status=%d attempts=%d", rec.Code, attempts)
				}
			} else if attempts != 1 {
				t.Fatalf("attempts=%d", attempts)
			}
		})
	}
}
