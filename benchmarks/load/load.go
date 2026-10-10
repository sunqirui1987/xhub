// Package load 提供有并发上限的真实 HTTP 压测；调用方负责目标环境与数据清理。
package load

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Config 是单阶段参数；Key 只用于请求，不进入报告。Duration 限制发压窗口，Timeout 限制单请求。
type Config struct {
	BaseURL     string
	Key         string
	Model       string
	Scenario    string
	Concurrency int
	Requests    int
	Duration    time.Duration
	Timeout     time.Duration
	PromptBytes int
}

// Distribution 是毫秒延迟分布；空样本返回零值，分位数采用 nearest-rank。
type Distribution struct {
	Samples int     `json:"samples"`
	Mean    float64 `json:"mean_ms"`
	P50     float64 `json:"p50_ms"`
	P95     float64 `json:"p95_ms"`
	P99     float64 `json:"p99_ms"`
	Max     float64 `json:"max_ms"`
}

// Result 记录一个阶段的已完成请求；预期的 401 算鉴权场景成功，仍单独保留状态码。
type Result struct {
	Scenario    string         `json:"scenario"`
	Concurrency int            `json:"concurrency"`
	Attempted   int            `json:"attempted"`
	Succeeded   int            `json:"succeeded"`
	Failed      int            `json:"failed"`
	Elapsed     float64        `json:"elapsed_seconds"`
	Dispatch    float64        `json:"dispatch_seconds"`
	RPS         float64        `json:"requests_per_second"`
	SuccessRPS  float64        `json:"successful_requests_per_second"`
	ErrorRate   float64        `json:"error_rate"`
	Bytes       int64          `json:"response_bytes"`
	Latency     Distribution   `json:"latency"`
	TTFT        Distribution   `json:"ttft"`
	Statuses    map[int]int    `json:"http_statuses"`
	Errors      map[string]int `json:"errors"`
}

// Validate 检查 CLI 与测试共用参数；参数 c 为单阶段配置，返回可展示错误，无网络副作用。
func (c Config) Validate() error {
	u, err := url.Parse(c.BaseURL)
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("base URL 必须是无凭据、查询参数和片段的 HTTP(S) 地址")
	}
	if c.Scenario != "chat" && c.Scenario != "stream" && c.Scenario != "models" && c.Scenario != "auth-reject" {
		return fmt.Errorf("不支持的场景 %q", c.Scenario)
	}
	if c.Concurrency < 1 || c.Concurrency > 4096 || c.Requests < 1 || c.Requests > 1000000 || c.Duration <= 0 || c.Timeout <= 0 || c.PromptBytes < 1 || c.PromptBytes > 1048576 {
		return errors.New("要求并发 1..4096、请求 1..1000000、正时长/超时、正文大小 1..1048576")
	}
	if strings.TrimSpace(c.Key) == "" || strings.TrimSpace(c.Model) == "" {
		return errors.New("必须提供 API key 和模型")
	}
	return nil
}

// Summarize 将毫秒样本复制排序，返回精确均值与分位数；调用方的切片不会被修改。
func Summarize(values []float64) Distribution {
	if len(values) == 0 {
		return Distribution{}
	}
	v := append([]float64(nil), values...)
	sort.Float64s(v)
	sum := 0.0
	for _, x := range v {
		sum += x
	}
	return Distribution{len(v), sum / float64(len(v)), v[int(math.Ceil(float64(len(v))*0.50))-1], v[int(math.Ceil(float64(len(v))*0.95))-1], v[int(math.Ceil(float64(len(v))*0.99))-1], v[len(v)-1]}
}

// Run 使用固定工作者闭环发压；ctx 中止后停止发新请求，并等待在途请求结束。
// 参数 c 为配置；返回完整统计或参数错误。内存最多保留 Requests 个延迟，不重试，目标会产生日志/用量。
func Run(ctx context.Context, c Config) (Result, error) {
	if err := c.Validate(); err != nil {
		return Result{}, err
	}
	transport := &http.Transport{Proxy: http.ProxyFromEnvironment, MaxIdleConns: c.Concurrency, MaxIdleConnsPerHost: c.Concurrency, MaxConnsPerHost: c.Concurrency, IdleConnTimeout: 30 * time.Second}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: c.Timeout, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	result := Result{Scenario: c.Scenario, Concurrency: c.Concurrency, Statuses: map[int]int{}, Errors: map[string]int{}}
	latencies := make([]float64, 0, min(c.Requests, 10000))
	ttfts := make([]float64, 0, min(c.Requests, 10000))
	var sequence atomic.Int64
	var mu sync.Mutex
	var wg sync.WaitGroup
	start := time.Now()
	deadline := start.Add(c.Duration)
	lastDispatch := start
	runID := fmt.Sprintf("%d", start.UnixNano())
	for worker := 0; worker < c.Concurrency; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				if ctx.Err() != nil || !time.Now().Before(deadline) {
					return
				}
				id := sequence.Add(1)
				if id > int64(c.Requests) {
					return
				}
				dispatched := time.Now()
				status, size, ttft, failure := request(ctx, client, c, fmt.Sprintf("%s-%d", runID, id))
				latency := float64(time.Since(dispatched)) / float64(time.Millisecond)
				mu.Lock()
				if dispatched.After(lastDispatch) {
					lastDispatch = dispatched
				}
				result.Attempted++
				result.Bytes += size
				result.Statuses[status]++
				latencies = append(latencies, latency)
				if failure == "" {
					result.Succeeded++
					if ttft > 0 {
						ttfts = append(ttfts, ttft)
					}
				} else {
					result.Failed++
					result.Errors[failure]++
				}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	result.Elapsed = time.Since(start).Seconds()
	result.Dispatch = lastDispatch.Sub(start).Seconds()
	result.RPS = float64(result.Attempted) / result.Elapsed
	result.SuccessRPS = float64(result.Succeeded) / result.Elapsed
	if result.Attempted > 0 {
		result.ErrorRate = float64(result.Failed) / float64(result.Attempted)
	}
	result.Latency = Summarize(latencies)
	result.TTFT = Summarize(ttfts)
	return result, nil
}

// request 执行并校验单次真实 HTTP；唯一正文防止缓存误当推理吞吐，密钥不会写进错误。
// 参数 ctx/client/c/id 分别为取消上下文、连接池、配置和请求标记；返回状态、字节、首字毫秒、固定错误类别。
func request(ctx context.Context, client *http.Client, c Config, id string) (int, int64, float64, string) {
	method, path, key := http.MethodPost, "/v1/chat/completions", c.Key
	var raw []byte
	if c.Scenario == "models" {
		method, path = http.MethodGet, "/v1/models"
	} else {
		body := map[string]any{"model": c.Model, "messages": []any{map[string]any{"role": "user", "content": id + " " + strings.Repeat("x", c.PromptBytes)}}, "stream": c.Scenario == "stream", "max_tokens": 32}
		// OpenAI 只在流式请求接受 stream_options，普通请求保持协议兼容。
		if c.Scenario == "stream" {
			body["stream_options"] = map[string]any{"include_usage": true}
		}
		raw, _ = json.Marshal(body)
	}
	if c.Scenario == "auth-reject" {
		key = "sk-benchmark-invalid-" + id
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.BaseURL, "/")+path, bytes.NewReader(raw))
	if err != nil {
		return 0, 0, 0, "request_build"
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return 0, 0, 0, "cancelled"
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return 0, 0, 0, "timeout"
		}
		return 0, 0, 0, "transport"
	}
	defer resp.Body.Close()
	if c.Scenario == "stream" && resp.StatusCode == 200 {
		n, ttft, err := readStream(resp.Body, start)
		if err != nil {
			return resp.StatusCode, n, 0, "invalid_stream"
		}
		return resp.StatusCode, n, ttft, ""
	}
	// 响应限长，避免错误页面或异常上游让压测进程无限分配内存。
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024+1))
	if err != nil {
		return resp.StatusCode, int64(len(body)), 0, "read_body"
	}
	if len(body) > 4*1024*1024 {
		return resp.StatusCode, int64(len(body)), 0, "response_too_large"
	}
	expected := http.StatusOK
	if c.Scenario == "auth-reject" {
		expected = http.StatusUnauthorized
	}
	if resp.StatusCode != expected {
		return resp.StatusCode, int64(len(body)), 0, "http_status"
	}
	if err := validateJSON(body, c.Scenario, c.Model); err != nil {
		return resp.StatusCode, int64(len(body)), 0, "invalid_response"
	}
	return resp.StatusCode, int64(len(body)), 0, ""
}

// validateJSON 校验可观察的协议结果；参数为正文、场景和模型，返回结构错误，无副作用。
func validateJSON(body []byte, scenario, model string) error {
	var doc struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
		Error json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return err
	}
	switch scenario {
	case "auth-reject":
		if len(doc.Error) > 0 && string(doc.Error) != "null" {
			return nil
		}
	case "models":
		for _, item := range doc.Data {
			if item.ID == model {
				return nil
			}
		}
	default:
		if len(doc.Choices) > 0 && doc.Choices[0].Message.Content != "" {
			return nil
		}
	}
	return errors.New("缺少预期业务结果")
}

// readStream 逐行读取 SSE；参数为响应流和起点，返回字节、首个 content 的毫秒和校验错误。
// 调用场景为流式压测；要求非空内容和 [DONE]，不把角色、心跳或 EOF 当作完整回答。
func readStream(body io.Reader, start time.Time) (int64, float64, error) {
	// 用原始读取量而非去除换行后的字符数限长，兼容 CRLF 并正确统计实际字节。
	limited := &io.LimitedReader{R: body, N: 4*1024*1024 + 1}
	scanner := bufio.NewScanner(limited)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	var size int64
	var ttft float64
	done := false
	for scanner.Scan() {
		line := scanner.Text()
		size = 4*1024*1024 + 1 - limited.N
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			if done {
				return size, ttft, errors.New("重复流结束标记")
			}
			done = true
			continue
		}
		if done {
			return size, ttft, errors.New("流结束后仍有数据")
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
			Error json.RawMessage `json:"error"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return size, ttft, err
		}
		if len(chunk.Error) > 0 && string(chunk.Error) != "null" {
			return size, ttft, errors.New("流内错误")
		}
		for _, choice := range chunk.Choices {
			if choice.Delta.Content != "" && ttft == 0 {
				ttft = float64(time.Since(start)) / float64(time.Millisecond)
			}
		}
	}
	size = 4*1024*1024 + 1 - limited.N
	if err := scanner.Err(); err != nil {
		return size, ttft, err
	}
	if !done || ttft == 0 || size > 4*1024*1024 {
		return size, ttft, errors.New("流未完成、内容为空或超长")
	}
	return size, ttft, nil
}
