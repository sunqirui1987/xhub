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

var logTraceOnceRedis sync.Once

// Open parses the Redis URL and pings it. On failure it closes the client and returns the error.
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
func (c *Client) Close() error {
	if c == nil || c.rdb == nil {
		return nil
	}
	return c.rdb.Close()
}

// RecordFailure increments the failure count. After allowed failures it writes the cooldown key. An allowed below 1 does nothing.
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
func minuteBucket() int64 { return time.Now().Unix() / 60 }

// HitRPM increments the current minute's request count and returns the new value.
func (c *Client) HitRPM(id string) (int64, error) {
	return c.bump("xhub:rpm:"+id, 1)
}

// HitTPM adds tokens to the current minute and returns the new value.
func (c *Client) HitTPM(id string, tokens int) (int64, error) {
	return c.bump("xhub:tpm:"+id, int64(tokens))
}

// bump adds n to a minute counter and returns the new value. The key includes the minute number, so the next minute starts at zero after expiry.
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
	RequestID    string  `json:"request_id"`
	CallType     string  `json:"call_type"`
	Model        string  `json:"model"`
	APIKey       string  `json:"api_key"`
	Prompt       int     `json:"prompt_tokens"`
	Completion   int     `json:"completion_tokens"`
	Spend        float64 `json:"spend"`
	SpendValid   bool    `json:"spend_valid"`
	Start        string  `json:"start"`
	End          string  `json:"end"`
	CacheHit     bool    `json:"cache_hit"`
	Status       string  `json:"status"`
	TeamID       string  `json:"team_id"`
	UserID       string  `json:"user_id"`
	OrgID        string  `json:"org_id"`
	Messages     string  `json:"messages,omitempty"`
	Response     string  `json:"response,omitempty"`
	ProxyRequest string  `json:"proxy_server_request,omitempty"`
}

// EnqueueLog pushes a log onto the Redis list. The request path does only this step.
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

// PeekLogs reads up to n items from the head of the list and does not delete them. It returns the parsed rows and the raw strings.
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

// AckFlushed acknowledges spend deltas and the written log prefix together after the database transaction succeeds.
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

// AckLogs drops only the first n log-queue items. Spend deltas must be acknowledged separately.
func (c *Client) AckLogs(n int) error {
	if c == nil || n < 1 {
		return nil
	}
	return c.rdb.LTrim(context.Background(), "xhub:spendlog", int64(n), -1).Err()
}

// DrainLogs reads and deletes up to n logs.
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
