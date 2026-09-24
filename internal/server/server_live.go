package server

import (
	"database/sql"
	"time"

	"github.com/sunqirui1987/xhub/internal/live"
	"github.com/sunqirui1987/xhub/internal/router"
	"github.com/sunqirui1987/xhub/internal/store"
)

func (s *Server) routerState() router.State {
	st := router.State{Busy: s.Busy}
	if s.Live == nil {
		return st
	}
	ids := make([]string, 0, len(s.Cfg.ModelList))
	for _, m := range s.Cfg.ModelList {
		ids = append(ids, router.DeploymentID(m))
	}
	st.Cooldown = s.Live.Cooled(ids)
	st.Latency = s.Live.Latencies(ids)
	st.Usage = s.Live.Usages(ids)
	return st
}

func (s *Server) noteFailure(id string) {
	if s.Live == nil || id == "" {
		return
	}
	m := s.mergedRouterSettings()
	allowed := asInt(m["allowed_fails"])
	if allowed < 1 {
		return
	}
	cd := time.Duration(asFloat(m["cooldown_time"]) * float64(time.Second))
	if cd <= 0 {
		cd = time.Minute
	}
	_ = s.Live.RecordFailure(id, allowed, cd)
}

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

func (s *Server) noteLatency(id string, ms float64) {
	if s.Live == nil {
		return
	}
	_ = s.Live.AddLatency(id, ms)
}

func (s *Server) noteUsage(id string, tokens int) {
	if s.Live == nil {
		return
	}
	_ = s.Live.AddUsage(id, tokens)
}

func (s *Server) flushLoop() {
	t := time.NewTicker(60 * time.Second)
	defer t.Stop()
	for range t.C {
		s.FlushSpend()
	}
}

// spendAck is the Redis step after a committed flush. Tests replace it to fail one ack.
var spendAck = func(c *live.Client, deltas map[string]float64, n int, head string) error {
	return c.AckFlushed(deltas, n, head)
}

// FlushSpend moves the Redis spend delta and queued logs into PostgreSQL in one batch.
// Redis is popped only after the transaction commits, so a failed write can be retried.
// Hot spend and the log prefix are acknowledged together. A failed ack leaves both in place.
func (s *Server) FlushSpend() {
	if s.Live == nil || s.Store == nil {
		return
	}
	logs, raw := s.Live.PeekLogs(500)
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
		_ = spendAck(s.Live, nil, n, head)
		return
	}
	if err := s.Store.ApplySpendBatch(rows); err != nil {
		return
	}
	_ = spendAck(s.Live, hotDeltas(logs), n, head)
}

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


