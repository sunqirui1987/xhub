package store

import (
	"database/sql"
	"strings"
	"sync/atomic"
	"time"
)

// SpendLogRow is one request log flushed from Redis into PostgreSQL.
type SpendLogRow struct {
	RequestID  string
	CallType   string
	Model      string
	APIKey     string
	Prompt     int
	Completion int
	Spend      sql.NullFloat64
	Start      time.Time
	End        time.Time
	CacheHit   bool
	Status     string
	TeamID     string
	UserID     string
	OrgID      string
}

func (s *Store) noteStmt() {
	if s == nil {
		return
	}
	atomic.AddInt64(&s.stmts, 1)
}

// Statements is how many SQL statements this store has counted since the last reset.
func (s *Store) Statements() int64 {
	if s == nil {
		return 0
	}
	return atomic.LoadInt64(&s.stmts)
}

// ResetStatements zeroes the statement counter used to check a flush is a batch.
func (s *Store) ResetStatements() {
	if s == nil {
		return
	}
	atomic.StoreInt64(&s.stmts, 0)
}

func (s *Store) execOn(tx *sql.Tx, query string, args ...any) error {
	s.noteStmt()
	_, err := tx.Exec(rewritePlaceholders(query), args...)
	return err
}

// ApplySpendBatch writes request logs and entity spend deltas in one transaction.
// Spend is added only for request ids inserted by this transaction. A replay of the
// same rows conflicts and does not add the deltas again.
// A failure rolls the transaction back and leaves the caller holding the queue.
func (s *Store) ApplySpendBatch(rows []SpendLogRow) error {
	if s == nil {
		return sql.ErrConnDone
	}
	if len(rows) == 0 {
		return nil
	}
	s.noteStmt()
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	inserted, err := s.insertSpendLogs(tx, rows)
	if err != nil {
		return err
	}
	keys, teams, users, orgs := deltasForInserted(rows, inserted)
	if err := s.addSpendDeltas(tx, "verification_tokens", "token", keys); err != nil {
		return err
	}
	if err := s.addSpendDeltas(tx, "teams", "team_id", teams); err != nil {
		return err
	}
	if err := s.addSpendDeltas(tx, "users", "user_id", users); err != nil {
		return err
	}
	if err := s.addSpendDeltas(tx, "organizations", "organization_id", orgs); err != nil {
		return err
	}
	s.noteStmt()
	return tx.Commit()
}

func deltasForInserted(rows []SpendLogRow, inserted map[string]struct{}) (keys, teams, users, orgs map[string]float64) {
	keys = map[string]float64{}
	teams = map[string]float64{}
	users = map[string]float64{}
	orgs = map[string]float64{}
	for _, row := range rows {
		if _, ok := inserted[row.RequestID]; !ok || !row.Spend.Valid || row.Spend.Float64 == 0 {
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
	return keys, teams, users, orgs
}

func (s *Store) insertSpendLogs(tx *sql.Tx, rows []SpendLogRow) (map[string]struct{}, error) {
	var b strings.Builder
	args := make([]any, 0, len(rows)*11)
	b.WriteString(`INSERT INTO spend_logs (
		request_id, call_type, model, api_key, prompt_tokens, completion_tokens, spend, start_time, end_time, cache_hit, status
	) VALUES `)
	for i, row := range rows {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString("(?,?,?,?,?,?,?,?,?,?,?)")
		status := row.Status
		if status == "" {
			status = "success"
		}
		var spendVal any
		if row.Spend.Valid {
			spendVal = row.Spend.Float64
		}
		args = append(args, row.RequestID, row.CallType, row.Model, row.APIKey, row.Prompt, row.Completion, spendVal,
			row.Start.UTC().Format(time.RFC3339Nano), row.End.UTC().Format(time.RFC3339Nano), boolInt(row.CacheHit), status)
	}
	b.WriteString(` ON CONFLICT (request_id) DO NOTHING RETURNING request_id`)
	s.noteStmt()
	scanned, err := tx.Query(rewritePlaceholders(b.String()), args...)
	if err != nil {
		return nil, err
	}
	defer scanned.Close()
	inserted := map[string]struct{}{}
	for scanned.Next() {
		var id string
		if err := scanned.Scan(&id); err != nil {
			return nil, err
		}
		inserted[id] = struct{}{}
	}
	return inserted, scanned.Err()
}

func (s *Store) addSpendDeltas(tx *sql.Tx, table, idCol string, deltas map[string]float64) error {
	if len(deltas) == 0 {
		return nil
	}
	var b strings.Builder
	args := make([]any, 0, len(deltas)*2)
	b.WriteString(`UPDATE ` + table + ` AS t SET spend = t.spend + d.delta FROM (VALUES `)
	i := 0
	for id, delta := range deltas {
		if id == "" || delta == 0 {
			continue
		}
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString("(?::text, ?::float8)")
		args = append(args, id, delta)
		i++
	}
	if i == 0 {
		return nil
	}
	b.WriteString(`) AS d(id, delta) WHERE t.` + idCol + ` = d.id`)
	return s.execOn(tx, b.String(), args...)
}
