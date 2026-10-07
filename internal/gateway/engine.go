// engine.go is the HTTP front door. Handler orders the steps; it does not
// contain inference or admin logic.
//
// Order for one request: recover and access log, call id, CORS, OPTIONS,
// idempotency replay, bypass match, then Gin. A streaming body skips the
// idempotency buffer and the response hold buffer because those would have
// to store the whole stream. Bypass runs before Gin so an official path is
// not swallowed by a catalog pattern that happens to share a prefix.

package gateway

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sunqirui1987/xhub/internal/auth"
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
		raw, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(raw))
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		if model := str(body["model"]); model != "" {
			w.Header().Set("x-litellm-model-name", model)
			w.Header().Set("x-litellm-model-id", model)
		}
		stream, _ := body["stream"].(bool)
		idemKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
		if idemKey != "" && !stream {
			ck := auth.APIKeyFrom(r) + "|" + r.Method + "|" + r.URL.Path + "|" + idemKey
			s.mu.Lock()
			hit, ok := s.idem[ck]
			s.mu.Unlock()
			if ok {
				logx.Debug("process %s %s step=idempotency hit=true", r.Method, r.URL.Path)
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
				return
			}
		}
		if s.serveBypass(lw, r) {
			return
		}
		if stream || strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
			logx.Debug("process %s %s step=dispatch mode=stream", r.Method, r.URL.Path)
			s.engine.ServeHTTP(w, r)
			return
		}
		logx.Debug("process %s %s step=dispatch mode=buffered", r.Method, r.URL.Path)
		hw := &holdWriter{ResponseWriter: w, code: 200}
		s.engine.ServeHTTP(hw, r)
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
		if idemKey != "" && code < 500 {
			ck := auth.APIKeyFrom(r) + "|" + r.Method + "|" + r.URL.Path + "|" + idemKey
			rec := idemRec{Code: code, CT: w.Header().Get("Content-Type"), Body: append([]byte(nil), hw.buf.Bytes()...)}
			rec.Hdr = map[string]string{}
			for _, k := range []string{"x-litellm-call-id", "x-litellm-version", "x-litellm-model-name", "x-litellm-model-id"} {
				if v := w.Header().Get(k); v != "" {
					rec.Hdr[k] = v
				}
			}
			s.mu.Lock()
			s.idem[ck] = rec
			s.mu.Unlock()
		}
	})
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
	return s.ResponseWriter.Write(p)
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
