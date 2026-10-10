// Package fixture 为基准测试创建真实网关、独立 PostgreSQL schema 和本地 OpenAI 上游。
package fixture

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/gateway"
	"github.com/sunqirui1987/xhub/internal/iam"
	"github.com/sunqirui1987/xhub/internal/store"
)

const Model = "benchmark-chat"
const DefaultDatabaseURL = "postgres://xhub:xhub_dev_password@127.0.0.1:5433/xhub?sslmode=disable"

// Environment 只持有本次运行的资源；Schema 用于追踪异常终止后残留，Key 不写进报告。
type Environment struct {
	URL      string
	Key      string
	Schema   string
	DB       *iam.DB
	server   *httptest.Server
	upstream *httptest.Server
	store    *store.Store
	root     *pgx.Conn
	calls    atomic.Int64
}

// Open 创建隔离环境并通过真实登录/发钥接口准备流量；参数为数据库地址与上游延迟。
// 返回资源句柄或错误；失败时立即回收，成功后调用方必须 Close。仅使用进程内限流，不连接共享 Redis。
func Open(ctx context.Context, dsn string, delay time.Duration) (_ *Environment, err error) {
	if delay < 0 {
		return nil, errors.New("上游延迟不能为负数")
	}
	u, err := url.Parse(dsn)
	if err != nil || u.Host == "" || (u.Scheme != "postgres" && u.Scheme != "postgresql") {
		return nil, errors.New("数据库地址必须为 PostgreSQL URL")
	}
	e := &Environment{}
	defer func() {
		if err != nil {
			err = errors.Join(err, e.Close())
		}
	}()
	connectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	e.root, err = pgx.Connect(connectCtx, dsn)
	if err != nil {
		return nil, errors.New("PostgreSQL 不可达，请配置 XHUB_TEST_DATABASE_URL 或启动本地数据库")
	}
	var random [16]byte
	if _, err = rand.Read(random[:]); err != nil {
		return nil, err
	}
	e.Schema = "benchmark_" + hex.EncodeToString(random[:])
	if _, err = e.root.Exec(ctx, "CREATE SCHEMA "+pgx.Identifier{e.Schema}.Sanitize()); err != nil {
		e.Schema = ""
		return nil, errors.New("无法创建基准测试 schema")
	}
	q := u.Query()
	q.Set("search_path", e.Schema)
	u.RawQuery = q.Encode()
	e.DB, err = iam.Open(ctx, u.String())
	if err != nil {
		return nil, errors.New("无法初始化隔离身份库")
	}
	e.store, err = store.Open(u.String())
	if err != nil {
		return nil, errors.New("无法初始化隔离模型库")
	}
	e.upstream = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { e.serveUpstream(w, r, delay) }))
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		ModelList:       []config.ModelEntry{{ModelName: Model, LiteLLMParams: map[string]any{"model": Model, "api_base": e.upstream.URL + "/v1", "api_key": "sk-local-benchmark", "custom_llm_provider": "openai", "input_cost_per_token": 0.000002, "output_cost_per_token": 0.000008}, ModelInfo: map[string]any{"transport": "bypass_openai_chat", "endpoint_types": []string{"chat"}}}},
		RouterSettings:  config.RouterSettings{RoutingStrategy: "simple-shuffle", NumRetries: 0, Timeout: 30},
		GeneralRaw:      map[string]any{},
		GeneralSettings: config.GeneralSettings{DatabaseURL: u.String(), MasterKey: "sk-benchmark-" + hex.EncodeToString(random[:]), StorePromptsInSpendLogs: true},
	}
	if _, err = e.DB.EnsureAdmin(ctx, "benchmark@example.invalid", "Benchmark", "benchmark-local-password"); err != nil {
		return nil, err
	}
	gw := gateway.New(cfg, e.store, e.DB)
	e.server = httptest.NewServer(gw.Handler())
	e.URL = e.server.URL
	login, err := JSON(ctx, e.URL, "/v2/login", "", map[string]any{"username": "benchmark@example.invalid", "password": "benchmark-local-password"})
	if err != nil {
		return nil, err
	}
	token, _ := login["token"].(string)
	if token == "" {
		return nil, errors.New("登录没有返回会话")
	}
	key, err := JSON(ctx, e.URL, "/key/generate", token, map[string]any{"key_alias": "benchmark-personal"})
	if err != nil {
		return nil, err
	}
	e.Key, _ = key["key"].(string)
	if e.Key == "" {
		return nil, errors.New("发钥接口没有返回密钥")
	}
	return e, nil
}

// Close 停止 HTTP、关闭连接并删除本次 schema；无参数，返回聚合清理错误，可重复调用。
// 清理用独立超时上下文，确保 SIGINT 取消运行后仍能删除数据；绝不修改 public 或共享 Redis。
func (e *Environment) Close() error {
	if e.server != nil {
		e.server.Close()
		e.server = nil
	}
	if e.upstream != nil {
		e.upstream.Close()
		e.upstream = nil
	}
	var err error
	if e.store != nil && e.store.Engine != nil {
		err = errors.Join(err, e.store.Engine.Close())
		e.store = nil
	}
	if e.DB != nil {
		err = errors.Join(err, e.DB.Close())
		e.DB = nil
	}
	if e.root != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if e.Schema != "" {
			_, dropErr := e.root.Exec(ctx, "DROP SCHEMA "+pgx.Identifier{e.Schema}.Sanitize()+" CASCADE")
			err = errors.Join(err, dropErr)
		}
		err = errors.Join(err, e.root.Close(ctx))
		e.root = nil
	}
	return err
}

// Audit 核对真实上游调用、持久化用量和费用；无参数，返回本次环境计数和查询错误。
// 调用方在发压结束后调用，流式写入可能稍晚于客户端 EOF，最多等待五秒。
func (e *Environment) Audit() (map[string]any, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// 过载可能使数据库查询本身失败，仍保留已知调用量，不能把审计失败写成空结果。
	result := map[string]any{"upstream_calls": e.calls.Load(), "consistent": false, "audit_complete": false}
	for {
		var rows []struct {
			Count  int64   `xorm:"'count'"`
			Tokens int64   `xorm:"'tokens'"`
			Cost   float64 `xorm:"'cost'"`
		}
		err := e.DB.Engine.Context(ctx).SQL("SELECT COUNT(*) AS count, COALESCE(SUM(prompt_tokens + completion_tokens), 0) AS tokens, COALESCE(SUM(cost), 0) AS cost FROM usage_events WHERE model = ? AND status = 'success'", Model).Find(&rows)
		if err != nil {
			result["audit_complete"] = false
			return result, err
		}
		if len(rows) != 1 {
			return result, errors.New("用量统计未返回结果")
		}
		row := rows[0]
		calls := e.calls.Load()
		// 固定输入/输出价格与 11/5 token 一起核对，避免只计数而漏掉费用回归。
		expectedCost := float64(calls) * 0.000062
		result = map[string]any{"upstream_calls": calls, "persisted_success": row.Count, "total_tokens": row.Tokens, "cost_usd": row.Cost, "expected_cost_usd": expectedCost, "audit_complete": true, "consistent": row.Count == calls && row.Tokens == calls*16 && math.Abs(row.Cost-expectedCost) <= 1e-9*math.Max(1, expectedCost)}
		if row.Count == calls {
			return result, nil
		}
		select {
		case <-ctx.Done():
			result["audit_complete"] = false
			return result, ctx.Err()
		case <-time.After(20 * time.Millisecond):
		}
	}
}

// JSON 通过真实 POST 准备或清理夹具；参数为上下文、目标、路径、会话和正文，返回对象或脱敏错误。
// 客户端禁用重定向且限时十秒；错误不回显含密钥的响应正文。
func JSON(ctx context.Context, base, path, key string, body any) (map[string]any, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(base, "/")+path, bytes.NewReader(raw))
	if err != nil {
		return nil, errors.New("构造夹具请求失败")
	}
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	client := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.New("夹具 HTTP 请求失败")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("夹具接口 %s 返回 HTTP %d", path, resp.StatusCode)
	}
	var doc map[string]any
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1048576)).Decode(&doc); err != nil {
		return nil, err
	}
	return doc, nil
}

// serveUpstream 生成固定 usage 的本地对话和完整 SSE；参数为响应、请求和延迟，返回无。
// 只接受 chat 路径并计数，延迟尊重取消；上游回 11/5 token 以验证计量完整性。
func (e *Environment) serveUpstream(w http.ResponseWriter, r *http.Request, delay time.Duration) {
	if r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" {
		http.NotFound(w, r)
		return
	}
	var body struct {
		Stream bool `json:"stream"`
	}
	if json.NewDecoder(io.LimitReader(r.Body, 2*1048576)).Decode(&body) != nil {
		http.Error(w, "invalid request", 400)
		return
	}
	e.calls.Add(1)
	if delay > 0 {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-r.Context().Done():
			return
		case <-timer.C:
		}
	}
	usage := map[string]any{"prompt_tokens": 11, "completion_tokens": 5, "total_tokens": 16}
	if !body.Stream {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "chatcmpl-benchmark", "object": "chat.completion", "model": Model, "choices": []any{map[string]any{"index": 0, "finish_reason": "stop", "message": map[string]any{"role": "assistant", "content": "benchmark-ok"}}}, "usage": usage})
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	for _, content := range []string{"benchmark-", "ok"} {
		chunk, _ := json.Marshal(map[string]any{"id": "chatcmpl-benchmark", "object": "chat.completion.chunk", "model": Model, "choices": []any{map[string]any{"index": 0, "delta": map[string]any{"content": content}, "finish_reason": nil}}})
		_, _ = fmt.Fprintf(w, "data: %s\n\n", chunk)
		w.(http.Flusher).Flush()
	}
	chunk, _ := json.Marshal(map[string]any{"id": "chatcmpl-benchmark", "object": "chat.completion.chunk", "model": Model, "choices": []any{map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": "stop"}}, "usage": usage})
	_, _ = fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", chunk)
	w.(http.Flusher).Flush()
}
