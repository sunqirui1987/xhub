// 热路径上的 Redis。冷却、延迟、RPM/TPM 和花费增量都在这里，请求本身不写 PostgreSQL。
package live

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// 热路径 Redis。请求只更新这里，花费日志由定时刷盘写入 PostgreSQL。
// Client is the gateway hot path. Router cooldown, latency, and token counters
// live here, as do RPM/TPM buckets and the spend delta. Spend logs are queued
// and flushed to PostgreSQL by the server; the request does not insert them.
type Client struct {
	rdb *redis.Client
}

// 解析 Redis URL 并 Ping。失败时关闭客户端并返回错误。
func Open(url string) (*Client, error) {
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

// 关闭 Redis。客户端为 nil 时不报错。
func (c *Client) Close() error {
	if c == nil || c.rdb == nil {
		return nil
	}
	return c.rdb.Close()
}

// 失败计数加一。达到 allowed 次后写入冷却键。allowed 小于 1 时什么都不做。
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

// 哪些部署 id 仍在冷却。Redis 不可用时返回空表，调用方按没有冷却处理。
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

// 把一次延迟从左侧推入列表，只保留最近 20 条，一小时过期。
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

// 每个部署最近延迟的平均值。没有样本的 id 不会出现。
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

// 把 token 加进当前分钟桶。
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

// 每个部署当前分钟的 token 用量。
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

// 当前分钟的 unix 分钟序号，用作 RPM/TPM 键的后缀。
func minuteBucket() int64 { return time.Now().Unix() / 60 }

// 当前分钟的请求数加一，并返回加完后的值。
func (c *Client) HitRPM(id string) (int64, error) {
	return c.bump("xhub:rpm:"+id, 1)
}

// 当前分钟的 token 数增加 tokens，并返回加完后的值。
func (c *Client) HitTPM(id string, tokens int) (int64, error) {
	return c.bump("xhub:tpm:"+id, int64(tokens))
}

// 把分钟计数器加上 n 并返回新值。键带分钟序号，过期后下一分钟从零开始。
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

// 把美元增量加到热花费，并把 id 放进待刷集合。
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

// 还没刷进 PostgreSQL 的花费增量。没有这个 id 时为 0。
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

// 读出待刷花费，但不清掉。刷盘失败后还可以再读。
// PeekSpend reads hot spend deltas without removing them.
// AckSpend subtracts a delta only after PostgreSQL has committed it.
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

// 确认一批花费已经入库，并从 Redis 减掉相应增量。
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

// 清空待刷花费。只给测试或显式重置使用。
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

// 取出并清零待刷花费。和 Peek 不同，失败后增量已经不在 Redis。
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

// 排队等待写入 PostgreSQL 的一条花费日志。
type SpendLog struct {
	RequestID  string  `json:"request_id"`
	CallType   string  `json:"call_type"`
	Model      string  `json:"model"`
	APIKey     string  `json:"api_key"`
	Prompt     int     `json:"prompt_tokens"`
	Completion int     `json:"completion_tokens"`
	Spend      float64 `json:"spend"`
	SpendValid bool    `json:"spend_valid"`
	Start      string  `json:"start"`
	End        string  `json:"end"`
	CacheHit   bool    `json:"cache_hit"`
	Status     string  `json:"status"`
	TeamID     string  `json:"team_id"`
	UserID     string  `json:"user_id"`
	OrgID      string  `json:"org_id"`
}

// 把日志放进 Redis 列表。请求路径只做这一步。
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

// 从列表头部读出最多 n 条，不删除。返回解析后的行和原始字符串。
// PeekLogs reads up to n queued logs without removing them.
// raw is the queue prefix, including rows that do not decode.
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

// 在数据库事务成功后，同时确认花费增量和已经写出的日志前缀。
// AckFlushed subtracts hot spend and trims the log prefix in one Redis script.
// If the queue head is no longer the batch that was peeked, it does nothing, so a retry
// cannot subtract twice after a successful ack and cannot drop the logs first.
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

// 只丢掉日志队列头部 n 条。花费增量要另确认。
func (c *Client) AckLogs(n int) error {
	if c == nil || n < 1 {
		return nil
	}
	return c.rdb.LTrim(context.Background(), "xhub:spendlog", int64(n), -1).Err()
}

// 取出并删除最多 n 条日志。
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
