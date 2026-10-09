package regression

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/sunqirui1987/xhub/internal/config"
)

// 下面这些用例补上请求链上还没单独走完的分叉。
// 可调用真实供应商的场景提供 live 子测试；脚本故障仅走本地模拟边界。

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
		first := h.doHeaders(http.MethodPost, "/v1/chat/completions", c.key, body, headers)
		second := h.doHeaders(http.MethodPost, "/v1/chat/completions", c.key, body, headers)
		for _, r := range []reply{first, second} {
			if r.status != http.StatusOK || !strings.Contains(r.text(), "data:") {
				t.Fatalf("stream with idempotency key failed: %s", r.describe())
			}
		}
		if got := len(h.upstreamCalls()); got != 2 {
			t.Fatalf("streaming reused an idempotency buffer, upstream calls=%d, want 2", got)
		}
		if first.header("x-litellm-call-id") == "" || first.header("x-litellm-call-id") == second.header("x-litellm-call-id") {
			t.Fatalf("independent streams need distinct call ids: %q, %q", first.header("x-litellm-call-id"), second.header("x-litellm-call-id"))
		}
		h.flushSpend()
		for _, r := range []reply{first, second} {
			row := logDetail(t, h, admin, r.header("x-litellm-call-id"))
			if numberOrZero(row["spend"]) <= 0 {
				t.Fatalf("stream call was not billed: %s", truncate(string(mustJSON(row)), 300))
			}
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

// TestUpstream429FailsOverAndAMissingKeyIsSkipped 验证 429 会切换部署，且缺少密钥的部署不会被拨号。
// 前置为同一公开名下的本地双部署；分别检查调用顺序和成功部署计费，隔离 schema 与 harness 负责清理。
// 参数 t（*testing.T）：当前回归测试；返回：无。
func TestUpstream429FailsOverAndAMissingKeyIsSkipped(t *testing.T) {
	runSimulated(t, func(t *testing.T) {
		const public = "regression-429"
		h := newHarness(t,
			deployment(public, "rate-limited", map[string]any{"input_cost_per_token": testInputRate / 2}),
			deployment(public, "spare", nil),
		)
		admin := h.adminSession()
		c := h.openScope(t, admin, "failover-429")
		// 成本排序保证先触发 429；显式关闭冷却，使后续缺密钥场景独立。
		h.modelRouteTemplate(t, admin, c, "429-template", public, "cost-based-routing", nil, 1, 0, 0)
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

// TestEachInferenceFamilyReachesItsOwnUpstreamPath 验证各推理族端点使用各自的上游路径。
// 前置为声明全部端点能力的本地部署；逐项检查 completions、图片、审核、重排和语音路径，harness 关闭上游并清理隔离 schema。
// 参数 t（*testing.T）：当前回归测试；返回：无。
func TestEachInferenceFamilyReachesItsOwnUpstreamPath(t *testing.T) {
	runSimulated(t, func(t *testing.T) {
		const model = "regression-family"
		// 原生入口按 transport 选择部署，同一个别名为各族显式登记独立执行协议。
		var entries []config.ModelEntry
		for _, binding := range []struct{ transport, endpoint string }{
			{"openai_completions", "completion"},
			{"openai_image_generation", "image_generation"},
			{"openai_moderations", "moderation"},
			{"rerank", "rerank"},
			{"openai_audio_speech", "audio_speech"},
		} {
			entry := chatDeployment(model)
			entry.ModelInfo["transport"] = binding.transport
			entry.ModelInfo["endpoint_types"] = []string{binding.endpoint}
			entries = append(entries, entry)
		}
		h := newHarness(t, entries...)
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
			if r.status < 200 || r.status >= 300 {
				t.Fatalf("%s should succeed, got %d: %s", tc.path, r.status, r.text())
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
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		h.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		h.t.Fatalf("read raw response %s %s: %v", method, path, err)
	}
	return reply{status: resp.StatusCode, body: raw, headers: resp.Header.Clone()}
}
