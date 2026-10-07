// live.go reads Redis for the router's cooldown, latency, and usage snapshot,
// and flushes the queued spend rows into PostgreSQL. The request path calls
// State, RecordFailure, RecordLatency, and RecordUsage. FlushLoop is started
// by the process only when Redis is configured.

package dataplane

import (
	"context"
	"time"

	"github.com/sunqirui1987/xhub/internal/iam"
	"github.com/sunqirui1987/xhub/internal/live"
	"github.com/sunqirui1987/xhub/internal/logx"
	"github.com/sunqirui1987/xhub/internal/router"
	"sync"
)

var logTraceOnceLive sync.Once

// State 组装这一刻的在途请求、冷却、延迟和用量，交给路由器排序。没有 Redis 时只有在途请求。
// 参数 h：Runtime。
// 返回：router.State。
// 调用：gateway routerState。测试不连 Redis，prefer_test.go 使用空 State。
func State(h Runtime) router.State {
	logTraceOnceLive.Do(func() { logx.Trace("enter dataplane.State") })

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

// RecordFailure 按当前路由设置记一次失败。allowed_fails 小于 1 时不写 Redis。
// cooldown_time 为 0 或缺失时冷却一分钟。
// 参数 h：Runtime。id：部署 id。空 id 直接返回。
// 返回：无。
// 调用：gateway noteFailure，由 Serve 的 NoteFailure 转来。无单独测试。
func RecordFailure(h Runtime, id string) {
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

// asFloat 把路由配置里的数字收成 float64。类型不符时为 0。
// 参数 v：RouterDocument 里的 cooldown_time 或 allowed_fails。
// 返回：浮点数，无法识别时为 0。
// 调用：RecordFailure。无单独测试。
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

// RecordLatency 把这次延迟累进 Redis，供下次排序使用。没有 Redis 时直接返回。
// 参数 id：部署 id。ms：耗时毫秒。
// 返回：无。
// 调用：gateway noteLatency。无单独测试。
func RecordLatency(h Runtime, id string, ms float64) {
	if h.Redis() == nil {
		return
	}
	_ = h.Redis().AddLatency(id, ms)
}

// RecordUsage 把这次 token 数累进 Redis。没有 Redis 时直接返回。
// 参数 id：部署 id。tokens：本次 token 数。
// 返回：无。
// 调用：gateway noteUsage。无单独测试。
func RecordUsage(h Runtime, id string, tokens int) {
	if h.Redis() == nil {
		return
	}
	_ = h.Redis().AddUsage(id, tokens)
}

// FlushLoop 每分钟调用一次 Flush，直到进程退出。只在配置了 Redis 时启动。
// 参数 h：Runtime。返回：无。这个函数不返回，直到进程退出。
// 调用：gateway flushLoop，且只在配置了 Redis 时。无单测。
// 测试：无直接单测
func FlushLoop(h Runtime) {
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

// Flush 把 Redis 队列里的花费日志写入 PostgreSQL，成功后再从队列确认删除。
// 参数 h：Runtime。Redis 或 Identity 为 nil 时立即返回。
// 返回：无。
// 调用：FlushLoop 和 gateway FlushSpend。无单测。
// 测试：无直接单测
func Flush(h Runtime) {
	redis := h.Redis()
	db := h.Identity()
	if redis == nil || db == nil {
		return
	}
	logs, raw := redis.PeekLogs(500)
	n := len(raw)
	if n == 0 {
		return
	}
	head := raw[0]
	records := make([]iam.UsageRecord, 0, len(logs))
	for _, row := range logs {
		start, err := time.Parse(time.RFC3339Nano, row.Start)
		if err != nil {
			start = time.Now()
		}
		end, err := time.Parse(time.RFC3339Nano, row.End)
		if err != nil {
			end = time.Now()
		}
		spend := 0.0
		if row.SpendValid {
			spend = row.Spend
		}
		rec := iam.UsageRecord{
			RequestID: row.RequestID, TS: start, KeyID: row.KeyID, OwnerType: row.OwnerType,
			UserID: row.UserID, TeamID: row.TeamID, ProjectID: row.ProjectID, OrganizationID: row.OrgID,
			Model: row.Model, CallType: row.CallType, Status: row.Status,
			PromptTokens: row.Prompt, CompletionTokens: row.Completion, Cost: spend,
			DurationMS:  int(end.Sub(start).Milliseconds()),
			RequestBody: row.Messages, ResponseBody: row.Response, ProxyRequest: row.ProxyRequest,
			EndedAt: end, TTFTMs: row.TTFTMs, CacheHit: row.CacheHit,
			KeyHash: row.KeyHash, KeyAlias: row.KeyAlias, TeamAlias: row.TeamAlias,
			Provider: row.Provider, CachedTokens: row.CachedTokens,
			SessionID: row.SessionID, CacheKey: row.CacheKey, Guardrail: row.Guardrail,
		}
		records = append(records, rec)
	}
	if len(records) == 0 {
		_ = SpendAck(redis, nil, n, head)
		return
	}
	if err := db.RecordUsage(context.Background(), records); err != nil {
		logx.Error("spend flush failed: %v", err)
		return
	}
	_ = SpendAck(redis, hotDeltas(logs), n, head)
}

// hotDeltas 把一批花费日志按密钥、团队、用户、组织和项目拆成要回写的热计数。
// 参数 logs：Redis 队列里取出的花费日志。
// 返回：live.SpendRef 到金额的映射，交给 Redis 确认。
// 调用：Flush。测试：live_test.go TestHotDeltasIncludesProject。
func hotDeltas(logs []live.SpendLog) map[string]float64 {
	out := map[string]float64{}
	for _, row := range logs {
		if !row.SpendValid || row.Spend == 0 {
			continue
		}
		for _, ref := range []struct{ kind, id string }{
			{"key", row.APIKey}, {"team", row.TeamID}, {"user", row.UserID},
			{"org", row.OrgID}, {"project", row.ProjectID},
		} {
			id := live.SpendRef(ref.kind, ref.id)
			if id != "" {
				out[id] += row.Spend
			}
		}
	}
	return out
}
