package dataplane

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sunqirui1987/xhub/internal/auth"
	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/gateway/prefs"
)

type auditRoundTripper func(*http.Request) (*http.Response, error)

func (f auditRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type streamAuditHost struct {
	*logHost
	status   int
	commits  int
	settings prefs.RouteSettings
}

func (h *streamAuditHost) RouteSettingsFor(*auth.Principal) prefs.RouteSettings { return h.settings }
func (h *streamAuditHost) RecordSpend(_ http.ResponseWriter, _ *auth.Principal, _, _, _ string, _ map[string]any, _ time.Time, _ bool, status int, _ string) {
	h.status = status
}
func (h *streamAuditHost) CommitRoute(RoutePlan, string, string) { h.commits++ }

func TestStreamFailureAfterOutputDoesNotRetryOrPin(t *testing.T) {
	attempts := 0
	client := &http.Client{Transport: auditRoundTripper(func(*http.Request) (*http.Response, error) {
		attempts++
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(&fragmentedErrorReader{parts: [][]byte{[]byte("data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\n")}, err: errors.New("reset")})}, nil
	})}
	cfg := chatConfig(config.ModelEntry{ModelName: "audit", LiteLLMParams: map[string]any{"model": "openai/audit", "api_base": "https://example.invalid/v1", "api_key": "test-key"}})
	h := &streamAuditHost{logHost: newLogHost(cfg, client), settings: prefs.PlatformSettings(map[string]any{"num_retries": 3})}
	rec := httptest.NewRecorder()
	Serve(h, rec, chatRequest(t, "audit", true), "chat")
	if attempts != 1 || h.status != 502 || h.commits != 0 || rec.Body.Len() == 0 {
		t.Fatalf("attempts=%d spendStatus=%d commits=%d body=%q", attempts, h.status, h.commits, rec.Body.String())
	}
}

func TestTemplateLookupFailureDoesNotContactUpstream(t *testing.T) {
	attempts := 0
	client := &http.Client{Transport: auditRoundTripper(func(*http.Request) (*http.Response, error) { attempts++; return nil, errors.New("unexpected request") })}
	h := &streamAuditHost{logHost: newLogHost(chatConfig(), client), settings: prefs.RouteSettings{Err: errors.New("database unavailable")}}
	rec := httptest.NewRecorder()
	Serve(h, rec, chatRequest(t, "audit", false), "chat")
	if rec.Code != 503 || attempts != 0 {
		t.Fatalf("status=%d attempts=%d body=%s", rec.Code, attempts, rec.Body.String())
	}
}
