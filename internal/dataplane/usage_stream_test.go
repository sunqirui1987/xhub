package dataplane

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"
)

func TestCompleteUsagePreservesReportedZeroAndCacheInput(t *testing.T) {
	usage := map[string]any{
		"input_tokens":                float64(0),
		"output_tokens":               float64(0),
		"cache_read_input_tokens":     float64(19),
		"cache_creation_input_tokens": float64(3),
	}
	got := completeUsage(usage, map[string]any{"messages": []any{map[string]any{"content": "estimate me"}}}, []byte("data: {\"choices\":[{\"delta\":{\"content\":\"text\"}}]}\n"))
	if !reflect.DeepEqual(got, usage) {
		t.Fatalf("explicit usage changed: got %#v want %#v", got, usage)
	}
}

func TestEstimateTokensDoesNotCountOutputLimitAsInput(t *testing.T) {
	without := EstimateTokens(map[string]any{"prompt": "abcd"})
	with := EstimateTokens(map[string]any{"prompt": "abcd", "max_tokens": 100000})
	if with != without {
		t.Fatalf("max_tokens changed input estimate: with=%d without=%d", with, without)
	}
}

func TestStreamUsageMergesAnthropicFramesAndNestedProviders(t *testing.T) {
	var usage map[string]any
	usage = streamUsage([]byte("data: {\"usage\":{\"input_tokens\":12,\"cache_read_input_tokens\":7}}\n"), usage)
	usage = streamUsage([]byte("data: {\"usage\":{\"output_tokens\":5}}\n"), usage)
	if pt, ct := usageCounts(usage); pt != 12 || ct != 5 || asInt(usage["cache_read_input_tokens"]) != 7 {
		t.Fatalf("merged Anthropic usage: %#v", usage)
	}

	responses := streamUsage([]byte("data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":9,\"output_tokens\":4}}}\n"), nil)
	if pt, ct := usageCounts(responses); pt != 9 || ct != 4 {
		t.Fatalf("nested Responses usage: %#v", responses)
	}

	gemini := streamUsage([]byte("data: {\"usageMetadata\":{\"promptTokenCount\":20,\"candidatesTokenCount\":6,\"thoughtsTokenCount\":2,\"cachedContentTokenCount\":11}}\n"), nil)
	if pt, ct := usageCounts(gemini); pt != 20 || ct != 8 {
		t.Fatalf("Gemini usage: %#v", gemini)
	}
	details := gemini["prompt_tokens_details"].(map[string]any)
	if details["cached_tokens"] != 11 {
		t.Fatalf("Gemini cached input: %#v", gemini)
	}
}

func TestPipeStreamParsesFragmentedSSEAndReturnsBodyError(t *testing.T) {
	readErr := errors.New("upstream reset")
	body := &fragmentedErrorReader{parts: [][]byte{
		[]byte("da"),
		[]byte("ta: {\"usageMeta"),
		[]byte("data\":{\"promptTokenCount\":13,\"candidatesTokenCount\":2,\"cachedContentTokenCount\":8}}\r"),
		[]byte("\n\r\n"),
	}, err: readErr}
	resp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(body), Header: make(http.Header)}
	w := httptest.NewRecorder()
	wrote, usage, _, captured, gotErr := pipeStream(w, resp, time.Now())
	if !wrote || !errors.Is(gotErr, readErr) {
		t.Fatalf("wrote=%v err=%v", wrote, gotErr)
	}
	if pt, ct := usageCounts(usage); pt != 13 || ct != 2 {
		t.Fatalf("fragmented usage: %#v", usage)
	}
	if len(captured) == 0 || captured[len(captured)-1] != '\n' {
		t.Fatalf("captured stream: %q", captured)
	}
}

type fragmentedErrorReader struct {
	parts [][]byte
	err   error
}

func (r *fragmentedErrorReader) Read(p []byte) (int, error) {
	if len(r.parts) == 0 {
		return 0, r.err
	}
	part := r.parts[0]
	r.parts = r.parts[1:]
	return copy(p, part), nil
}
