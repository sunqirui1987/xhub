package regression

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

// 下面这些用例补上请求链上还没单独走完的分叉。
// 每条都有 simulated 和 live 两个子测试：能打真实供应商的会在 live 里再跑一遍，
// 必须脚本化上游的，live 子测试会跳过并说明原因。

func TestIdempotencyReplaysWithoutASecondCharge(t *testing.T) {
	runBoth(t, func(t *testing.T) { idempotencyChain(t, false) }, func(t *testing.T) { idempotencyChain(t, true) })
}

func idempotencyChain(t *testing.T, live bool) {
	var h *harness
	var c chained
	var model string
	if live {
		var admin string
		h, admin, c, model = openLiveChat(t, "idem-live")
		_ = admin
	} else {
		model = "regression-idem"
		h = newHarness(t, chatDeployment(model))
		admin := h.adminSession()
		c = h.openScope(t, admin, "idem")
	}
	headers := map[string]string{"Idempotency-Key": "same-call"}
	before := h.moneyOf(t, c)
	mark := len(h.upstreamCalls())
	body := chatRequest(model, "idempotent")
	first := h.doHeaders(http.MethodPost, "/v1/chat/completions", c.key, body, headers)
	if first.status != http.StatusOK {
		t.Fatalf("first call: %s", first.describe())
	}
	second := h.doHeaders(http.MethodPost, "/v1/chat/completions", c.key, body, headers)
	if second.status != first.status || second.text() != first.text() {
		t.Fatalf("replay differed:\nfirst %s\nsecond %s", first.describe(), second.describe())
	}
	after := h.moneyOf(t, c)
	if !after.grewBy(before, parseFloatOrZero(first.header("x-litellm-response-cost"))) {
		t.Fatalf("replay charged a second time: %+v -> %+v", before, after)
	}
	if !h.live {
		if got := h.upstreamSince(mark); len(got) != 1 {
			t.Fatalf("replay dialed the upstream %d times, want 1", len(got))
		}
	}
}

func TestStreamSkipsIdempotency(t *testing.T) {
	runSimulated(t, func(t *testing.T) {
		const model = "regression-idem-stream"
		h := newHarness(t, chatDeployment(model))
		admin := h.adminSession()
		c := h.openScope(t, admin, "idem-stream")
		body := chatRequest(model, "stream")
		body["stream"] = true
		headers := map[string]string{"Idempotency-Key": "stream-key"}
		h.doHeaders(http.MethodPost, "/v1/chat/completions", c.key, body, headers)
		h.doHeaders(http.MethodPost, "/v1/chat/completions", c.key, body, headers)
		if got := len(h.upstreamCalls()); got != 2 {
			t.Fatalf("streaming reused an idempotency buffer, upstream calls=%d, want 2", got)
		}
	})
}

func TestMalformedRequestsNeverDial(t *testing.T) {
	runBoth(t, func(t *testing.T) { malformedChain(t, false) }, func(t *testing.T) { malformedChain(t, true) })
}

func malformedChain(t *testing.T, live bool) {
	var h *harness
	var key, model string
	if live {
		var admin string
		var c chained
		h, admin, c, model = openLiveChat(t, "malformed-live")
		_, _ = admin, model
		key = c.key
	} else {
		model = "regression-malformed"
		h = newHarness(t, chatDeployment(model))
		admin := h.adminSession()
		c := h.openScope(t, admin, "malformed")
		key = c.key
	}
	mark := len(h.upstreamCalls())
	raw := h.doRaw(http.MethodPost, "/v1/chat/completions", key, "application/json", "{")
	if raw.status != http.StatusBadRequest {
		t.Fatalf("bad json answered %d: %s", raw.status, raw.text())
	}
	missing := h.do(http.MethodPost, "/v1/chat/completions", key, map[string]any{
		"messages": []any{map[string]any{"role": "user", "content": "no model"}},
	})
	if missing.status != http.StatusBadRequest || !strings.Contains(errorMessage(missing), "model") {
		t.Fatalf("missing model answered %s", missing.describe())
	}
	if !h.live {
		if got := h.upstreamSince(mark); len(got) != 0 {
			t.Fatalf("a malformed request reached the upstream: %v", got)
		}
	}
}

func TestTPMLimitIsNotABudgetError(t *testing.T) {
	runBoth(t, func(t *testing.T) { tpmChain(t, false) }, func(t *testing.T) { tpmChain(t, true) })
}

func tpmChain(t *testing.T, live bool) {
	var h *harness
	var admin string
	var c chained
	var model string
	if live {
		h, admin, c, model = openLiveChat(t, "tpm-live")
	} else {
		model = "regression-tpm"
		h = newHarness(t, chatDeployment(model))
		admin = h.adminSession()
		c = h.openScope(t, admin, "tpm")
	}
	// 第二次估算 99 token；加上第一次实际 16 / Redis 估算 33，均超过 100。
	h.ok(http.MethodPost, "/key/update", admin, map[string]any{"key": c.key, "tpm_limit": 100})
	mark := len(h.upstreamCalls())
	first := h.do(http.MethodPost, "/v1/chat/completions", c.key, chatRequest(model, "tpm one"))
	if first.status != http.StatusOK {
		t.Fatalf("first call under the token window: %s", first.describe())
	}
	second := h.do(http.MethodPost, "/v1/chat/completions", c.key, chatRequest(model, strings.Repeat("x", 268)))
	if second.status != http.StatusTooManyRequests || !strings.Contains(errorMessage(second), "tpm_limit") {
		t.Fatalf("tpm refusal: %s", second.describe())
	}
	if strings.Contains(errorMessage(second), "budget") {
		t.Fatalf("a token limit was reported as a budget problem: %s", errorMessage(second))
	}
	if !h.live && len(h.upstreamSince(mark)) != 1 {
		t.Fatalf("the refused call was dialed: %v", h.upstreamSince(mark))
	}
}

func TestUpstream429FailsOverAndAMissingKeyIsSkipped(t *testing.T) {
	runSimulated(t, func(t *testing.T) {
		const public = "regression-429"
		h := newHarness(t,
			deployment(public, "openai/rate-limited", map[string]any{"weight": 10}),
			deployment(public, "openai/spare", map[string]any{"weight": 1}),
		)
		admin := h.adminSession()
		c := h.openScope(t, admin, "failover-429")
		h.scriptStatus("rate-limited", http.StatusTooManyRequests)
		mark := len(h.upstreamCalls())
		ok := h.ok(http.MethodPost, "/v1/chat/completions", c.key, chatRequest(public, "429 then spare"))
		if got := h.upstreamSince(mark); strings.Join(got, ",") != "rate-limited,spare" {
			t.Fatalf("429 failover reached %v", got)
		}
		if parseFloatOrZero(ok.header("x-litellm-response-cost")) <= 0 {
			t.Fatalf("the spare deployment was not billed: %s", ok.describe())
		}

		h.clearAPIKey(public)
		h.scriptStatus("rate-limited", 0)
		h.resetUpstream()
		skipped := h.ok(http.MethodPost, "/v1/chat/completions", c.key, chatRequest(public, "skip the keyless deployment"))
		if got := h.upstreamSince(0); strings.Join(got, ",") != "spare" {
			t.Fatalf("a deployment without a key was dialed: %v", got)
		}
		if parseFloatOrZero(skipped.header("x-litellm-response-cost")) <= 0 {
			t.Fatalf("the remaining deployment was not billed: %s", skipped.describe())
		}
	})
}

func TestGuardrailDoesNotCoverEmbeddings(t *testing.T) {
	runSimulated(t, func(t *testing.T) {
		const model = "regression-embed-guard"
		h := newHarness(t, embeddingDeployment(model))
		admin := h.adminSession()
		c := h.openScope(t, admin, "embed-guard")
		h.putGuardrail(t, "embed-block", guardrailRow("embed-block", kindBlockedWords, []string{"launch-codes"}, ""))
		r := h.ok(http.MethodPost, "/v1/embeddings", c.key, map[string]any{"model": model, "input": "launch-codes"})
		if got := stringField(r.json(), "object"); got == "" {
			t.Fatalf("embedding was blocked by a chat guardrail: %s", r.describe())
		}
		if calls := h.upstreamCalls(); len(calls) != 1 || !strings.Contains(calls[0].Path, "embedding") {
			t.Fatalf("embedding upstream %v", calls)
		}
	})
}

func TestEachInferenceFamilyReachesItsOwnUpstreamPath(t *testing.T) {
	runSimulated(t, func(t *testing.T) {
		const model = "regression-family"
		entry := chatDeployment(model)
		entry.ModelInfo["endpoint_types"] = []any{
			"chat", "completion", "embedding", "image", "rerank", "moderation", "audio_speech",
		}
		h := newHarness(t, entry)
		admin := h.adminSession()
		c := h.openScope(t, admin, "family")
		cases := []struct {
			path, fragment string
			body           map[string]any
		}{
			{"/v1/completions", "/completions", map[string]any{"model": model, "prompt": "hi"}},
			{"/v1/images/generations", "/images/generations", map[string]any{"model": model, "prompt": "a cat"}},
			{"/v1/moderations", "/moderations", map[string]any{"model": model, "input": "hi"}},
			{"/v1/rerank", "/rerank", map[string]any{"model": model, "query": "q", "documents": []any{"a", "b"}}},
			{"/v1/audio/speech", "/audio/speech", map[string]any{"model": model, "input": "hi", "voice": "alloy"}},
		}
		for _, tc := range cases {
			h.resetUpstream()
			r := h.do(http.MethodPost, tc.path, c.key, tc.body)
			if r.status >= 500 {
				t.Fatalf("%s -> %d: %s", tc.path, r.status, r.text())
			}
			calls := h.upstreamCalls()
			if len(calls) != 1 || !strings.Contains(calls[0].Path, tc.fragment) {
				t.Fatalf("%s reached %v, want a path containing %s; status %d body %s",
					tc.path, calls, tc.fragment, r.status, r.text())
			}
		}
	})
}

func (h *harness) doRaw(method, path, token, contentType, body string) reply {
	h.t.Helper()
	req, err := http.NewRequest(method, h.server.URL+path, strings.NewReader(body))
	if err != nil {
		h.t.Fatalf("build raw request: %v", err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := h.server.Client().Do(req)
	if err != nil {
		h.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return reply{status: resp.StatusCode, body: raw, headers: resp.Header.Clone()}
}
