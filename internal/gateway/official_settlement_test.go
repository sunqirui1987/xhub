package gateway

import (
	"fmt"
	"net"
	"net/http/httptest"
	"os/exec"
	"sync"
	"testing"
	"time"

	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/dataplane"
	"github.com/sunqirui1987/xhub/internal/live"
)

func settlementRedis(t *testing.T) *live.Client {
	t.Helper()
	binary, err := exec.LookPath("redis-server")
	if err != nil {
		t.Skip("redis-server is required")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	_, port, _ := net.SplitHostPort(addr)
	_ = listener.Close()
	cmd := exec.Command(binary, "--bind", "127.0.0.1", "--port", port, "--save", "", "--appendonly", "no")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	for deadline := time.Now().Add(5 * time.Second); ; {
		c, err := live.Open("redis://" + addr)
		if err == nil {
			t.Cleanup(func() { _ = c.Close() })
			return c
		}
		if time.Now().After(deadline) {
			t.Fatal(err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestOfficialSettlementSkipsRouteTPMAndConsumesOwnMetadata(t *testing.T) {
	c := settlementRedis(t)
	s := &Server{Cfg: &config.Config{}, Live: c, exchanges: map[string]promptExchange{}}
	const dep = "official-deployment"
	const settlement = "official-settlement:shared"
	const polls = 16
	// All temporary metadata coexists before any recordSpend consumes it.
	for i := 0; i < polls; i++ {
		id := fmt.Sprintf("poll-%d", i)
		s.exchanges[id] = promptExchange{messages: id, response: id, proxy: id}
		s.AnnotateCall(id, dataplane.CallNote{SettlementID: settlement, SkipRouteUsage: true, Provider: id})
	}
	var wg sync.WaitGroup
	for i := 0; i < polls; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			s.recordSpend(httptest.NewRecorder(), nil, fmt.Sprintf("poll-%d", i), "unknown", "official:query", map[string]any{"completion_tokens": 12}, time.Now(), false, 200, dep)
		}(i)
	}
	wg.Wait()
	if got := c.Usages([]string{dep})[dep]; got != 0 {
		t.Fatalf("official polls increased route TPM: %v", got)
	}
	rows, _ := c.PeekLogs(100)
	if len(rows) != 1 || rows[0].RequestID != settlement {
		t.Fatalf("duplicate settlement rows: %+v", rows)
	}
	row := rows[0]
	if row.Messages == "" || row.Messages != row.Response || row.Messages != row.ProxyRequest || row.Messages != row.Provider {
		t.Fatalf("mixed exchange metadata: %+v", row)
	}
	if len(s.exchanges) != 0 || len(s.callNotes) != 0 {
		t.Fatal("temporary metadata leaked")
	}
	// The same path still measures ordinary synchronous generation.
	s.recordSpend(httptest.NewRecorder(), nil, "sync-generation", "unknown", "chat", map[string]any{"completion_tokens": 12}, time.Now(), false, 200, dep)
	if got := c.Usages([]string{dep})[dep]; got != 12 {
		t.Fatalf("synchronous route TPM = %v, want 12", got)
	}
}
