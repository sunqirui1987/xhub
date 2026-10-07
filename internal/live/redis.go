// Package live is Redis for the hot path. Cooldown, latency, RPM, TPM, and spend deltas live here. The request itself does not write PostgreSQL.
package live

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/sunqirui1987/xhub/internal/logx"
	"sync"
)

// Client is the hot-path Redis client. A request only updates it. Spend logs are written to PostgreSQL by the timed flush.
// Client is the gateway hot path. Router cooldown, latency, and token counters
// live here, as do RPM/TPM buckets and the spend delta. Spend logs are queued
// and flushed to PostgreSQL by the server; the request does not insert them.
type Client struct {
	rdb *redis.Client
}

// SpendRef 拼出热花费计数的 Redis 键，按密钥、团队、用户、组织或项目分开。
// 参数 kind（string）：分类名，用来选择限额主体、日志类型或官方端点；id（string）：花费引用使用的主键。空串表示调用方没有指定记录。
// 返回 string（string）：热花费计数的 Redis 前缀，按密钥、团队、用户或组织分开。
// 调用：dataplane/live.go、gateway/limits.go、gateway/spend.go
// 测试：无直接单测
func SpendRef(kind, id string) string {
	if kind == "" || id == "" {
		return id
	}
	return kind + "/" + id
}

var logTraceOnceRedis sync.Once

// Open parses the Redis URL and pings it. On failure it closes the client and returns the error.
// 参数 url（string）：根地址或完整 URL。空串表示改用供应商默认根，末尾斜杠会去掉。
// 返回 *Client（*Client）：已 ping 通的 Redis 客户端。解析 URL 或 ping 失败时为 nil，并且会关掉半开的客户端；error（error）：URL 不合法或 ping 失败。nil 表示可以用。
// 调用：gateway/server.go、iam/db.go、store/engine.go、store/store.go
// 测试：authz_test.go、builtin_providers_test.go、chains_test.go
func Open(url string) (*Client, error) {
	logTraceOnceRedis.Do(func() { logx.Trace("enter live.Open") })

	opt, err := redis.ParseURL(url)
	if err != nil {
		return nil, err
	}
	c := redis.NewClient(opt)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := c.Ping(ctx).Err(); err != nil {
		_ = c.Close()
		return nil, err
	}
	return &Client{rdb: c}, nil
}

// Close closes Redis. A nil client is not an error.
// 参数：无。
// 返回 error（error）：失败原因，nil 表示这一步成功。
// 调用：dataplane/official.go、dataplane/serve.go、dataplane/stream.go、gateway/ingress.go
// 测试：authz_test.go、builtin_providers_test.go、bypass_logic_test.go
func (c *Client) Close() error {
	if c == nil || c.rdb == nil {
		return nil
	}
	return c.rdb.Close()
}

// RecordFailure increments the failure count. After allowed failures it writes the cooldown key. An allowed below 1 does nothing.
// 参数 id（string）：记录失败使用的主键。空串表示调用方没有指定记录；allowed（int）：记录失败使用的整数。零表示没有这项或尚未计数；cooldown（time.Duration）：一段时间。零值表示改用调用方约定的默认时长，例如会话一小时、官方任务七天。
// 返回 error（error）：失败原因，nil 表示这一步成功。
// 调用：dataplane/live.go、gateway/wire.go
// 测试：无直接单测
func (c *Client) RecordFailure(id string, allowed int, cooldown time.Duration) error {
	if c == nil || id == "" || allowed < 1 {
		return nil
	}
	ctx := context.Background()
	n, err := c.rdb.Incr(ctx, "xhub:fails:"+id).Result()
	if err != nil {
		return err
	}
	_ = c.rdb.Expire(ctx, "xhub:fails:"+id, cooldown).Err()
	if int(n) < allowed {
		return nil
	}
	return c.rdb.Set(ctx, "xhub:cooldown:"+id, "1", cooldown).Err()
}

// Cooled reports which deployment ids are still cooling down. If Redis is unavailable it returns an empty map and the caller treats that as no cooldown.
// 参数 ids（[]string）：要保留或查询的一组 id。空切片表示没有可处理的记录。
// 返回 map[string]bool（map[string]bool）：Cooled。没有该键表示假，不要当成缺省 JSON。
// 调用：dataplane/live.go
// 测试：无直接单测
func (c *Client) Cooled(ids []string) map[string]bool {
	out := map[string]bool{}
	if c == nil || len(ids) == 0 {
		return out
	}
	ctx := context.Background()
	keys := make([]string, len(ids))
	for i, id := range ids {
		keys[i] = "xhub:cooldown:" + id
	}
	vals, err := c.rdb.MGet(ctx, keys...).Result()
	if err != nil {
		return out
	}
	for i, v := range vals {
		if v != nil {
			out[ids[i]] = true
		}
	}
	return out
}

// AddLatency pushes one latency onto the left of the list, keeps the latest 20, and expires the key after one hour.
// 参数 id（string）：累加延迟使用的主键。空串表示调用方没有指定记录；ms（float64）：累加延迟使用的小数。0 表示没有费用或尚未计价。
// 返回 error（error）：失败原因，nil 表示这一步成功。
// 调用：dataplane/live.go
// 测试：无直接单测
func (c *Client) AddLatency(id string, ms float64) error {
	if c == nil || id == "" {
		return nil
	}
	ctx := context.Background()
	key := "xhub:latency:" + id
	pipe := c.rdb.TxPipeline()
	pipe.LPush(ctx, key, fmt.Sprintf("%g", ms))
	pipe.LTrim(ctx, key, 0, 19)
	pipe.Expire(ctx, key, time.Hour)
	_, err := pipe.Exec(ctx)
	return err
}

// Latencies returns the average of recent latencies for each deployment. An id with no sample is omitted.
// 参数 ids（[]string）：要保留或查询的一组 id。空切片表示没有可处理的记录。
// 返回 map[string]float64（map[string]float64）：Latencies的数值表，例如花费或 token。没有该键表示还没有发生过。
// 调用：dataplane/live.go
// 测试：无直接单测
func (c *Client) Latencies(ids []string) map[string]float64 {
	out := map[string]float64{}
	if c == nil {
		return out
	}
	ctx := context.Background()
	for _, id := range ids {
		vals, err := c.rdb.LRange(ctx, "xhub:latency:"+id, 0, 19).Result()
		if err != nil || len(vals) == 0 {
			continue
		}
		sum := 0.0
		n := 0
		for _, raw := range vals {
			v, err := strconv.ParseFloat(raw, 64)
			if err != nil {
				continue
			}
			sum += v
			n++
		}
		if n > 0 {
			out[id] = sum / float64(n)
		}
	}
	return out
}

// AddUsage adds tokens to the current minute bucket.
// 参数 id（string）：累加用量使用的主键。空串表示调用方没有指定记录；tokens（int）：累加用量使用的整数。零表示没有这项或尚未计数。
// 返回 error（error）：失败原因，nil 表示这一步成功。
// 调用：dataplane/live.go
// 测试：无直接单测
func (c *Client) AddUsage(id string, tokens int) error {
	if c == nil || id == "" || tokens == 0 {
		return nil
	}
	ctx := context.Background()
	key := "xhub:routetpm:" + id
	if err := c.rdb.IncrBy(ctx, key, int64(tokens)).Err(); err != nil {
		return err
	}
	return c.rdb.Expire(ctx, key, time.Minute).Err()
}

// Usages returns each deployment's token usage for the current minute.
// 参数 ids（[]string）：要保留或查询的一组 id。空切片表示没有可处理的记录。
// 返回 map[string]float64（map[string]float64）：Usages的数值表，例如花费或 token。没有该键表示还没有发生过。
// 调用：dataplane/live.go
// 测试：无直接单测
func (c *Client) Usages(ids []string) map[string]float64 {
	out := map[string]float64{}
	if c == nil || len(ids) == 0 {
		return out
	}
	ctx := context.Background()
	keys := make([]string, len(ids))
	for i, id := range ids {
		keys[i] = "xhub:routetpm:" + id
	}
	vals, err := c.rdb.MGet(ctx, keys...).Result()
	if err != nil {
		return out
	}
	for i, v := range vals {
		s, _ := v.(string)
		n, err := strconv.ParseFloat(s, 64)
		if err == nil {
			out[ids[i]] = n
		}
	}
	return out
}

// minuteBucket is the current Unix minute, used as the suffix of RPM and TPM keys.
// 参数：无。
// 返回 int64（int64）：当前 Unix 分钟，即 time.Now().Unix()/60。用作 RPM 和 TPM 键的后缀。这一步不会失败，也不会返回 0 来表示缺失。
// 调用：仅在 redis.go 内使用
// 测试：无直接单测
func minuteBucket() int64 { return time.Now().Unix() / 60 }

// HitRPM increments the current minute's request count and returns the new value.
// 参数 id（string）：虚拟密钥的令牌哈希，即 Principal.Hash。不是部署 id。计数键是 xhub:rpm: 加上这个哈希，再加当前分钟。
// 返回 int64（int64）：这一分钟累加 1 之后的请求数。客户端为 nil 时返回 0 且 error 为 nil；Redis INCR 失败时返回 0 和 error。
// 调用：gateway/limits.go
// 测试：无直接单测
func (c *Client) HitRPM(id string) (int64, error) {
	return c.bump("xhub:rpm:"+id, 1)
}

// HitTPM adds tokens to the current minute and returns the new value.
// 参数 id（string）：虚拟密钥的令牌哈希，即 Principal.Hash。不是部署 id。计数键是 xhub:tpm: 加上这个哈希，再加当前分钟；tokens（int）：这次估算要累加的 token 数。
// 返回 int64（int64）：这一分钟累加后的 token 数。客户端为 nil 时返回 0 且 error 为 nil；Redis INCR 失败时返回 0 和 error。
// 调用：gateway/limits.go
// 测试：无直接单测
func (c *Client) HitTPM(id string, tokens int) (int64, error) {
	return c.bump("xhub:tpm:"+id, int64(tokens))
}

// bump adds n to a minute counter and returns the new value. The key includes the minute number, so the next minute starts at zero after expiry.
// 参数 prefix（string）：Redis 键的前半段。HitRPM 传入 xhub:rpm: 加上密钥哈希，HitTPM 传入 xhub:tpm: 加上密钥哈希。不是部署 id，也不是要剥掉的路径前缀；n（int64）：要加到这个分钟计数上的数量。
// 返回 int64（int64）：INCRBY 之后的新计数。客户端为 nil 时返回 0 且 error 为 nil。Redis 失败时返回 0 和 error。键两分钟后过期。
// 调用：仅在 redis.go 内使用
// 测试：无直接单测
func (c *Client) bump(prefix string, n int64) (int64, error) {
	if c == nil {
		return 0, nil
	}
	ctx := context.Background()
	key := fmt.Sprintf("%s:%d", prefix, minuteBucket())
	v, err := c.rdb.IncrBy(ctx, key, n).Result()
	if err != nil {
		return 0, err
	}
	_ = c.rdb.Expire(ctx, key, 2*time.Minute).Err()
	return v, nil
}

// ChargeSpend adds a dollar delta to hot spend and puts the id in the set waiting to be flushed.
// 参数 id（string）：Charge花费使用的主键。空串表示调用方没有指定记录；usd（float64）：Charge花费使用的小数。0 表示没有费用或尚未计价。
// 返回 error（error）：失败原因，nil 表示这一步成功。
// 调用：仅在 redis.go 内使用
// 测试：无直接单测
func (c *Client) ChargeSpend(id string, usd float64) error {
	if c == nil || id == "" || usd == 0 {
		return nil
	}
	ctx := context.Background()
	if err := c.rdb.IncrByFloat(ctx, "xhub:spend:"+id, usd).Err(); err != nil {
		return err
	}
	return c.rdb.SAdd(ctx, "xhub:spend:ids", id).Err()
}

// HotSpend is the spend delta not yet flushed to PostgreSQL. A missing id returns 0.
// 参数 id（string）：热花费使用的主键。空串表示调用方没有指定记录。
// 返回 float64（float64）：热花费。没有计数或类型不符时为 0。
// 调用：gateway/limits.go、gateway/spend.go
// 测试：无直接单测
func (c *Client) HotSpend(id string) float64 {
	if c == nil || id == "" {
		return 0
	}
	v, err := c.rdb.Get(context.Background(), "xhub:spend:"+id).Float64()
	if err != nil {
		return 0
	}
	return v
}

// PeekSpend reads hot spend with GET and does not subtract or delete it. After a failed flush the same deltas can still be read.
// 参数：无。
// 返回 map[string]float64（map[string]float64）：Peek花费的数值表，例如花费或 token。没有该键表示还没有发生过。
// 调用：仅在 redis.go 内使用
// 测试：无直接单测
func (c *Client) PeekSpend() map[string]float64 {
	out := map[string]float64{}
	if c == nil {
		return out
	}
	ctx := context.Background()
	ids, err := c.rdb.SMembers(ctx, "xhub:spend:ids").Result()
	if err != nil {
		return out
	}
	for _, id := range ids {
		v, err := c.rdb.Get(ctx, "xhub:spend:"+id).Float64()
		if err != nil || v == 0 {
			continue
		}
		out[id] = v
	}
	return out
}

// AckSpend subtracts a delta only after PostgreSQL has committed it. It decrements the Redis counters for the map it is given and does not read the database itself.
// 参数 deltas（map[string]float64）：这次要累加的热计数，键是花费或 token 维度。
// 返回 error（error）：失败原因，nil 表示这一步成功。
// 调用：仅在 redis.go 内使用
// 测试：无直接单测
func (c *Client) AckSpend(deltas map[string]float64) error {
	if c == nil || len(deltas) == 0 {
		return nil
	}
	ctx := context.Background()
	for id, delta := range deltas {
		if id == "" || delta == 0 {
			continue
		}
		if err := c.rdb.IncrByFloat(ctx, "xhub:spend:"+id, -delta).Err(); err != nil {
			return err
		}
	}
	return nil
}

// ClearSpendQueue drops spend waiting to be flushed. Use it only from a test or an explicit reset.
// 参数：无。
// 返回 error（error）：失败原因，nil 表示这一步成功。
// 调用：仅在 redis.go 内使用
// 测试：无直接单测
func (c *Client) ClearSpendQueue() error {
	if c == nil {
		return nil
	}
	ctx := context.Background()
	ids, _ := c.rdb.SMembers(ctx, "xhub:spend:ids").Result()
	keys := []string{"xhub:spendlog", "xhub:spend:ids"}
	for _, id := range ids {
		keys = append(keys, "xhub:spend:"+id)
	}
	return c.rdb.Del(ctx, keys...).Err()
}

// TakeSpend reads and clears spend waiting to be flushed. Unlike Peek, a later failure no longer finds the delta in Redis.
// 参数：无。
// 返回 map[string]float64（map[string]float64）：取走花费的数值表，例如花费或 token。没有该键表示还没有发生过。
// 调用：仅在 redis.go 内使用
// 测试：无直接单测
func (c *Client) TakeSpend() map[string]float64 {
	out := map[string]float64{}
	if c == nil {
		return out
	}
	ctx := context.Background()
	ids, err := c.rdb.SMembers(ctx, "xhub:spend:ids").Result()
	if err != nil {
		return out
	}
	script := redis.NewScript(`local v = redis.call('GET', KEYS[1]); if not v then return '0' end; redis.call('SET', KEYS[1], '0'); return v`)
	for _, id := range ids {
		raw, err := script.Run(ctx, c.rdb, []string{"xhub:spend:" + id}).Text()
		if err != nil {
			continue
		}
		n, err := strconv.ParseFloat(raw, 64)
		if err != nil || n == 0 {
			continue
		}
		out[id] = n
	}
	return out
}

// SpendLog is one spend log waiting in the queue to be written to PostgreSQL.
type SpendLog struct {
	RequestID string `json:"request_id"`
	CallType  string `json:"call_type"`
	Model     string `json:"model"`
	APIKey    string `json:"api_key"`
	// KeyID is the api_keys row id, which is what usage_events keys ownership
	// by. APIKey stays the token hash, because that is the live-spend reference.
	KeyID        string  `json:"key_id,omitempty"`
	Prompt       int     `json:"prompt_tokens"`
	Completion   int     `json:"completion_tokens"`
	Spend        float64 `json:"spend"`
	SpendValid   bool    `json:"spend_valid"`
	Start        string  `json:"start"`
	End          string  `json:"end"`
	CacheHit     bool    `json:"cache_hit"`
	Status       string  `json:"status"`
	OwnerType    string  `json:"owner_type,omitempty"`
	TeamID       string  `json:"team_id"`
	UserID       string  `json:"user_id"`
	OrgID        string  `json:"org_id"`
	ProjectID    string  `json:"project_id,omitempty"`
	Messages     string  `json:"messages,omitempty"`
	Response     string  `json:"response,omitempty"`
	ProxyRequest string  `json:"proxy_server_request,omitempty"`
	TTFTMs       *int    `json:"ttft_ms,omitempty"`
	KeyHash      string  `json:"key_hash,omitempty"`
	KeyAlias     string  `json:"key_alias,omitempty"`
	TeamAlias    string  `json:"team_alias,omitempty"`
	Provider     string  `json:"provider,omitempty"`
	CachedTokens *int    `json:"cached_tokens,omitempty"`
	SessionID    string  `json:"session_id,omitempty"`
	CacheKey     string  `json:"cache_key,omitempty"`
	Guardrail    string  `json:"guardrail,omitempty"`
}

// EnqueueLog pushes a log onto the Redis list. The request path does only this step. EnqueueSpend publishes the log and its hot budget deltas in one Redis operation. A flusher cannot acknowledge a log before its matching deltas exist.
// 参数 row（SpendLog）：从用量或目录读出的SpendLog。
// 返回 error（error）：失败原因，nil 表示这一步成功。
// 调用：gateway/spend.go
// 测试：无直接单测
func (c *Client) EnqueueSpend(row SpendLog) error {
	if c == nil {
		return nil
	}
	raw, err := json.Marshal(row)
	if err != nil {
		return err
	}
	args := []any{raw, 0.0}
	if row.SpendValid {
		args[1] = row.Spend
	}
	for _, ref := range []struct{ kind, id string }{
		{"key", row.APIKey}, {"team", row.TeamID}, {"user", row.UserID},
		{"org", row.OrgID}, {"project", row.ProjectID},
	} {
		id := SpendRef(ref.kind, ref.id)
		if id != "" {
			args = append(args, id)
		}
	}
	return enqueueSpendScript.Run(context.Background(), c.rdb, []string{"xhub:spendlog"}, args...).Err()
}

var enqueueSpendScript = redis.NewScript(`
local delta = tonumber(ARGV[2])
for i = 3, #ARGV do
  if delta ~= 0 then
    redis.call('INCRBYFLOAT', 'xhub:spend:' .. ARGV[i], delta)
    redis.call('SADD', 'xhub:spend:ids', ARGV[i])
  end
end
redis.call('RPUSH', KEYS[1], ARGV[1])
return 1
`)

// 把一条花费日志放进 Redis 队列，等刷写进 PostgreSQL。客户端为空时直接返回。
// 参数 row（SpendLog）：从用量或目录读出的SpendLog。
// 返回 error（error）：失败原因，nil 表示这一步成功。
// 调用：仅在 redis.go 内使用
// 测试：无直接单测
func (c *Client) EnqueueLog(row SpendLog) error {
	if c == nil {
		return nil
	}
	raw, err := json.Marshal(row)
	if err != nil {
		return err
	}
	return c.rdb.RPush(context.Background(), "xhub:spendlog", raw).Err()
}

// PeekLogs 查看花费队列头部的若干条，不删除。客户端为空或没有日志时两个返回值都是 nil。
// 参数 n（int）：数量。
// 返回 rows（[]SpendLog）：这一批花费日志；raw（[]string）：原始文本或 JSON 字节。
// 调用：dataplane Flush，查看队列头部且不删除。
// 测试：无直接单测
func (c *Client) PeekLogs(n int) (rows []SpendLog, raw []string) {
	if c == nil || n < 1 {
		return nil, nil
	}
	vals, err := c.rdb.LRange(context.Background(), "xhub:spendlog", 0, int64(n-1)).Result()
	if err != nil || len(vals) == 0 {
		return nil, nil
	}
	out := make([]SpendLog, 0, len(vals))
	for _, item := range vals {
		var row SpendLog
		if json.Unmarshal([]byte(item), &row) != nil {
			continue
		}
		out = append(out, row)
	}
	return out, vals
}

// AckFlushed acknowledges spend deltas and the written log prefix together after the database transaction succeeds. AckFlushed subtracts hot spend and trims the log prefix in one Redis script. If the queue head is no longer the batch that was peeked, it does nothing, so a retry cannot subtract twice after a successful ack and cannot drop the logs first.
// 参数 deltas（map[string]float64）：这次要累加的热计数，键是花费或 token 维度；n（int）：数量；head（string）：AckFlushed使用的head。空串表示调用方没有提供这项。
// 返回 error（error）：失败原因，nil 表示这一步成功。
// 调用：dataplane/live.go
// 测试：无直接单测
func (c *Client) AckFlushed(deltas map[string]float64, n int, head string) error {
	if c == nil || n < 1 || head == "" {
		return nil
	}
	args := make([]any, 0, 2+len(deltas)*2)
	args = append(args, n, head)
	for id, delta := range deltas {
		if id == "" || delta == 0 {
			continue
		}
		args = append(args, id, delta)
	}
	return ackFlushScript.Run(context.Background(), c.rdb, []string{"xhub:spendlog"}, args...).Err()
}

var ackFlushScript = redis.NewScript(`
local head = redis.call('LINDEX', KEYS[1], 0)
if (not head) or head ~= ARGV[2] then
  return 0
end
local n = tonumber(ARGV[1])
local i = 3
while i <= #ARGV do
  local id = ARGV[i]
  local delta = tonumber(ARGV[i + 1])
  if id ~= '' and delta ~= 0 then
    redis.call('INCRBYFLOAT', 'xhub:spend:' .. id, -delta)
  end
  i = i + 2
end
redis.call('LTRIM', KEYS[1], n, -1)
return 1
`)

// AckLogs 从花费队列头部丢掉已经刷进数据库的 n 条。
// 参数 n（int）：要丢掉的条数。小于 1 时什么都不做。
// 返回 error（error）：Redis 修剪失败时非 nil。客户端为空或 n 小于 1 时为 nil。
// 调用：dataplane Flush。
// 测试：无直接单测
func (c *Client) AckLogs(n int) error {
	if c == nil || n < 1 {
		return nil
	}
	return c.rdb.LTrim(context.Background(), "xhub:spendlog", int64(n), -1).Err()
}

// DrainLogs 从队列头部弹出最多 n 条花费日志。弹出后队列里不再保留它们。
// 参数 n（int）：最多弹出的条数。小于 1 时返回 nil。
// 返回 []SpendLog（[]SpendLog）：弹出的花费日志。客户端为空或队列空时为 nil。
// 调用：仅在 redis.go 内使用。
// 测试：无直接单测
func (c *Client) DrainLogs(n int) []SpendLog {
	if c == nil || n < 1 {
		return nil
	}
	ctx := context.Background()
	var out []SpendLog
	for i := 0; i < n; i++ {
		raw, err := c.rdb.LPop(ctx, "xhub:spendlog").Bytes()
		if err != nil {
			break
		}
		var row SpendLog
		if json.Unmarshal(raw, &row) != nil {
			continue
		}
		out = append(out, row)
	}
	return out
}

// GetString reads one Redis string. A missing key returns ok false.
// 参数 ctx（context.Context）：上下文，取消时停止；key（string）：缓存或配置表的键。
// 返回 string（string）：读到的字符串。键不存在时为空串，调用方要把空串当成没有钉住；bool（bool）：Redis 里存在这个字符串键时返回真。没有这个键时返回假。
// 调用：gateway/affinity.go
// 测试：无直接单测
func (c *Client) GetString(ctx context.Context, key string) (string, bool) {
	if c == nil || c.rdb == nil || key == "" {
		return "", false
	}
	v, err := c.rdb.Get(ctx, key).Result()
	if err != nil || v == "" {
		return "", false
	}
	return v, true
}

// SetString stores one Redis string with a TTL. A nil client does nothing.
// 参数 ctx（context.Context）：上下文，取消时停止；key（string）：缓存或配置表的键；value（string）：写入字符串使用的值。空串表示调用方没有提供这项；ttl（time.Duration）：一段时间。零值表示改用调用方约定的默认时长，例如会话一小时、官方任务七天。
// 返回：无。这条带 TTL 的字符串已写入 Redis。客户端或键为空时什么都不写。
// 调用：gateway/affinity.go
// 测试：无直接单测
func (c *Client) SetString(ctx context.Context, key, value string, ttl time.Duration) {
	if c == nil || c.rdb == nil || key == "" {
		return
	}
	_ = c.rdb.Set(ctx, key, value, ttl).Err()
}
