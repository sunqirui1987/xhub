// Gin 引擎、CORS 和幂等缓冲。业务处理函数不写在这里。
package gateway

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sunqirui1987/xhub/internal/auth"
	"github.com/sunqirui1987/xhub/internal/gateway/ui"
	"github.com/sunqirui1987/xhub/internal/httpx"
)

// 创建 Gin 引擎。未注册路径返回 JSON 404，而不是 Gin 的纯文本。
func newEngine() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	e := gin.New()
	e.NoRoute(gin.WrapF(func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteError(w, http.StatusNotFound, "not_found", "Not Found")
	}))
	return e
}

// 按请求的 Origin 回写 CORS。没有 Origin 时不添加允许头。
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

// SetUIProxy 换成另一个控制台反向代理。传 nil 表示控制台没有启动。
func (s *Server) SetUIProxy(h http.Handler) { s.uiProxy = h }

// GinRoutes 返回已经挂到引擎上的方法、路径和处理器，测试用来确认没有 "/" 兜底。
func (s *Server) GinRoutes() gin.RoutesInfo { return s.engine.Routes() }

// HTTP 入口。处理 CORS、调用 ID，再交给 Gin。OPTIONS 直接 204。
func (s *Server) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		callID := httpx.CallID()
		httpx.SetCallID(w, callID)
		w.Header().Set("x-litellm-version", Version)
		setCORS(w, r)
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if ui.Try(s.uiProxy, w, r) {
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
				for k, v := range hit.Hdr {
					w.Header().Set(k, v)
				}
				if hit.CT != "" {
					w.Header().Set("Content-Type", hit.CT)
				}
				w.WriteHeader(hit.Code)
				_, _ = w.Write(hit.Body)
				return
			}
		}
		if stream || strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
			s.engine.ServeHTTP(w, r)
			return
		}
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

type holdWriter struct {
	http.ResponseWriter
	code int
	buf  bytes.Buffer
	hdr  bool
}

// 延迟写出状态码，让中间件还能补响应头。
func (h *holdWriter) WriteHeader(c int) {
	if !h.hdr {
		h.code = c
		h.hdr = true
	}
}

// 先缓冲正文。调用方 Finish 之后才真正写到 ResponseWriter。
func (h *holdWriter) Write(p []byte) (int, error) {
	if !h.hdr {
		h.code = 200
		h.hdr = true
	}
	return h.buf.Write(p)
}
