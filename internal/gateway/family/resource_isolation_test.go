package family

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sunqirui1987/xhub/internal/auth"
	"github.com/sunqirui1987/xhub/internal/store"
	"github.com/sunqirui1987/xhub/cmd/regression/testsupport"
)

type resourceHost struct {
	Host
	st   *store.Store
	user string
}

func (h *resourceHost) RecordStore() *store.Store { return h.st }
func (h *resourceHost) RequireLLM(http.ResponseWriter, *http.Request) *auth.Principal {
	return &auth.Principal{UserID: h.user}
}

func resourceCall(h *resourceHost, method, path, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	ServeDataPlane(h, w, httptest.NewRequest(method, path, bytes.NewBufferString(body)))
	return w
}

func TestInferenceResourcesAreIsolatedByCaller(t *testing.T) {
	st, err := store.Open(testsupport.Postgres(t, "resource_isolation"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Engine.Close() })
	h := &resourceHost{st: st, user: "alice"}
	created := resourceCall(h, "POST", "/v1/assistants", `{"id":"shared-id","name":"private"}`)
	if created.Code != 200 {
		t.Fatalf("create: %d %s", created.Code, created.Body.String())
	}
	h.user = "bob"
	for _, method := range []string{"GET", "DELETE", "POST"} {
		w := resourceCall(h, method, "/v1/assistants/shared-id", `{"name":"changed"}`)
		if w.Code != 404 {
			t.Fatalf("%s foreign resource: %d %s", method, w.Code, w.Body.String())
		}
	}
	w := resourceCall(h, "GET", "/v1/assistants", "")
	var doc struct {
		Data []any `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &doc)
	if w.Code != 200 || len(doc.Data) != 0 {
		t.Fatalf("foreign list: %s", w.Body.String())
	}
	h.user = "alice"
	w = resourceCall(h, "GET", "/v1/assistants/shared-id", "")
	if w.Code != 200 || !bytes.Contains(w.Body.Bytes(), []byte("private")) {
		t.Fatalf("owner read: %s", w.Body.String())
	}
}

func TestUnsupportedInferenceDoesNotFabricateUsage(t *testing.T) {
	w := httptest.NewRecorder()
	writeInferenceNative(w, "count_tokens", map[string]any{"model": "test"})
	if w.Code != 501 || bytes.Contains(w.Body.Bytes(), []byte("usage")) {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	h := &resourceHost{user: "alice"}
	for _, path := range []string{"/v1/batches", "/v1/threads/thread-1/runs", "/v1/search"} {
		w := resourceCall(h, "POST", path, `{}`)
		if w.Code != 501 {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
	}
}

func TestInjectModelPreservesInvalidPayloadForValidation(t *testing.T) {
	for _, raw := range []string{`{bad`, `null`, `[]`} {
		if got := string(injectModel("/v1/models/test:generateContent", []byte(raw))); got != raw {
			t.Fatalf("%q became %q", raw, got)
		}
	}
}
