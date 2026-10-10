// engine.go is the HTTP front door. Handler orders the steps; it does not
// contain inference or admin logic.
//
// Order for one request: recover and access log, call id, CORS, OPTIONS,
// body read, authenticated idempotency, bypass match, then Gin. A streaming body skips the
// idempotency buffer and the response hold buffer because those would have
// to store the whole stream. Bypass runs before Gin so an official path is
// not swallowed by a catalog pattern that happens to share a prefix.

package gateway

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sunqirui1987/xhub/internal/authz"
	"github.com/sunqirui1987/xhub/internal/httpx"
	"github.com/sunqirui1987/xhub/internal/logx"
)

// newEngine creates the Gin engine. An unregistered path returns a JSON 404 instead of Gin's plain text.
// 参数：无。
// 调用：gateway/server.go
// 测试：access_log_test.go、console_split_test.go、dial_log_test.go
// 返回 *gin.Engine（*gin.Engine）：发布模式的 Gin 引擎。未注册路径返回 JSON 404，不用 Gin 的纯文本。
func newEngine() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	e := gin.New()
	e.NoRoute(gin.WrapF(func(w http.ResponseWriter, r *http.Request) {
		logx.Debug("process %s %s step=noroute", r.Method, r.URL.Path)
		httpx.WriteError(w, http.StatusNotFound, "not_found", "Not Found")
	}))
	return e
}

// setCORS copies the request Origin into the CORS headers. A request without Origin gets no allow headers.
// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态码和正文写在这里；r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文。
// 调用：仅在 engine.go 内使用
// 测试：无直接单测
// 返回：无。状态码和正文写进调用方的响应。
func setCORS(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return
	}
	h := w.Header()
	h.Set("Access-Control-Allow-Origin", origin)
	h.Set("Access-Control-Allow-Credentials", "true")
	// Echo the preflight list. The playground OpenAI SDK sends x-stainless-*
	// and Chrome client hints; a fixed allow-list fails that preflight.
	allow := r.Header.Get("Access-Control-Request-Headers")
	if allow == "" {
		allow = "Authorization, Content-Type, x-litellm-api-key, Idempotency-Key, x-litellm-tags, x-litellm-end-user-id"
	}
	h.Set("Access-Control-Allow-Headers", allow)
	h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
	h.Set("Access-Control-Expose-Headers", "x-litellm-call-id, x-litellm-response-cost, x-litellm-model-name, Retry-After, x-litellm-cache-key, cache_hit, x-litellm-cache-hit, x-litellm-version, x-litellm-response-duration-ms")
	h.Set("Access-Control-Max-Age", "600")
	// 127.0.0.1:3000 calling localhost:4000 is a private-network request in Chrome.
	h.Set("Access-Control-Allow-Private-Network", "true")
	h.Add("Vary", "Origin")
	h.Add("Vary", "Access-Control-Request-Headers")
}

// GinRoutes returns the methods, paths, and handlers mounted on the engine, so a caller can confirm there is no "/" catch-all.
// 参数：无。
// 返回 RoutesInfo（gin.RoutesInfo）：引擎上已挂的方法、路径和处理函数，用来确认没有 "/" 兜底路由。
// 调用：仅在 engine.go 内使用
// 测试：无直接单测
func (s *Server) GinRoutes() gin.RoutesInfo { return s.engine.Routes() }

// Handler is the process HTTP entry. Tests mount this, not the Gin engine. Bypass that returns true has already written the response. Gin then either streams straight through or buffers a non-stream response so the idempotency key can replay it.
// 参数：无。
// 调用：gateway/server.go
// 测试：access_log_test.go、activity_http_test.go、builtin_providers_test.go
// 返回 http.Handler（http.Handler）：接住该路径的处理函数。
func (s *Server) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		var raw []byte
		var body map[string]any
		lw := &statusRecorder{ResponseWriter: w, code: http.StatusOK}
		logx.Trace("process %s %s step=enter", r.Method, r.URL.Path)
		defer func() {
			if rec := recover(); rec != nil {
				logx.Error("panic %s %s %v", r.Method, r.URL.Path, rec)
				if !lw.set {
					lw.code = http.StatusInternalServerError
					lw.set = true
					lw.ResponseWriter.WriteHeader(http.StatusInternalServerError)
				}
			}
			// Method, path, status, and elapsed time only. Headers and bodies stay off this line.
			logx.Info("%s %s %d %s", r.Method, r.URL.Path, lw.code, time.Since(start))
			if lw.code >= 400 {
				s.recordEarlyError(lw, r, raw, body, start)
				note := lw.note
				if note == "" {
					note = "request failed"
				}
				logx.Error("%s %s %d %s", r.Method, r.URL.Path, lw.code, note)
			}
		}()
		w = lw
		callID := httpx.CallID()
		httpx.SetCallID(w, callID)
		w.Header().Set("x-litellm-version", Version)
		setCORS(w, r)
		if r.Method == http.MethodOptions {
			logx.Debug("process %s %s step=options", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		var readErr error
		raw, readErr = io.ReadAll(http.MaxBytesReader(w, r.Body, 64<<20))
		if readErr != nil {
			httpx.WriteTypedError(w, r.URL.Path, http.StatusRequestEntityTooLarge, "invalid_request", "request body exceeds limit or cannot be read")
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(raw))
		_ = json.Unmarshal(raw, &body)
		if model := str(body["model"]); model != "" {
			w.Header().Set("x-litellm-model-name", model)
			w.Header().Set("x-litellm-model-id", model)
		}
		stream, _ := body["stream"].(bool)
		// 图片编辑可以用 multipart 提交流式请求。必须在幂等响应缓冲之前识别，
		// 否则 SSE 会被完整缓存后才返回，失去流式行为。这里只读控制字段，不读文件。
		if media, params, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err == nil && media == "multipart/form-data" {
			parts := multipart.NewReader(bytes.NewReader(raw), params["boundary"])
			for {
				part, err := parts.NextPart()
				if err != nil {
					break
				}
				if part.FormName() == "stream" && part.FileName() == "" {
					value, _ := io.ReadAll(io.LimitReader(part, 16))
					stream = strings.TrimSpace(string(value)) == "true"
				}
				part.Close()
			}
		}
		idemKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
		var lease *idempotencyLease
		if idemKey != "" && !stream {
			var hit *idemRec
			var conflict bool
			lease, hit, conflict = s.beginIdempotency(r, idemKey, raw)
			if conflict {
				httpx.WriteTypedError(w, r.URL.Path, http.StatusConflict, "idempotency_conflict", "Idempotency-Key was already used with a different request payload")
				return
			}
			if hit != nil {
				logx.Debug("process %s %s step=idempotency hit=true", r.Method, r.URL.Path)
				s.writeIdempotentResponse(w, lw, hit)
				return
			}
		}
		if lease != nil {
			defer lease.abort()
		}
		if lease == nil && s.serveBypass(lw, r) {
			return
		}
		if stream || strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
			logx.Debug("process %s %s step=dispatch mode=stream", r.Method, r.URL.Path)
			s.engine.ServeHTTP(w, r)
			return
		}
		logx.Debug("process %s %s step=dispatch mode=buffered", r.Method, r.URL.Path)
		hw := &holdWriter{ResponseWriter: w, code: 200}
		if !s.serveBypass(hw, r) {
			s.engine.ServeHTTP(hw, r)
		}
		if w.Header().Get("Content-Type") == "" {
			w.Header().Set("Content-Type", "application/json")
		}
		if isDataPlanePath(r.URL.Path) {
			w.Header().Set("x-litellm-response-duration-ms", strconv.FormatInt(time.Since(start).Milliseconds(), 10))
			if w.Header().Get("x-litellm-model-api-base") == "" {
				if p, err := s.resolve(r); err == nil && p != nil && p.Key != nil {
					s.setChatHeaders(w, p, w.Header().Get("x-litellm-model-name"), w.Header().Get("x-litellm-model-api-base"))
				}
			}
		}
		code := hw.code
		if !hw.hdr {
			code = 200
		}
		if code >= 400 {
			lw.note = errorNote(hw.buf.Bytes())
		}
		w.WriteHeader(code)
		_, _ = w.Write(hw.buf.Bytes())
		if lease != nil && code < 500 {
			rec := idemRec{Code: code, CT: w.Header().Get("Content-Type"), Body: append([]byte(nil), hw.buf.Bytes()...)}
			rec.Hdr = map[string]string{}
			for _, k := range []string{"x-litellm-call-id", "x-litellm-version", "x-litellm-model-name", "x-litellm-model-id"} {
				if v := w.Header().Get(k); v != "" {
					rec.Hdr[k] = v
				}
			}
			lease.complete(&rec)
		}
	})
}

type idempotencyLease struct {
	server *Server
	key    string
	rec    *idemRec
	done   bool
}

// beginIdempotency authenticates the caller and either owns, replays, or rejects an idempotent operation.
// 参数 r（*http.Request）：带当前凭证、方法、路径和查询串的请求；key（string）：调用方提供的幂等键；body（[]byte）：用于请求指纹的原始正文。
// 返回 lease（*idempotencyLease）：首次请求的执行所有权；replay（*idemRec）：已完成响应或等待取消响应；conflict（bool）：同键请求指纹不一致。
// 调用：Handler 在非流式请求进入 bypass 或 Gin 前调用。
// 测试：TestIdempotencyScopesHashesAndExpires、TestIdempotencyConcurrentRequestWaitsForOwner 和 TestIdempotencyReplaysWithoutASecondCharge。
func (s *Server) beginIdempotency(r *http.Request, key string, body []byte) (*idempotencyLease, *idemRec, bool) {
	p, err := s.resolve(r)
	if err != nil || p == nil {
		return nil, nil, false
	}
	namespace := ""
	switch p.Kind {
	case authz.KindKey:
		namespace = "key:" + p.KeyID
	case authz.KindSession:
		sessionHash := sha256.Sum256([]byte(p.Session))
		namespace = "session:" + p.UserID + ":" + stringHex(sessionHash[:])
	default:
		return nil, nil, false
	}
	cacheKey := namespace + "|" + r.Method + "|" + r.URL.Path + "|" + key
	hashInput := append([]byte(r.URL.RawQuery), 0)
	hashInput = append(hashInput, body...)
	hash := sha256.Sum256(hashInput)
	for {
		now := time.Now()
		if s.now != nil {
			now = s.now()
		}
		ttl := s.idemTTL
		if ttl <= 0 {
			ttl = defaultIdempotencyTTL
		}
		s.mu.Lock()
		for expiredKey, expired := range s.idem {
			if expired.Done == nil && !now.Before(expired.ExpiresAt) {
				delete(s.idem, expiredKey)
			}
		}
		rec := s.idem[cacheKey]
		if rec == nil {
			rec = &idemRec{RequestHash: hash, ExpiresAt: now.Add(ttl), Done: make(chan struct{})}
			s.idem[cacheKey] = rec
			s.mu.Unlock()
			return &idempotencyLease{server: s, key: cacheKey, rec: rec}, nil, false
		}
		if rec.RequestHash != hash {
			s.mu.Unlock()
			return nil, nil, true
		}
		done := rec.Done
		if done == nil {
			hit := cloneIdemRec(rec)
			s.mu.Unlock()
			return nil, hit, false
		}
		s.mu.Unlock()
		select {
		case <-done:
		case <-r.Context().Done():
			return nil, &idemRec{Code: http.StatusRequestTimeout, CT: "application/json", Body: []byte("{\"error\":{\"message\":\"request cancelled while waiting for the original idempotent request\",\"type\":\"request_cancelled\"}}")}, false
		}
	}
}

// stringHex encodes bytes as lowercase hexadecimal for secret-free session namespaces.
// 参数 raw（[]byte）：要编码的摘要字节。
// 返回 string（string）：每字节两个字符的小写十六进制文本。
// 调用：beginIdempotency 编码会话凭证的 SHA-256 摘要。
// 测试：TestIdempotencyScopesHashesAndExpires 间接验证命名空间不包含原始会话凭证。
func stringHex(raw []byte) string {
	const digits = "0123456789abcdef"
	out := make([]byte, len(raw)*2)
	for i, b := range raw {
		out[i*2] = digits[b>>4]
		out[i*2+1] = digits[b&15]
	}
	return string(out)
}

// complete publishes a successful idempotent response and wakes duplicate requests.
// 参数 response（*idemRec）：要保存的状态码、响应头和响应正文。
// 返回：无；接收者对应的占位记录原地变为已完成记录。
// 调用：Handler 在非 5xx 缓冲响应写出后调用。
// 测试：TestIdempotencyScopesHashesAndExpires 和 TestIdempotencyConcurrentRequestWaitsForOwner。
func (l *idempotencyLease) complete(response *idemRec) {
	if l == nil || l.done {
		return
	}
	l.server.mu.Lock()
	if l.server.idem[l.key] == l.rec {
		l.rec.Code, l.rec.CT, l.rec.Body, l.rec.Hdr = response.Code, response.CT, response.Body, response.Hdr
		close(l.rec.Done)
		l.rec.Done = nil
	}
	l.server.mu.Unlock()
	l.done = true
}

// abort releases an unfinished idempotency claim so a later request can retry it.
// 参数：无；使用接收者保存的服务器、缓存键和占位记录。
// 返回：无；仍由本 lease 持有时删除占位并唤醒等待者。
// 调用：Handler 延迟调用；complete 已提交时该调用为空操作。
// 测试：TestIdempotencyScopesHashesAndExpires 覆盖释放后的重新占有。
func (l *idempotencyLease) abort() {
	if l == nil || l.done {
		return
	}
	l.server.mu.Lock()
	if l.server.idem[l.key] == l.rec {
		delete(l.server.idem, l.key)
		close(l.rec.Done)
	}
	l.server.mu.Unlock()
	l.done = true
}

// cloneIdemRec copies a completed response before releasing the idempotency lock.
// 参数 rec（*idemRec）：锁保护下的已完成幂等记录。
// 返回 *idemRec：正文和响应头均独立复制的响应快照。
// 调用：beginIdempotency 在缓存命中时调用。
// 测试：TestIdempotencyScopesHashesAndExpires 和 TestIdempotencyConcurrentRequestWaitsForOwner 间接覆盖。
func cloneIdemRec(rec *idemRec) *idemRec {
	return &idemRec{Code: rec.Code, CT: rec.CT, Body: append([]byte(nil), rec.Body...), Hdr: cloneStringMap(rec.Hdr)}
}

// cloneStringMap returns an independent copy of a string response-header map.
// 参数 in（map[string]string）：可能为空的原始响应头映射。
// 返回 map[string]string：可由调用方独立修改的新映射。
// 调用：cloneIdemRec 复制缓存响应头时调用。
// 测试：幂等重放测试通过响应快照间接覆盖。
func cloneStringMap(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// writeIdempotentResponse writes a cached status, selected headers, and body to the client.
// 参数 w（http.ResponseWriter）：客户端响应；lw（*statusRecorder）：访问日志状态记录器；hit（*idemRec）：缓存响应快照。
// 返回：无；响应已写入 w，错误响应同时更新 lw 的错误摘要。
// 调用：Handler 在 beginIdempotency 返回缓存命中时调用。
// 测试：TestIdempotencyReplaysWithoutASecondCharge 通过完整入口重放覆盖。
func (s *Server) writeIdempotentResponse(w http.ResponseWriter, lw *statusRecorder, hit *idemRec) {
	for k, v := range hit.Hdr {
		w.Header().Set(k, v)
	}
	if hit.CT != "" {
		w.Header().Set("Content-Type", hit.CT)
	}
	w.WriteHeader(hit.Code)
	_, _ = w.Write(hit.Body)
	if hit.Code >= 400 {
		lw.note = errorNote(hit.Body)
	}
}

// errorNote reads the error type and message from a JSON error body. A non-JSON body is shortened.
// 参数 body（[]byte）：原始正文。可能是 JSON，也可能是 SSE，由调用方按内容解析。
// 返回 string（string）：JSON 错误体里的 type 和 message。不是 JSON 时取正文前 180 个字符。
// 调用：仅在 engine.go 内使用
// 测试：无直接单测
func errorNote(body []byte) string {
	var env struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &env) == nil && (env.Error.Message != "" || env.Error.Type != "") {
		return strings.TrimSpace(env.Error.Type + " " + env.Error.Message)
	}
	s := strings.TrimSpace(string(body))
	if len(s) > 180 {
		s = s[:180]
	}
	return s
}

// statusRecorder records the status written to the client so the access log can print it.
type statusRecorder struct {
	http.ResponseWriter
	code int
	set  bool
	note string
	// body 只收集失败响应，供统一早期错误日志保存客户端实际收到的完整正文。
	body bytes.Buffer
}

// WriteHeader 记下第一次写出的状态码，并转给底层 ResponseWriter。之后的 WriteHeader 不再改状态。
// 参数 code（int）：要写给调用方的 HTTP 状态码。
// 返回：无。状态码已经通过底层 WriteHeader 写进响应。
// 调用：net/http 在处理函数写出状态码时。
// 测试：无直接单测
func (s *statusRecorder) WriteHeader(code int) {
	if !s.set {
		s.code = code
		s.set = true
		s.ResponseWriter.WriteHeader(code)
	}
}

// Write records status 200 when the handler writes a body without a status.
// 参数 p（[]byte）：要写给调用方的正文。
// 返回 int（int）：实际写出的字节数，等于底层 Write 的结果；error（error）：底层写出失败的原因。nil 表示这段正文已经写出。还没写过状态码时先记成 200。
// 调用：net/http 在处理函数写出正文时。
// 测试：bypass_logic_test.go、guardrail_block_test.go
func (s *statusRecorder) Write(p []byte) (int, error) {
	if !s.set {
		s.code = http.StatusOK
		s.set = true
	}
	if s.code >= 400 {
		_, _ = s.body.Write(p)
	}
	return s.ResponseWriter.Write(p)
}

// recordEarlyError 为受支持的数据面尚未进入常规结算路径的失败补写日志。
// 参数 lw：最终状态和客户端正文；r/raw/body：原请求及解析结果；start：请求开始时间。
// 返回：无。已删除或退役接口仅保留进程访问日志；已有 RecordSpend 行依赖 request_id 幂等去重；鉴权失败无法认领身份时仅管理员可见。
// 调用：Handler 的失败 defer，覆盖鉴权、限流、未认领模型和路由配置错误；测试见 early_error_log_test.go。
func (s *Server) recordEarlyError(lw *statusRecorder, r *http.Request, raw []byte, body map[string]any, start time.Time) {
	if s == nil || lw == nil || lw.code < 400 || !isDataPlanePath(r.URL.Path) {
		return
	}
	// 兼容路由仍会返回 410，已删除路由会返回 404；它们不再提供模型服务，
	// 不能因为历史数据面前缀而写入模型请求日志、用量统计或 Redis 花费队列。
	if IsRemovedColumn(r.URL.Path) || IsRetiredPath(r.URL.Path) {
		return
	}
	callID := strings.TrimSpace(lw.Header().Get("x-litellm-call-id"))
	if callID == "" {
		return
	}
	alias := strings.TrimSpace(lw.Header().Get("x-litellm-model-name"))
	if alias == "" {
		alias = strings.TrimSpace(str(body["model"]))
	}
	p, _ := s.resolve(r)
	// 使用实际响应正文，而不是重新构造错误包络，确保日志与调用方看到的字节一致。
	s.rememberExchange(callID, r, raw, lw.body.Bytes())
	s.recordSpend(lw, p, callID, alias, "request", nil, start, false, lw.code, "")
}

// Flush 在底层支持刷新时，把已经缓冲的字节推给调用方。
// 参数：无。
// 调用：net/http 在流式响应需要刷新时。
// 测试：无直接单测
// 返回：无。底层实现了 http.Flusher 时才会真正刷新。
func (s *statusRecorder) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Hijack 把连接从 HTTP 服务里交出去，给需要升级的协议使用。
// 参数：无。
// 返回 Conn（net.Conn）：接管后的连接。底层不支持时为 nil；ReadWriter（*bufio.ReadWriter）：这条连接上的缓冲读写器。不支持时为 nil；error（error）：底层不能接管时的原因。nil 表示已经交给底层。
// 调用：net/http 在升级连接时。
// 测试：无直接单测
func (s *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := s.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("response writer does not support hijack")
	}
	return h.Hijack()
}

// Unwrap 返回包在里面的原始响应，让 ResponseController 能找到连接。
// 参数：无。
// 返回 http.ResponseWriter（http.ResponseWriter）：包在 statusRecorder 里面的原始响应。
// 调用：net/http 的 ResponseController 在需要到底层连接时。
// 测试：无直接单测
func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }

type holdWriter struct {
	http.ResponseWriter
	code int
	buf  bytes.Buffer
	hdr  bool
}

// WriteHeader 先把状态码留在缓冲里，让中间件还能补响应头。Finish 才真正写出。
// 参数 c（int）：处理函数要写的 HTTP 状态码。这一步只记下，不交给底层。
// 返回：无。状态码留在缓冲里，Finish 才写给调用方。
// 调用：net/http 在处理函数写出状态码时。
// 测试：failure_log_test.go、guard_test.go
func (h *holdWriter) WriteHeader(c int) {
	if !h.hdr {
		h.code = c
		h.hdr = true
	}
}

// Write buffers the body. The bytes reach the ResponseWriter only after Finish.
// 参数 p（[]byte）：先放进缓冲的正文，Finish 才交给底层。
// 返回 int（int）：写进缓冲区的字节数；error（error）：缓冲写入失败的原因。nil 表示已经放进缓冲。还没记过状态码时先记成 200。
// 调用：net/http 在处理函数写出正文时。
// 测试：bypass_logic_test.go、guardrail_block_test.go
func (h *holdWriter) Write(p []byte) (int, error) {
	if !h.hdr {
		h.code = 200
		h.hdr = true
	}
	return h.buf.Write(p)
}
