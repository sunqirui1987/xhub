// Package gateway builds the Gin engine, CORS, and the idempotency buffer. Business handlers are not written in this file.
package gateway

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sunqirui1987/xhub/internal/auth"

	"github.com/sunqirui1987/xhub/internal/httpx"
)

// newEngine creates the Gin engine. An unregistered path returns a JSON 404 instead of Gin's plain text.
func newEngine() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	e := gin.New()
	e.NoRoute(gin.WrapF(func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteError(w, http.StatusNotFound, "not_found", "Not Found")
	}))
	return e
}

// setCORS copies the request Origin into the CORS headers. A request without Origin gets no allow headers.
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

// SetUIProxy replaces the dashboard reverse proxy. Nil means the console is not running.
func (s *Server) SetUIProxy(h http.Handler) { s.uiProxy = h }

// GinRoutes returns the methods, paths, and handlers mounted on the engine, so a caller can confirm there is no "/" catch-all.
func (s *Server) GinRoutes() gin.RoutesInfo { return s.engine.Routes() }

// Handler is the HTTP entry. It applies CORS and the call ID, then hands the request to Gin. OPTIONS returns 204.
func (s *Server) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		lw := &statusRecorder{ResponseWriter: w, code: http.StatusOK}
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("error panic %s %s %v", r.Method, r.URL.Path, rec)
				if !lw.set {
					lw.code = http.StatusInternalServerError
					lw.set = true
					lw.ResponseWriter.WriteHeader(http.StatusInternalServerError)
				}
			}
			// Method, path, status, and elapsed time only. Headers and bodies stay off this line.
			log.Printf("%s %s %d %s", r.Method, r.URL.Path, lw.code, time.Since(start))
			if lw.code >= 400 {
				note := lw.note
				if note == "" {
					note = "request failed"
				}
				log.Printf("error %s %s %d %s", r.Method, r.URL.Path, lw.code, redactLog(note))
			}
		}()
		w = lw
		callID := httpx.CallID()
		httpx.SetCallID(w, callID)
		w.Header().Set("x-litellm-version", Version)
		setCORS(w, r)
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if Try(s.uiProxy, w, r) {
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
				if hit.Code >= 400 {
					lw.note = errorNote(hit.Body)
				}
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

var (
	bearerValue = regexp.MustCompile(`(?i)bearer\s+\S+`)
	secretValue = regexp.MustCompile(`sk-[A-Za-z0-9_\-]+`)
)

// redactLog removes bearer tokens and sk- keys from a log line. The method and path stay.
func redactLog(s string) string {
	s = bearerValue.ReplaceAllString(s, "Bearer ***")
	return secretValue.ReplaceAllString(s, "sk-***")
}

// errorNote reads the error type and message from a JSON error body. A non-JSON body is shortened.
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

// WriteHeader records the first status and forwards it. A later call does not replace that status.
func (s *statusRecorder) WriteHeader(code int) {
	if !s.set {
		s.code = code
		s.set = true
		s.ResponseWriter.WriteHeader(code)
	}
}

// Write records status 200 when the handler writes a body without a status.
func (s *statusRecorder) Write(p []byte) (int, error) {
	if !s.set {
		s.code = http.StatusOK
		s.set = true
	}
	return s.ResponseWriter.Write(p)
}

// Flush forwards to the underlying writer when it can flush a streamed response.
func (s *statusRecorder) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Hijack forwards connection takeover for websocket upgrades.
func (s *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := s.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("response writer does not support hijack")
	}
	return h.Hijack()
}

// Unwrap returns the writer underneath so http.ResponseController can reach the connection.
func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }

type holdWriter struct {
	http.ResponseWriter
	code int
	buf  bytes.Buffer
	hdr  bool
}

// WriteHeader delays the status so middleware can still add response headers.
func (h *holdWriter) WriteHeader(c int) {
	if !h.hdr {
		h.code = c
		h.hdr = true
	}
}

// Write buffers the body. The bytes reach the ResponseWriter only after Finish.
func (h *holdWriter) Write(p []byte) (int, error) {
	if !h.hdr {
		h.code = 200
		h.hdr = true
	}
	return h.buf.Write(p)
}
