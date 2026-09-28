// Package store writes a Redis spend batch into PostgreSQL. Replaying the same request does not add the amount again.
package store

import (
	"database/sql"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sunqirui1987/xhub/internal/logx"
	"xorm.io/xorm"
)

// orgSpendMu serializes the read-modify-write of organization spend stored in extra_json.
// xorm ForUpdate emits a lock only for MySQL. Two PostgreSQL flushes at once would drop a delta, so this mutex covers that race.
var orgSpendMu sync.Mutex

// SpendLogRow is one request log in a batch flushed from Redis into PostgreSQL.
type SpendLogRow struct {
	RequestID    string
	CallType     string
	Model        string
	APIKey       string
	Prompt       int
	Completion   int
	Spend        sql.NullFloat64
	Start        time.Time
	End          time.Time
	CacheHit     bool
	Status       string
	TeamID       string
	UserID       string
	OrgID        string
	Messages     string
	Response     string
	ProxyRequest string
}

var logTraceOnceSpendBatch sync.Once

// Statements returns how many SQL statements have actually run since the engine was opened.
func (s *Store) Statements() int64 {
	logTraceOnceSpendBatch.Do(func() { logx.Trace("enter store.Statements") })

	if s == nil || s.stmts == nil {
		return 0
	}
	return atomic.LoadInt64(s.stmts)
}

// ResetStatements zeroes the SQL counter so the next read can show whether it hit the database.
func (s *Store) ResetStatements() {
	if s == nil || s.stmts == nil {
		return
	}
	atomic.StoreInt64(s.stmts, 0)
}

// ApplySpendBatch inserts request logs that have not been seen and adds their spend onto the key, team, user, and organization.
func (s *Store) ApplySpendBatch(rows []SpendLogRow) error {
	if s == nil {
		return sql.ErrConnDone
	}
	if len(rows) == 0 {
		return nil
	}
	orgSpendMu.Lock()
	defer orgSpendMu.Unlock()
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.RequestID)
	}
	sess := s.Engine.NewSession()
	defer sess.Close()
	if err := sess.Begin(); err != nil {
		return err
	}
	var have []spendRow
	if err := sess.In("request_id", ids).Cols("request_id").Find(&have); err != nil {
		return err
	}
	seen := map[string]struct{}{}
	for _, row := range have {
		seen[row.RequestID] = struct{}{}
	}
	fresh := make([]spendRow, 0, len(rows))
	keys := map[string]float64{}
	teams := map[string]float64{}
	users := map[string]float64{}
	orgs := map[string]float64{}
	for _, row := range rows {
		if _, ok := seen[row.RequestID]; ok || row.RequestID == "" {
			continue
		}
		status := row.Status
		if status == "" {
			status = "success"
		}
		fresh = append(fresh, spendRow{
			RequestID: row.RequestID, CallType: row.CallType, Model: row.Model, APIKey: row.APIKey,
			Prompt: row.Prompt, Completion: row.Completion, Spend: fptr(row.Spend),
			StartTime: row.Start.UTC().Format(time.RFC3339Nano), EndTime: row.End.UTC().Format(time.RFC3339Nano),
			CacheHit: boolInt(row.CacheHit), Status: status, UserID: row.UserID,
			MessagesJSON: row.Messages, ResponseJSON: row.Response, ProxyRequestJSON: row.ProxyRequest,
		})
		if !row.Spend.Valid || row.Spend.Float64 == 0 {
			continue
		}
		if row.APIKey != "" {
			keys[row.APIKey] += row.Spend.Float64
		}
		if row.TeamID != "" {
			teams[row.TeamID] += row.Spend.Float64
		}
		if row.UserID != "" {
			users[row.UserID] += row.Spend.Float64
		}
		if row.OrgID != "" {
			orgs[row.OrgID] += row.Spend.Float64
		}
	}
	if len(fresh) > 0 {
		if _, err := sess.Insert(&fresh); err != nil {
			return err
		}
	}
	if err := addMappedSpend(sess, keys, teams, users, orgs); err != nil {
		return err
	}
	if err := sess.Commit(); err != nil {
		return err
	}
	s.bust(new(spendRow), new(tokenRow), new(teamRow), new(userRow), new(orgRow))
	return nil
}

// addMappedSpend adds the summarized spend inside the current transaction. Organization spend is stored in extra_json.
func addMappedSpend(sess *xorm.Session, keys, teams, users, orgs map[string]float64) error {
	for id, delta := range keys {
		if _, err := sess.ID(id).Incr("spend", delta).Update(&tokenRow{}); err != nil {
			return err
		}
	}
	for id, delta := range teams {
		if _, err := sess.ID(id).Incr("spend", delta).Update(&teamRow{}); err != nil {
			return err
		}
	}
	for id, delta := range users {
		if _, err := sess.ID(id).Incr("spend", delta).Update(&userRow{}); err != nil {
			return err
		}
	}
	for id, delta := range orgs {
		var row orgRow
		ok, err := sess.ID(id).Get(&row)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		e := orgFrom(row)
		e.Spend += delta
		packed := packOrg(e)
		packed.CreatedAt = row.CreatedAt
		if _, err := sess.ID(id).Cols("extra_json", "updated_at").Update(&packed); err != nil {
			return err
		}
	}
	return nil
}
