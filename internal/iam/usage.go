package iam

import (
	"context"
	"fmt"
	"math"
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
	// TaskSettled 是原异步请求的持久化结算标记；防止重复或并发轮询二次扣费。
	TaskSettled    bool      `xorm:"'task_settled'" json:"-"`
	ID             int64     `xorm:"pk autoincr 'id'" json:"-"`
	RequestID      string    `xorm:"'request_id'" json:"request_id"`
	TS             time.Time `xorm:"'ts'" json:"ts"`
	KeyID          string    `xorm:"'key_id'" json:"key_id"`
	OwnerType      string    `xorm:"'owner_type'" json:"owner_type"`
	UserID         string    `xorm:"'user_id'" json:"user_id"`
	TeamID         string    `xorm:"'team_id'" json:"team_id"`
	ProjectID      string    `xorm:"'project_id'" json:"project_id"`
	OrganizationID string    `xorm:"'organization_id'" json:"organization_id"`
	Model          string    `xorm:"'model'" json:"model"`
	CallType       string    `xorm:"'call_type'" json:"call_type"`
	Status         string    `xorm:"'status'" json:"status"`
	// HTTPStatus 保存网关最终返回给调用方的 HTTP 状态，供日志列表和详情展示。
	HTTPStatus       int        `xorm:"'http_status'" json:"http_status,omitempty"`
	PromptTokens     int        `xorm:"'prompt_tokens'" json:"prompt_tokens"`
	CompletionTokens int        `xorm:"'completion_tokens'" json:"completion_tokens"`
	Cost             float64    `xorm:"'cost'" json:"cost"`
	DurationMS       int        `xorm:"'duration_ms'" json:"duration_ms"`
	EndedAt          *time.Time `xorm:"'ended_at'" json:"ended_at,omitempty"`
	TTFTMs           *int       `xorm:"'ttft_ms'" json:"ttft_ms,omitempty"`
	CacheHit         bool       `xorm:"'cache_hit'" json:"cache_hit"`
	KeyHash          string     `xorm:"'key_hash'" json:"key_hash"`
	KeyAlias         string     `xorm:"'key_alias'" json:"key_alias"`
	TeamAlias        string     `xorm:"'team_alias'" json:"team_alias"`
	Provider         string     `xorm:"'provider'" json:"provider"`
	CachedTokens     *int       `xorm:"'cached_tokens'" json:"cached_tokens,omitempty"`
	SessionID        string     `xorm:"'session_id'" json:"session_id"`
	CacheKey         string     `xorm:"'cache_key'" json:"cache_key"`
	// Guardrail is the JSON array the logs drawer reads as guardrail monitoring.
	// Empty means this call was not checked.
	Guardrail string `xorm:"'guardrail'" json:"-"`
	// PriceSnapshot is the rates this call was billed at, as catalog.Snapshot
	// wrote them. The log detail reads it instead of recomputing from the
	// current price table, so a price change cannot rewrite history. Empty means
	// the call was never priced, which is not the same as costing nothing.
	PriceSnapshot string `xorm:"'price_snapshot'" json:"price_snapshot,omitempty"`
}

// 告诉 xorm 这个结构体对应数据库表 usage_events。
// 参数：无。
// 返回 string（string）：xorm 使用的表名 usage_events。这个结构体的行都进这张表。
// 调用：xorm 在映射这张表时。
// 测试：无直接单测
func (UsageEvent) TableName() string { return "usage_events" }

// RequestLog is the stored request and response for one usage event. It is the
// sensitive half of a log: reading another account's row is what the audit log
// records.
type RequestLog struct {
	RequestID    string `xorm:"pk 'request_id'" json:"request_id"`
	RequestBody  string `xorm:"'request_body'" json:"request_body"`
	ResponseBody string `xorm:"'response_body'" json:"response_body"`
	Error        string `xorm:"'error'" json:"error"`
	// UpstreamResponse 独立保存诊断，不受提示词存储开关控制。
	UpstreamResponse string `xorm:"'upstream_response'"`
	ProxyRequest     string `xorm:"'proxy_request'" json:"proxy_request"`
}

// 告诉 xorm 这个结构体对应数据库表 request_logs。
// 参数：无。
// 返回 string（string）：xorm 使用的表名 request_logs。这个结构体的行都进这张表。
// 调用：xorm 在映射这张表时。
// 测试：无直接单测
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

// 告诉 xorm 这个结构体对应数据库表 usage_daily。
// 参数：无。
// 返回 string（string）：xorm 使用的表名 usage_daily。这个结构体的行都进这张表。
// 调用：xorm 在映射这张表时。
// 测试：无直接单测
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
	HTTPStatus       int
	PromptTokens     int
	CompletionTokens int
	Cost             float64
	DurationMS       int
	RequestBody      string
	ResponseBody     string
	Error            string
	// UpstreamResponse 是已脱敏的传输事实 JSON，随请求日志持久化。
	UpstreamResponse string
	ProxyRequest     string
	EndedAt          time.Time
	TTFTMs           *int
	CacheHit         bool
	KeyHash          string
	KeyAlias         string
	TeamAlias        string
	Provider         string
	CachedTokens     *int
	SessionID        string
	CacheKey         string
	Guardrail        string
	// PriceSnapshot is the JSON of the rates this call was billed at. It is what
	// makes a historical log row explainable without recomputing from the price
	// table of the day it is read.
	PriceSnapshot string
}

// RecordUsage writes the event, its stored request/response, the daily roll-up and the live spend counters in one transaction. The batch is idempotent on request_id. A retried flush, or a second gateway
//
//	process replaying the same Redis entry, inserts the event once and therefore increments everything else once. The existence check and the increments share the transaction so a partial replay cannot double-count.
//
// 参数 ctx（context.Context）：上下文，取消时停止；records（[]UsageRecord）：一次调用的用量，含 token、费用、密钥和团队，准备写入用量表。
// 返回 error（error）：失败原因，nil 表示这一步成功。
// 调用：dataplane/live.go、gateway/spend.go、gateway/wire.go
// 测试：activity_http_test.go、prompt_log_test.go、usage_idempotency_test.go
func (db *DB) RecordUsage(ctx context.Context, records []UsageRecord) error {
	logTraceOnceUsage.Do(func() { logx.Trace("enter iam.RecordUsage") })

	if len(records) == 0 {
		return nil
	}
	records = append([]UsageRecord(nil), records...)
	for i := range records {
		r := &records[i]
		if r.RequestID == "" || r.Cost < 0 || math.IsNaN(r.Cost) || math.IsInf(r.Cost, 0) || r.PromptTokens < 0 || r.CompletionTokens < 0 || (r.CachedTokens != nil && *r.CachedTokens < 0) {
			return fmt.Errorf("usage record %d: invalid identity, cost or token count", i)
		}
		r.TS = stamp(r.TS)
	}
	return db.tx(ctx, func(s *xorm.Session) error {
		for _, r := range records {
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

// insertEvent stores one usage event and reports whether it was new. It never raises a unique violation: a replayed batch, and a second gateway process flushing the same Redis entry, both hit ON CONFLIC
// T DO NOTHING and report "not new". Raising instead would abort the surrounding transaction on PostgreSQL, which would fail the whole batch over one duplicate.
// 参数 s（*xorm.Session）：当前事务里的数据库会话。调用方负责提交，这里不关闭它；r（UsageRecord）：一次调用的用量，含 token、费用、密钥和团队，准备写入用量表。
// 返回 bool（bool）：这次写入是新的用量事件时返回真。冲突插入被忽略时返回假，同时不报唯一约束错误；error（error）：失败原因。nil 表示这一步成功。
// 调用：仅在 usage.go 内使用
// 测试：无直接单测
func insertEvent(s *xorm.Session, r UsageRecord) (bool, error) {
	var ended any
	if !r.EndedAt.IsZero() {
		ended = r.EndedAt.UTC()
	}
	res, err := s.Exec(`INSERT INTO usage_events
        (request_id, ts, key_id, owner_type, user_id, team_id, project_id, organization_id,
		 model, call_type, status, http_status, prompt_tokens, completion_tokens, cost, duration_ms,
         ended_at, ttft_ms, cache_hit, key_hash, key_alias, team_alias, provider,
         cached_tokens, session_id, cache_key, guardrail, price_snapshot)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
        ON CONFLICT (request_id) DO NOTHING`,
		r.RequestID, stamp(r.TS), r.KeyID, ownerType(r), r.UserID, r.TeamID, r.ProjectID,
		r.OrganizationID, r.Model, r.CallType, status(r), r.HTTPStatus,
		r.PromptTokens, r.CompletionTokens, r.Cost, r.DurationMS,
		ended, r.TTFTMs, r.CacheHit, r.KeyHash, r.KeyAlias, r.TeamAlias, r.Provider,
		r.CachedTokens, r.SessionID, r.CacheKey, r.Guardrail, r.PriceSnapshot)
	if err != nil {
		return false, mapErr(err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, mapErr(err)
	}
	return n > 0, nil
}

// putRequestLog 保存调用正文及已脱敏的上游诊断；即使正文关闭存储仍保留日志行和独立诊断。
// 参数 s（*xorm.Session）：当前事务里的数据库会话。调用方负责提交，这里不关闭它；r（UsageRecord）：一次调用的用量，含 token、费用、密钥和团队，准备写入用量表。
// 返回 error（error）：失败原因，nil 表示这一步成功。
// 调用：普通用量与异步任务创建事务；失败由调用方回滚。
// 测试：task_usage_test.go、cmd/regression/upstream_diagnostics_test.go 验证真实落库。
func putRequestLog(s *xorm.Session, r UsageRecord) error {
	row := RequestLog{RequestID: r.RequestID, RequestBody: r.RequestBody,
		ResponseBody: r.ResponseBody, Error: r.Error, ProxyRequest: r.ProxyRequest, UpstreamResponse: r.UpstreamResponse}
	if _, err := s.Insert(&row); err != nil {
		return mapErr(err)
	}
	return nil
}

// bumpDaily adds one request to the roll-up row for its day and ownership. The conflict target is the full primary key, so the increment is a single statement that cannot lose an update to a concurrentflusher.
// 参数 s（*xorm.Session）：当前事务里的数据库会话。调用方负责提交，这里不关闭它；r（UsageRecord）：一次调用的用量，含 token、费用、密钥和团队，准备写入用量表。
// 返回 error（error）：失败原因，nil 表示这一步成功。
// 调用：仅在 usage.go 内使用
// 测试：无直接单测
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

// addScopeSpend moves the live counters the budget check reads. It runs in the same transaction as the event, so a budget can never count a call whose event was rolled back, and a rolled-back flush cannot leave a phantom charge.
// 参数 s（*xorm.Session）：当前事务里的数据库会话。调用方负责提交，这里不关闭它；r（UsageRecord）：一次调用的用量，含 token、费用、密钥和团队，准备写入用量表。
// 返回 error（error）：失败原因，nil 表示这一步成功。
// 调用：仅在 usage.go 内使用
// 测试：无直接单测
func addScopeSpend(s *xorm.Session, r UsageRecord) error {
	if r.Cost == 0 {
		return nil
	}
	// Stable scope order within an event. Differently ordered batches can still
	// deadlock; callers must retry the entire transaction on failure.
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
// 参数 t（time.Time）：时间点。零值表示调用方没有提供时间。
// 返回 time.Time（time.Time）：解析出的时间。
// 调用：仅在 usage.go 内使用
// 测试：无直接单测
func stamp(t time.Time) time.Time {
	if t.IsZero() {
		return time.Now().UTC()
	}
	return t.UTC()
}

// day truncates an event time to the UTC calendar day the roll-up keys on.
// 参数 t（time.Time）：时间点。零值表示调用方没有提供时间。
// 返回 time.Time（time.Time）：解析出的时间。
// 调用：仅在 usage.go 内使用
// 测试：无直接单测
func day(t time.Time) time.Time {
	s := stamp(t)
	return time.Date(s.Year(), s.Month(), s.Day(), 0, 0, 0, 0, time.UTC)
}

// ownerType is the stored owner, defaulting to service the way the schema does.
// 参数 r（UsageRecord）：一次调用的用量，含 token、费用、密钥和团队，准备写入用量表。
// 返回 string（string）：库存的归属类型。空值按服务账号处理，和表的默认一致。
// 调用：仅在 usage.go 内使用
// 测试：无直接单测
func ownerType(r UsageRecord) string {
	if r.OwnerType == "" {
		return OwnerService
	}
	return r.OwnerType
}

// status is the stored outcome, defaulting to success.
// 参数 r（UsageRecord）：一次调用的用量，含 token、费用、密钥和团队，准备写入用量表。
// 返回 string（string）：库存的结果状态。空值按 success 处理。
// 调用：仅在 usage.go 内使用
// 测试：无直接单测
func status(r UsageRecord) string {
	if r.Status == "" {
		return "success"
	}
	return r.Status
}
