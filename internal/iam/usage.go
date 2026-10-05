package iam

import (
	"context"
	"time"

	"xorm.io/xorm"

	"github.com/sunqirui1987/xhub/internal/logx"
	"sync"
)

var logTraceOnceUsage sync.Once

// UsageEvent is one inference call, with the ownership it had at the time.
// Nothing here is a foreign key: a member who leaves a team does not move the
// spend they already produced, and deleting a team does not erase history.
type UsageEvent struct {
	ID               int64     `xorm:"pk autoincr 'id'" json:"-"`
	RequestID        string    `xorm:"'request_id'" json:"request_id"`
	TS               time.Time `xorm:"'ts'" json:"ts"`
	KeyID            string    `xorm:"'key_id'" json:"key_id"`
	OwnerType        string    `xorm:"'owner_type'" json:"owner_type"`
	UserID           string    `xorm:"'user_id'" json:"user_id"`
	TeamID           string    `xorm:"'team_id'" json:"team_id"`
	ProjectID        string    `xorm:"'project_id'" json:"project_id"`
	OrganizationID   string    `xorm:"'organization_id'" json:"organization_id"`
	Model            string    `xorm:"'model'" json:"model"`
	CallType         string    `xorm:"'call_type'" json:"call_type"`
	Status           string    `xorm:"'status'" json:"status"`
	PromptTokens     int       `xorm:"'prompt_tokens'" json:"prompt_tokens"`
	CompletionTokens int       `xorm:"'completion_tokens'" json:"completion_tokens"`
	Cost             float64   `xorm:"'cost'" json:"cost"`
	DurationMS       int       `xorm:"'duration_ms'" json:"duration_ms"`
}

func (UsageEvent) TableName() string { return "usage_events" }

// RequestLog is the stored request and response for one usage event. It is the
// sensitive half of a log: reading another account's row is what the audit log
// records.
type RequestLog struct {
	RequestID    string `xorm:"pk 'request_id'" json:"request_id"`
	RequestBody  string `xorm:"'request_body'" json:"request_body"`
	ResponseBody string `xorm:"'response_body'" json:"response_body"`
	Error        string `xorm:"'error'" json:"error"`
}

func (RequestLog) TableName() string { return "request_logs" }

// UsageDaily is the pre-aggregated roll-up, keyed by day and full ownership.
// Every dimension is part of the key so one increment touches exactly one row
// and concurrent flushes cannot land on different rows.
type UsageDaily struct {
	Day              time.Time `xorm:"pk 'day'" json:"day"`
	OrganizationID   string    `xorm:"pk 'organization_id'" json:"organization_id"`
	TeamID           string    `xorm:"pk 'team_id'" json:"team_id"`
	ProjectID        string    `xorm:"pk 'project_id'" json:"project_id"`
	UserID           string    `xorm:"pk 'user_id'" json:"user_id"`
	KeyID            string    `xorm:"pk 'key_id'" json:"key_id"`
	OwnerType        string    `xorm:"pk 'owner_type'" json:"owner_type"`
	Model            string    `xorm:"pk 'model'" json:"model"`
	Requests         int64     `xorm:"'requests'" json:"requests"`
	PromptTokens     int64     `xorm:"'prompt_tokens'" json:"prompt_tokens"`
	CompletionTokens int64     `xorm:"'completion_tokens'" json:"completion_tokens"`
	Cost             float64   `xorm:"'cost'" json:"cost"`
}

func (UsageDaily) TableName() string { return "usage_daily" }

// UsageRecord is one call to be persisted. Ownership is the snapshot taken when
// the request was authorized, not a fresh lookup: a call must be attributed to
// the identity that actually made it, not to whatever the hierarchy looks like
// by the time the batch is flushed.
type UsageRecord struct {
	RequestID        string
	TS               time.Time
	KeyID            string
	OwnerType        string
	UserID           string
	TeamID           string
	ProjectID        string
	OrganizationID   string
	Model            string
	CallType         string
	Status           string
	PromptTokens     int
	CompletionTokens int
	Cost             float64
	DurationMS       int
	RequestBody      string
	ResponseBody     string
	Error            string
}

// RecordUsage writes the event, its stored request/response, the daily roll-up
// and the live spend counters in one transaction.
//
// The batch is idempotent on request_id. A retried flush, or a second gateway
// process replaying the same Redis entry, inserts the event once and therefore
// increments everything else once. The existence check and the increments share
// the transaction so a partial replay cannot double-count.
func (db *DB) RecordUsage(ctx context.Context, records []UsageRecord) error {
	logTraceOnceUsage.Do(func() { logx.Trace("enter iam.RecordUsage") })

	if len(records) == 0 {
		return nil
	}
	return db.tx(ctx, func(s *xorm.Session) error {
		for _, r := range records {
			if r.RequestID == "" {
				continue
			}
			fresh, err := insertEvent(s, r)
			if err != nil {
				return err
			}
			if !fresh {
				continue
			}
			if err := putRequestLog(s, r); err != nil {
				return err
			}
			if err := bumpDaily(s, r); err != nil {
				return err
			}
			if err := addScopeSpend(s, r); err != nil {
				return err
			}
		}
		return nil
	})
}

// insertEvent stores one usage event and reports whether it was new.
//
// It never raises a unique violation: a replayed batch, and a second gateway
// process flushing the same Redis entry, both hit ON CONFLICT DO NOTHING and
// report "not new". Raising instead would abort the surrounding transaction on
// PostgreSQL, which would fail the whole batch over one duplicate.
func insertEvent(s *xorm.Session, r UsageRecord) (bool, error) {
	res, err := s.Exec(`INSERT INTO usage_events
        (request_id, ts, key_id, owner_type, user_id, team_id, project_id, organization_id,
         model, call_type, status, prompt_tokens, completion_tokens, cost, duration_ms)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
        ON CONFLICT (request_id) DO NOTHING`,
		r.RequestID, stamp(r.TS), r.KeyID, ownerType(r), r.UserID, r.TeamID, r.ProjectID,
		r.OrganizationID, r.Model, r.CallType, status(r),
		r.PromptTokens, r.CompletionTokens, r.Cost, r.DurationMS)
	if err != nil {
		return false, mapErr(err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, mapErr(err)
	}
	return n > 0, nil
}

// putRequestLog stores the bodies for one event. An empty pair is still stored,
// so "prompt storage was off" stays distinguishable from "the row is gone".
func putRequestLog(s *xorm.Session, r UsageRecord) error {
	row := RequestLog{RequestID: r.RequestID, RequestBody: r.RequestBody,
		ResponseBody: r.ResponseBody, Error: r.Error}
	if _, err := s.Insert(&row); err != nil {
		return mapErr(err)
	}
	return nil
}

// bumpDaily adds one request to the roll-up row for its day and ownership. The
// conflict target is the full primary key, so the increment is a single
// statement that cannot lose an update to a concurrent flusher.
func bumpDaily(s *xorm.Session, r UsageRecord) error {
	_, err := s.Exec(`INSERT INTO usage_daily
        (day, organization_id, team_id, project_id, user_id, key_id, owner_type, model,
         requests, prompt_tokens, completion_tokens, cost)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?, ?)
        ON CONFLICT (day, organization_id, team_id, project_id, user_id, key_id, owner_type, model)
        DO UPDATE SET requests = usage_daily.requests + 1,
                      prompt_tokens = usage_daily.prompt_tokens + EXCLUDED.prompt_tokens,
                      completion_tokens = usage_daily.completion_tokens + EXCLUDED.completion_tokens,
                      cost = usage_daily.cost + EXCLUDED.cost`,
		day(r.TS), r.OrganizationID, r.TeamID, r.ProjectID, r.UserID, r.KeyID, ownerType(r), r.Model,
		r.PromptTokens, r.CompletionTokens, r.Cost)
	return mapErr(err)
}

// addScopeSpend moves the live counters the budget check reads. It runs in the
// same transaction as the event, so a budget can never count a call whose event
// was rolled back, and a rolled-back flush cannot leave a phantom charge.
func addScopeSpend(s *xorm.Session, r UsageRecord) error {
	if r.Cost == 0 {
		return nil
	}
	// Deterministic order, matching every other writer, so two flushers cannot
	// deadlock by taking the same rows in opposite order.
	for _, t := range []struct{ table, id string }{
		{"organizations", r.OrganizationID}, {"teams", r.TeamID}, {"projects", r.ProjectID},
		{"users", r.UserID}, {"api_keys", r.KeyID},
	} {
		if t.id == "" {
			continue
		}
		if _, err := s.Exec("UPDATE "+t.table+" SET spend = spend + ? WHERE id = ?", r.Cost, t.id); err != nil {
			return mapErr(err)
		}
	}
	return nil
}

// stamp returns the event time in UTC, using now when the caller had none.
func stamp(t time.Time) time.Time {
	if t.IsZero() {
		return time.Now().UTC()
	}
	return t.UTC()
}

// day truncates an event time to the UTC calendar day the roll-up keys on.
func day(t time.Time) time.Time {
	s := stamp(t)
	return time.Date(s.Year(), s.Month(), s.Day(), 0, 0, 0, 0, time.UTC)
}

// ownerType is the stored owner, defaulting to service the way the schema does.
func ownerType(r UsageRecord) string {
	if r.OwnerType == "" {
		return OwnerService
	}
	return r.OwnerType
}

// status is the stored outcome, defaulting to success.
func status(r UsageRecord) string {
	if r.Status == "" {
		return "success"
	}
	return r.Status
}
