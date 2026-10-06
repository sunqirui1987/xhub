package iam

import (
	"context"
	"strconv"
	"sync"
	"time"

	"xorm.io/builder"
	"xorm.io/xorm"

	"github.com/sunqirui1987/xhub/internal/logx"
)

var logTraceOnceRead sync.Once

// UsageQuery narrows a usage or log read. Cond is the authorization scope and is
// never nil: the caller passes authz.UsageScope or authz.LogsScope, both of which
// return a fail-closed filter for an actor who may see nothing. It is applied as
// a WHERE fragment so the narrowing happens in SQL and a large table can never
// widen what a handler returns.
type UsageQuery struct {
	Cond      builder.Cond
	From      time.Time
	To        time.Time
	UserID    string
	TeamID    string
	ProjectID string
	KeyID     string
	Model     string
	Status    string
	SessionID string
	// TeamIDs, ExcludeTeamIDs and OrganizationIDs are caller filters. They only
	// narrow a scope that was already decided; an empty slice adds no predicate,
	// so it cannot turn into "every team".
	TeamIDs         []string
	ExcludeTeamIDs  []string
	OrganizationIDs []string
	Limit           int
	Offset          int
}

// scopedSession builds a session with the scope and the shared filters applied.
func (q UsageQuery) scopedSession(db *DB, ctx context.Context, table string) *xorm.Session {
	s := db.session(ctx).Table(table)
	if q.Cond != nil {
		s = s.Where(q.Cond)
	}
	if !q.From.IsZero() {
		s = s.And("ts >= ?", q.From.UTC())
	}
	if !q.To.IsZero() {
		s = s.And("ts <= ?", q.To.UTC())
	}
	if q.UserID != "" {
		s = s.And("user_id = ?", q.UserID)
	}
	if q.TeamID != "" {
		s = s.And("team_id = ?", q.TeamID)
	}
	if q.ProjectID != "" {
		s = s.And("project_id = ?", q.ProjectID)
	}
	if q.KeyID != "" {
		s = s.And("key_id = ?", q.KeyID)
	}
	if q.Model != "" {
		s = s.And("model = ?", q.Model)
	}
	if q.Status != "" {
		s = s.And("status = ?", q.Status)
	}
	if q.SessionID != "" {
		s = s.And("session_id = ?", q.SessionID)
	}
	return q.applyLists(s)
}

// applyLists adds the caller's id filters. Each one is optional. A filter that
// is absent must not become a predicate, and a present filter is an AND, so it
// can only remove rows the scope already allowed.
func (q UsageQuery) applyLists(s *xorm.Session) *xorm.Session {
	if len(q.TeamIDs) > 0 {
		s = s.And(builder.In("team_id", q.TeamIDs))
	}
	if len(q.ExcludeTeamIDs) > 0 {
		s = s.And(builder.NotIn("team_id", q.ExcludeTeamIDs))
	}
	if len(q.OrganizationIDs) > 0 {
		s = s.And(builder.In("organization_id", q.OrganizationIDs))
	}
	return s
}

// maxUsageRead is the most events one read may return. The activity page asks
// for the whole window it will fold; a log page asks for far fewer.
const maxUsageRead = 5000

// ListUsage returns usage events inside the scope, newest first.
//
// A missing limit is a page of logs. A caller that names a limit gets that
// many rows, capped here. Clamping an over-large limit down to a small page
// would make the usage screen report a sample as the whole window.
func (db *DB) ListUsage(ctx context.Context, q UsageQuery) ([]UsageEvent, error) {
	logTraceOnceRead.Do(func() { logx.Trace("enter iam.ListUsage") })
	limit := q.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > maxUsageRead {
		limit = maxUsageRead
	}
	var out []UsageEvent
	err := q.scopedSession(db, ctx, "usage_events").Desc("ts").Limit(limit, q.Offset).Find(&out)
	if err != nil {
		return nil, mapErr(err)
	}
	return out, nil
}

// CountUsage counts the events inside the scope, for paging.
func (db *DB) CountUsage(ctx context.Context, q UsageQuery) (int64, error) {
	n, err := q.scopedSession(db, ctx, "usage_events").Count()
	if err != nil {
		return 0, mapErr(err)
	}
	return n, nil
}

// GetUsageEvent loads one event inside the scope. A row outside the scope is
// reported as not found rather than forbidden, so a caller cannot probe which
// request ids exist.
func (db *DB) GetUsageEvent(ctx context.Context, q UsageQuery, requestID string) (*UsageEvent, error) {
	var row UsageEvent
	ok, err := q.scopedSession(db, ctx, "usage_events").Where("request_id = ?", requestID).Get(&row)
	if err != nil {
		return nil, mapErr(err)
	}
	if !ok {
		return nil, ErrNotFound
	}
	return &row, nil
}

// GetRequestLog loads the stored bodies of one event that is already inside the
// scope. The caller checks the scope with GetUsageEvent first.
func (db *DB) GetRequestLog(ctx context.Context, requestID string) (*RequestLog, error) {
	var row RequestLog
	ok, err := db.session(ctx).Where("request_id = ?", requestID).Get(&row)
	if err != nil {
		return nil, mapErr(err)
	}
	if !ok {
		return nil, ErrNotFound
	}
	return &row, nil
}

// DailyRow is one day of the roll-up inside a scope, with the day already
// truncated in the caller's timezone.
type DailyRow struct {
	Day              string  `xorm:"'day'" json:"day"`
	Requests         int64   `xorm:"'requests'" json:"requests"`
	PromptTokens     int64   `xorm:"'prompt_tokens'" json:"prompt_tokens"`
	CompletionTokens int64   `xorm:"'completion_tokens'" json:"completion_tokens"`
	Cost             float64 `xorm:"'cost'" json:"cost"`
}

// DailyByModelRow is one day and model of the roll-up, for the per-model breakdown.
type DailyByModelRow struct {
	Day              string  `xorm:"'day'" json:"day"`
	Model            string  `xorm:"'model'" json:"model"`
	Requests         int64   `xorm:"'requests'" json:"requests"`
	PromptTokens     int64   `xorm:"'prompt_tokens'" json:"prompt_tokens"`
	CompletionTokens int64   `xorm:"'completion_tokens'" json:"completion_tokens"`
	Cost             float64 `xorm:"'cost'" json:"cost"`
}

// dailyScope applies the scope and the date window to the roll-up table. The
// roll-up carries the same ownership columns as the events, so the scope needs
// no join. A timezone offset in minutes shifts the day boundary, because the
// roll-up is stored on UTC days but the console renders the caller's day.
//
// The table is not joined to anything, so the scope's column names stay
// unqualified and no reference can become ambiguous.
func (q UsageQuery) dailyScope(db *DB, ctx context.Context, tzMinutes int) *xorm.Session {
	s := db.session(ctx).Table("usage_daily")
	if q.Cond != nil {
		s = s.Where(q.Cond)
	}
	shift := tzMinutes * 60
	if !q.From.IsZero() {
		s = s.And("(day + INTERVAL '"+strconv.Itoa(shift)+" seconds') >= ?", q.From.UTC().Format("2006-01-02"))
	}
	if !q.To.IsZero() {
		s = s.And("(day + INTERVAL '"+strconv.Itoa(shift)+" seconds') <= ?", q.To.UTC().Format("2006-01-02"))
	}
	if q.UserID != "" {
		s = s.And("user_id = ?", q.UserID)
	}
	if q.TeamID != "" {
		s = s.And("team_id = ?", q.TeamID)
	}
	if q.ProjectID != "" {
		s = s.And("project_id = ?", q.ProjectID)
	}
	if q.KeyID != "" {
		s = s.And("key_id = ?", q.KeyID)
	}
	if q.Model != "" {
		s = s.And("model = ?", q.Model)
	}
	return q.applyLists(s)
}

// DailyUsage totals the roll-up per day inside the scope.
func (db *DB) DailyUsage(ctx context.Context, q UsageQuery, tzMinutes int) ([]DailyRow, error) {
	var out []DailyRow
	err := q.dailyScope(db, ctx, tzMinutes).
		Select(dayExpr(tzMinutes) + ", SUM(requests) AS requests, SUM(prompt_tokens) AS prompt_tokens, " +
			"SUM(completion_tokens) AS completion_tokens, SUM(cost) AS cost").
		GroupBy("day").Asc("day").Find(&out)
	if err != nil {
		return nil, mapErr(err)
	}
	return out, nil
}

// DailyUsageByModel totals the roll-up per day and model inside the scope.
func (db *DB) DailyUsageByModel(ctx context.Context, q UsageQuery, tzMinutes int) ([]DailyByModelRow, error) {
	var out []DailyByModelRow
	err := q.dailyScope(db, ctx, tzMinutes).
		Select(dayExpr(tzMinutes) + ", model, SUM(requests) AS requests, SUM(prompt_tokens) AS prompt_tokens, " +
			"SUM(completion_tokens) AS completion_tokens, SUM(cost) AS cost").
		GroupBy("day, model").Asc("day").Find(&out)
	if err != nil {
		return nil, mapErr(err)
	}
	return out, nil
}

// dayExpr renders the day column shifted into the caller's timezone. The offset
// is a validated integer of minutes and is interpolated rather than bound: it
// sits inside the SELECT list, where xorm binds no arguments.
func dayExpr(tzMinutes int) string {
	return "to_char(day + INTERVAL '" + strconv.Itoa(tzMinutes) + " minutes', 'YYYY-MM-DD') AS day"
}

// RollupByModel totals the roll-up per model inside the scope, for the per-model
// table the usage page shows.
func (db *DB) RollupByModel(ctx context.Context, q UsageQuery, tzMinutes int) ([]DailyByModelRow, error) {
	var out []DailyByModelRow
	err := q.dailyScope(db, ctx, tzMinutes).
		Select("'' AS day, model, SUM(requests) AS requests, SUM(prompt_tokens) AS prompt_tokens, " +
			"SUM(completion_tokens) AS completion_tokens, SUM(cost) AS cost").
		GroupBy("model").Asc("model").Find(&out)
	if err != nil {
		return nil, mapErr(err)
	}
	return out, nil
}

// RollupByKey totals the roll-up per key inside the scope. The display name is a
// correlated subquery rather than a join, so the grouped table stays the only
// source of rows and the scope cannot widen or duplicate them.
//
// A key that was deleted since keeps its spend: the roll-up carries the id, and
// the name falls back to it.
func (db *DB) RollupByKey(ctx context.Context, q UsageQuery, tzMinutes int) ([]KeySpendRow, error) {
	var out []KeySpendRow
	err := q.dailyScope(db, ctx, tzMinutes).
		Select("key_id, COALESCE((SELECT name FROM api_keys WHERE api_keys.id = usage_daily.key_id), key_id) AS name, " +
			"SUM(requests) AS requests, SUM(prompt_tokens) AS prompt_tokens, " +
			"SUM(completion_tokens) AS completion_tokens, SUM(cost) AS cost").
		GroupBy("key_id").Asc("key_id").Find(&out)
	if err != nil {
		return nil, mapErr(err)
	}
	return out, nil
}

// KeySpendRow is one key's total inside a scope.
type KeySpendRow struct {
	KeyID            string  `xorm:"'key_id'" json:"key_id"`
	Name             string  `xorm:"'name'" json:"name"`
	Requests         int64   `xorm:"'requests'" json:"requests"`
	PromptTokens     int64   `xorm:"'prompt_tokens'" json:"prompt_tokens"`
	CompletionTokens int64   `xorm:"'completion_tokens'" json:"completion_tokens"`
	Cost             float64 `xorm:"'cost'" json:"cost"`
}

// RollupByTeam totals the roll-up per team inside the scope, with the same
// correlated-subquery shape as RollupByKey.
func (db *DB) RollupByTeam(ctx context.Context, q UsageQuery, tzMinutes int) ([]TeamSpendRow, error) {
	var out []TeamSpendRow
	err := q.dailyScope(db, ctx, tzMinutes).
		Select("team_id, COALESCE((SELECT name FROM teams WHERE teams.id = usage_daily.team_id), team_id) AS name, " +
			"SUM(requests) AS requests, SUM(prompt_tokens) AS prompt_tokens, " +
			"SUM(completion_tokens) AS completion_tokens, SUM(cost) AS cost").
		GroupBy("team_id").Asc("team_id").Find(&out)
	if err != nil {
		return nil, mapErr(err)
	}
	return out, nil
}

// TeamSpendRow is one team's total inside a scope.
type TeamSpendRow struct {
	TeamID           string  `xorm:"'team_id'" json:"team_id"`
	Name             string  `xorm:"'name'" json:"name"`
	Requests         int64   `xorm:"'requests'" json:"requests"`
	PromptTokens     int64   `xorm:"'prompt_tokens'" json:"prompt_tokens"`
	CompletionTokens int64   `xorm:"'completion_tokens'" json:"completion_tokens"`
	Cost             float64 `xorm:"'cost'" json:"cost"`
}

// AuditLogRead records that an actor read a log they do not own. The caller
// writes this before returning the row: the access is the event, not the row.
func (db *DB) AuditLogRead(ctx context.Context, by Actor, requestID string, e UsageEvent) error {
	detail := map[string]any{"request_id": requestID}
	if e.TeamID != "" {
		detail["team_id"] = e.TeamID
	}
	if e.UserID != "" {
		detail["user_id"] = e.UserID
	}
	return db.RecordAudit(ctx, by, Audit{Action: "log.read", ObjectType: "request_log",
		ObjectID: requestID, TeamID: e.TeamID, Detail: detail})
}
