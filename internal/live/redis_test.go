package live

import (
	"context"
	"math"
	"net"
	"os/exec"
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func testRedis(t *testing.T) *Client {
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
	rdb := redis.NewClient(&redis.Options{Addr: addr, MaxRetries: -1, DialTimeout: time.Second})
	t.Cleanup(func() { _ = rdb.Close() })
	for deadline := time.Now().Add(5 * time.Second); ; {
		if err := rdb.Ping(context.Background()).Err(); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("redis did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	return &Client{rdb: rdb}
}

func TestEnqueueSpendConcurrentDedupAndAck(t *testing.T) {
	c := testRedis(t)
	row := SpendLog{RequestID: "tenant/task/settlement", APIKey: "key", OrgID: "org", Spend: 1.25, SpendValid: true}
	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := c.EnqueueSpend(row); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	rows, raw := c.PeekLogs(100)
	if len(rows) != 1 {
		t.Fatalf("queued %d duplicates", len(rows))
	}
	if got := c.HotSpend("key/key"); got != 1.25 {
		t.Fatalf("hot spend %v", got)
	}
	row.RequestID = "tenant/task/other"
	if err := c.EnqueueSpend(row); err != nil {
		t.Fatal(err)
	}
	deltas := map[string]float64{"key/key": 1.25, "org/org": 1.25}
	if err := c.AckFlushed(deltas, 1, raw[0]); err != nil {
		t.Fatal(err)
	}
	if err := c.AckFlushed(deltas, 1, raw[0]); err != nil {
		t.Fatal(err)
	}
	if got := c.HotSpend("key/key"); got != 1.25 {
		t.Fatalf("ack lost concurrent spend: %v", got)
	}
	row.RequestID = "tenant/task/settlement"
	if err := c.EnqueueSpend(row); err != nil {
		t.Fatal(err)
	}
	rows, _ = c.PeekLogs(100)
	if len(rows) != 1 {
		t.Fatalf("replay after ack queued again: %d", len(rows))
	}
	if ttl := c.rdb.TTL(context.Background(), "xhub:spend:requests").Val(); ttl != -1 {
		t.Fatalf("dedup unexpectedly expires: %v", ttl)
	}
}

func TestEnqueueSpendRequiresIdentity(t *testing.T) {
	c := testRedis(t)
	if err := c.EnqueueSpend(SpendLog{Spend: 1, SpendValid: true}); err == nil {
		t.Fatal("accepted empty identity")
	}
}

func TestEnqueueSpendRejectsInvalidStateWithoutPartialWrites(t *testing.T) {
	c := testRedis(t)
	ctx := context.Background()
	row := SpendLog{RequestID: "settlement", APIKey: "key", OrgID: "org", Spend: 2, SpendValid: true}
	if err := c.rdb.Set(ctx, "xhub:spend:org/org", "broken", 0).Err(); err != nil {
		t.Fatal(err)
	}
	if err := c.EnqueueSpend(row); err == nil {
		t.Fatal("accepted corrupt counter")
	}
	if got := c.rdb.Exists(ctx, "xhub:spend:key/key", "xhub:spendlog", "xhub:spend:requests").Val(); got != 0 {
		t.Fatalf("partial writes: %d", got)
	}
	if err := c.rdb.Del(ctx, "xhub:spend:org/org").Err(); err != nil {
		t.Fatal(err)
	}
	for _, cost := range []float64{-1, math.NaN(), math.Inf(1)} {
		row.Spend = cost
		if err := c.EnqueueSpend(row); err == nil {
			t.Fatalf("accepted %v", cost)
		}
	}
	row.Spend = 2
	if err := c.EnqueueSpend(row); err != nil {
		t.Fatal(err)
	}
	row.Spend = 99
	if err := c.EnqueueSpend(row); err != nil {
		t.Fatal(err)
	}
	rows, raw := c.PeekLogs(10)
	if len(rows) != 1 || rows[0].Spend != 2 {
		t.Fatalf("first payload did not win: %+v", rows)
	}
	if err := c.rdb.Set(ctx, "xhub:spend:org/org", "broken", 0).Err(); err != nil {
		t.Fatal(err)
	}
	if err := c.AckFlushed(map[string]float64{"key/key": 2, "org/org": 2}, 1, raw[0]); err == nil {
		t.Fatal("ack accepted corrupt counter")
	}
	if c.HotSpend("key/key") != 2 || c.rdb.LLen(ctx, "xhub:spendlog").Val() != 1 {
		t.Fatal("ack partially mutated queue or spend")
	}
}

func TestPeekLogsStopsAtCorruptEvent(t *testing.T) {
	c := testRedis(t)
	if err := c.EnqueueSpend(SpendLog{RequestID: "first"}); err != nil {
		t.Fatal(err)
	}
	if err := c.rdb.RPush(context.Background(), "xhub:spendlog", "{broken").Err(); err != nil {
		t.Fatal(err)
	}
	if err := c.EnqueueSpend(SpendLog{RequestID: "last"}); err != nil {
		t.Fatal(err)
	}
	rows, raw := c.PeekLogs(10)
	if len(rows) != 1 || len(raw) != 1 {
		t.Fatalf("invalid prefix: %d/%d", len(rows), len(raw))
	}
	if err := c.AckFlushed(nil, 1, raw[0]); err != nil {
		t.Fatal(err)
	}
	rows, raw = c.PeekLogs(10)
	if len(rows) != 0 || len(raw) != 0 || c.rdb.LLen(context.Background(), "xhub:spendlog").Val() != 2 {
		t.Fatal("corrupt event was skipped")
	}
}

func TestConcurrentTokenCounterHasExpiry(t *testing.T) {
	c := testRedis(t)
	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := c.bump("test:tpm", 3); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	keys := c.rdb.Keys(context.Background(), "test:tpm*").Val()
	if len(keys) != 1 {
		t.Fatalf("keys: %v", keys)
	}
	if got := c.rdb.Get(context.Background(), keys[0]).Val(); got != "120" {
		t.Fatalf("lost increments: %s", got)
	}
	if ttl := c.rdb.PTTL(context.Background(), keys[0]).Val(); ttl <= 0 || ttl > 120*time.Second {
		t.Fatalf("expiry: %v", ttl)
	}
	if _, err := c.bump("test:tpm", -1); err == nil {
		t.Fatal("accepted negative tokens")
	}
	if err := c.AddUsage("deployment", -1); err == nil {
		t.Fatal("accepted negative usage")
	}
}
