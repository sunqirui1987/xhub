// 请求路径上的 Redis 状态，以及把花费队列刷进 PostgreSQL。
package dataplane

import (
	"database/sql"
	"time"

	"github.com/sunqirui1987/xhub/internal/live"
	"github.com/sunqirui1987/xhub/internal/router"
	"github.com/sunqirui1987/xhub/internal/store"
)

// State 把进程内并发和 Redis 里的冷却、延迟、用量交给路由器。
// 没有 Redis 时只带 Busy，冷却和延迟退回空，路由器按本地策略选。
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

// RecordFailure 按当前路由设置累计失败。allowed_fails 小于 1 时不写 Redis。
// cooldown_time 为 0 或缺失时按 1 分钟冷却，和 LiteLLM 的缺省一致。
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

// 把 JSON 数字收成 float64。int 也可以。其它类型返回 0。
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

// RecordLatency 记下一次成功调用的毫秒数。没有 Redis 时直接返回。
func RecordLatency(h Host, id string, ms float64) {
	if h.Redis() == nil {
		return
	}
	_ = h.Redis().AddLatency(id, ms)
}

// RecordUsage 把本次 token 加进该部署的分钟桶。没有 Redis 时直接返回。
func RecordUsage(h Host, id string, tokens int) {
	if h.Redis() == nil {
		return
	}
	_ = h.Redis().AddUsage(id, tokens)
}

// FlushLoop 每 60 秒把 Redis 里的花费和日志写进 PostgreSQL。调用方应在进程启动后单独跑它。
func FlushLoop(h Host) {
	t := time.NewTicker(60 * time.Second)
	defer t.Stop()
	for range t.C {
		Flush(h)
	}
}

// SpendAck 是提交成功后的 Redis 确认。测试可以替换它，让第一次确认失败。
var SpendAck = func(c *live.Client, deltas map[string]float64, n int, head string) error {
	return c.AckFlushed(deltas, n, head)
}

// Flush 把 Redis 队列里的花费日志一次写入 PostgreSQL。
// 事务提交之后才确认 Redis。确认失败时两边都留着，下次还能重试。
// 热花费和日志前缀一起确认，避免只扣掉其中一边。
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

// 从一批日志汇总每个密钥、团队、用户、组织要增加的花费。零花费或无效行跳过。
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
