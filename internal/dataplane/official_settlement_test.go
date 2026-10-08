package dataplane

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sunqirui1987/xhub/internal/auth"
	"github.com/sunqirui1987/xhub/internal/provider"
	"github.com/sunqirui1987/xhub/internal/router"
)

type concurrentPollRecord struct {
	callID, poll, response string
	note                   CallNote
	usage                  map[string]any
}

type concurrentSettlementHost struct {
	*logicHost
	mu          sync.Mutex
	exchanges   map[string]concurrentPollRecord
	annotations map[string]CallNote
	records     []concurrentPollRecord
	ready       sync.WaitGroup
}

func (h *concurrentSettlementHost) RememberExchange(id string, r *http.Request, _, response []byte) {
	h.mu.Lock()
	h.exchanges[id] = concurrentPollRecord{callID: id, poll: r.Header.Get("X-Poll"), response: string(response)}
	h.mu.Unlock()
	// Force every exchange to coexist before any poll consumes its metadata.
	h.ready.Done()
	h.ready.Wait()
}

func (h *concurrentSettlementHost) AnnotateCall(id string, note CallNote) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.annotations[id] = note
}

func (h *concurrentSettlementHost) RecordSpend(_ http.ResponseWriter, _ *auth.Principal, id, _, _ string, usage map[string]any, _ time.Time, _ bool, _ int, _ string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	row := h.exchanges[id]
	row.note, row.usage = h.annotations[id], usage
	h.records = append(h.records, row)
	delete(h.exchanges, id)
	delete(h.annotations, id)
}

func TestOfficialConcurrentCompletedPollsIsolateMetadata(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "succeeded", "poll": r.Header.Get("X-Poll"), "usage": map[string]any{"completion_tokens": 12}})
	}))
	defer up.Close()
	dep := deployment("video", "volcengine/model", "key", up.URL, "ark_contents_generation", nil)
	h := &concurrentSettlementHost{logicHost: officialHost(up, dep), exchanges: map[string]concurrentPollRecord{}, annotations: map[string]CallNote{}}
	h.pins[officialTaskScope(&auth.Principal{UserID: "test-user"}, "ark_contents_generation", "task")] = router.CooldownID(dep)
	const polls = 16
	h.ready.Add(polls)
	var wg sync.WaitGroup
	for i := 0; i < polls; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			r := httptest.NewRequest(http.MethodGet, "/api/v3/contents/generations/tasks/task", nil)
			r.Header.Set("X-Poll", fmt.Sprint(i))
			hit, ok := provider.Match(r.Method, r.URL.Path, h.models)
			if !ok {
				t.Error("missing official route")
				return
			}
			w := httptest.NewRecorder()
			ServeBypass(h, w, r, hit)
			if w.Code != http.StatusOK {
				t.Errorf("poll %d: %d %s", i, w.Code, w.Body.String())
			}
		}(i)
	}
	wg.Wait()
	if len(h.records) != polls {
		t.Fatalf("recorded %d polls", len(h.records))
	}
	seen := map[string]bool{}
	settlement := h.records[0].note.SettlementID
	for _, row := range h.records {
		var response struct {
			Poll string `json:"poll"`
		}
		if err := json.Unmarshal([]byte(row.response), &response); err != nil {
			t.Fatal(err)
		}
		if row.callID == "" || seen[row.callID] || response.Poll != row.poll || settlement == "" || row.note.SettlementID != settlement || !row.note.SkipRouteUsage || !positiveOfficialUsage(row.usage) {
			t.Fatalf("metadata collision or lost settlement: %+v response=%+v", row, response)
		}
		seen[row.callID] = true
	}
	if len(h.exchanges) != 0 || len(h.annotations) != 0 {
		t.Fatal("metadata was not consumed")
	}
}

// Model a failed first persistence attempt: a later poll must submit the same
// positive event again, even if a legacy host has a billed marker already.
type retrySettlementHost struct {
	*logicHost
	attempts []string
}

func (h *retrySettlementHost) RecordSpend(w http.ResponseWriter, p *auth.Principal, id, alias, op string, usage map[string]any, start time.Time, cached bool, status int, dep string) {
	h.attempts = append(h.attempts, h.notes[len(h.notes)-1].SettlementID)
	if len(h.attempts) == 1 {
		return
	}
	h.logicHost.RecordSpend(w, p, id, alias, op, usage, start, cached, status, dep)
}

func TestOfficialSettlementRetriesAfterPersistenceFailure(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"status":"succeeded","usage":{"completion_tokens":12}}`)
	}))
	defer up.Close()
	dep := deployment("video", "volcengine/model", "key", up.URL, "ark_contents_generation", nil)
	h := &retrySettlementHost{logicHost: officialHost(up, dep)}
	scope := officialTaskScope(&auth.Principal{UserID: "test-user"}, "ark_contents_generation", "task")
	h.pins[scope] = router.CooldownID(dep)
	h.billed[scope] = true // Old markers cannot veto durable retries.
	for i := 0; i < 2; i++ {
		r := httptest.NewRequest(http.MethodGet, "/api/v3/contents/generations/tasks/task", nil)
		hit, ok := provider.Match(r.Method, r.URL.Path, h.models)
		if !ok {
			t.Fatal("missing official route")
		}
		w := httptest.NewRecorder()
		ServeBypass(h, w, r, hit)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	if len(h.attempts) != 2 || h.attempts[0] != h.attempts[1] || !strings.HasPrefix(h.attempts[0], "official-settlement:") || len(h.spend) != 1 || h.spend[0].usage == nil {
		t.Fatalf("lost settlement retry: attempts=%v persisted=%+v", h.attempts, h.spend)
	}
	if h.attempts[0] == officialSettlementID(scope, "other-deployment") || h.attempts[0] == officialSettlementID(officialTaskScope(&auth.Principal{UserID: "other"}, "ark_contents_generation", "task"), router.CooldownID(dep)) {
		t.Fatal("settlement identity lacks caller/deployment isolation")
	}
}

func TestOfficialZeroAndPendingPollsKeepUniqueIDs(t *testing.T) {
	response := `{"status":"succeeded","usage":{"completion_tokens":0}}`
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, response) }))
	defer up.Close()
	dep := deployment("video", "volcengine/model", "key", up.URL, "ark_contents_generation", nil)
	h := officialHost(up, dep)
	h.pins[officialTaskScope(&auth.Principal{UserID: "test-user"}, "ark_contents_generation", "task")] = router.CooldownID(dep)
	for _, body := range []string{response, response, `{"status":"running","usage":{"completion_tokens":12}}`, `{"status":"failed","usage":{"completion_tokens":12}}`} {
		response = body
		h.call(t, http.MethodGet, "/api/v3/contents/generations/tasks/task", "")
	}
	seen := map[string]bool{}
	for _, note := range h.notes {
		if note.SettlementID != "" || !note.SkipRouteUsage {
			t.Fatalf("invalid poll note: %+v", note)
		}
	}
	for _, row := range h.spend {
		if row.usage != nil || strings.HasPrefix(row.callID, "official-settlement:") || seen[row.callID] {
			t.Fatalf("poll collapsed or charged: %+v", row)
		}
		seen[row.callID] = true
	}
	if len(h.billed) != 0 {
		t.Fatal("pre-persistence marker written")
	}
}
