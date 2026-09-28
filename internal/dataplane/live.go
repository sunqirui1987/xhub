// Package dataplane reads Redis state on the request path and flushes the spend queue into PostgreSQL.
package dataplane

import (
	"database/sql"
	"time"

	"github.com/sunqirui1987/xhub/internal/live"
	"github.com/sunqirui1987/xhub/internal/router"
	"github.com/sunqirui1987/xhub/internal/store"
)

// State gives the router the in-process concurrency plus cooldown, latency, and usage from Redis.
// Without Redis only Busy is filled. Cooldown and latency stay empty, and the router uses its local strategy.
func State(h Host) router.State {
	st := router.State{Busy: h.BusyMap()}
	redis := h.Redis()
	if redis == nil {
		return st
	}
	ids := make([]string, 0, len(h.Models()))
	for _, m := range h.Models() {
		ids = append(ids, router.DeploymentID(m))
	}
	st.Cooldown = redis.Cooled(ids)
	st.Latency = redis.Latencies(ids)
	st.Usage = redis.Usages(ids)
	return st
}

// RecordFailure counts a failure using the current router settings. An allowed_fails below 1 does not write Redis.
// A cooldown_time of 0 or a missing value cools down for one minute, matching the LiteLLM default.
func RecordFailure(h Host, id string) {
	redis := h.Redis()
	if redis == nil || id == "" {
		return
	}
	m := h.RouterDocument()
	allowed := asInt(m["allowed_fails"])
	if allowed < 1 {
		return
	}
	cd := time.Duration(asFloat(m["cooldown_time"]) * float64(time.Second))
	if cd <= 0 {
		cd = time.Minute
	}
	_ = redis.RecordFailure(id, allowed, cd)
}

// asFloat converts a JSON number to float64. An int is accepted. Any other type returns 0.
func asFloat(v any) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case int:
		return float64(t)
	default:
		return 0
	}
}

// RecordLatency stores the milliseconds of one successful call. Without Redis it returns immediately.
func RecordLatency(h Host, id string, ms float64) {
	if h.Redis() == nil {
		return
	}
	_ = h.Redis().AddLatency(id, ms)
}

// RecordUsage adds this call's tokens to the deployment's minute bucket. Without Redis it returns immediately.
func RecordUsage(h Host, id string, tokens int) {
	if h.Redis() == nil {
		return
	}
	_ = h.Redis().AddUsage(id, tokens)
}

// FlushLoop writes Redis spend and logs into PostgreSQL every 60 seconds. The caller should run it on its own after the process starts.
func FlushLoop(h Host) {
	t := time.NewTicker(60 * time.Second)
	defer t.Stop()
	for range t.C {
		Flush(h)
	}
}

// SpendAck acknowledges Redis after a successful commit. A test can replace it so the first acknowledgement fails.
var SpendAck = func(c *live.Client, deltas map[string]float64, n int, head string) error {
	return c.AckFlushed(deltas, n, head)
}

// Flush writes the spend logs currently queued in Redis into PostgreSQL once.
// The Redis acknowledgement runs only after the transaction commits. If acknowledgement fails, both sides keep the data so the next flush can retry.
// Hot spend and the log prefix are acknowledged together so one side is not dropped without the other.
func Flush(h Host) {
	redis := h.Redis()
	db := h.SpendStore()
	if redis == nil || db == nil {
		return
	}
	logs, raw := redis.PeekLogs(500)
	n := len(raw)
	if n == 0 {
		return
	}
	head := raw[0]
	rows := make([]store.SpendLogRow, 0, len(logs))
	for _, row := range logs {
		logged := sql.NullFloat64{}
		if row.SpendValid {
			logged = sql.NullFloat64{Float64: row.Spend, Valid: true}
		}
		start, err := time.Parse(time.RFC3339Nano, row.Start)
		if err != nil {
			start = time.Now()
		}
		end, err := time.Parse(time.RFC3339Nano, row.End)
		if err != nil {
			end = time.Now()
		}
		rows = append(rows, store.SpendLogRow{
			RequestID: row.RequestID, CallType: row.CallType, Model: row.Model, APIKey: row.APIKey,
			Prompt: row.Prompt, Completion: row.Completion, Spend: logged,
			Start: start, End: end, CacheHit: row.CacheHit, Status: row.Status,
			TeamID: row.TeamID, UserID: row.UserID, OrgID: row.OrgID,
		})
	}
	if len(rows) == 0 {
		_ = SpendAck(redis, nil, n, head)
		return
	}
	if err := db.ApplySpendBatch(rows); err != nil {
		return
	}
	_ = SpendAck(redis, hotDeltas(logs), n, head)
}

// hotDeltas totals the spend each key, team, user, and organization should gain from a batch. A zero or invalid row is skipped.
func hotDeltas(logs []live.SpendLog) map[string]float64 {
	out := map[string]float64{}
	for _, row := range logs {
		if !row.SpendValid || row.Spend == 0 {
			continue
		}
		for _, id := range []string{row.APIKey, row.TeamID, row.UserID, row.OrgID} {
			if id != "" {
				out[id] += row.Spend
			}
		}
	}
	return out
}
