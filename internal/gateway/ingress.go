// ingress.go mounts catalog paths onto Gin after the modules have mounted.
// handle is the only registration function. A method and pattern already in
// s.registered is left alone, which is why a module route beats a catalog
// route for the same path. Gin parameters are renamed to :pN so two templates
// that use different placeholder names at the same position do not conflict.
// The original name is written back with SetPathValue before the handler runs.
//
// handlerFor picks the inference family. The default is serveFamilyRoute,
// which enters the chat data plane. Paths that contain /images, /audio,
// /rerank, /videos, /responses, /files, or /realtime stay on their own
// handlers, and those handlers still record spend through the same data plane.

package gateway

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/sunqirui1987/xhub/internal/httpx"
	"github.com/sunqirui1987/xhub/internal/llm"
	"github.com/sunqirui1987/xhub/internal/logx"
	"sync"
)

var logTraceOnceIngress sync.Once

// handle mounts one "METHOD /path" pattern on Gin. A {param} in the path becomes :pN per segment. The same position keeps the same parameter name so Gin wildcards do not conflict. A method and pattern that is already mounted is skipped, so a dedicated handler registered first is not overwritten by the catalog.
// 参数 pattern（string）：要匹配的路径模板或正则；h（http.HandlerFunc）：要挂上的 HTTP 处理函数。
// 返回：无。这条 METHOD /path 已挂到 Gin。路径里的 {param} 按段变成 :pN。模式无效或路由已删除时不挂。
// 调用：gateway/routes.go
// 测试：无直接单测
func (s *Server) handle(pattern string, h http.HandlerFunc) {
	logTraceOnceIngress.Do(func() { logx.Trace("enter gateway.handle") })

	method, path, ok := strings.Cut(pattern, " ")
	if !ok || h == nil || removedColumnRoute(path) {
		return
	}
	gp, names := ginPathNames(path)
	key := method + " " + gp
	if _, exists := s.registered[key]; exists {
		return
	}
	s.registered[key] = struct{}{}
	s.engine.Handle(method, gp, func(c *gin.Context) {
		// A dedicated handler reads a path parameter with PathValue("key"). Gin names parameters by segment index, and this writes the original name back.
		for i, name := range names {
			if name == "" {
				continue
			}
			if v := c.Param("p" + strconv.Itoa(i)); v != "" {
				c.Request.SetPathValue(name, v)
			}
		}
		logx.Debug("process %s %s step=route pattern=%s", c.Request.Method, c.Request.URL.Path, pattern)
		h(c.Writer, c.Request)
	})
}

// mountCatalog registers a Gin pattern for every route in routes.json. A path that is not in this table does not match these patterns and gets a 404 from NoRoute.
// 参数：无。
// 调用：gateway/server.go
// 测试：无直接单测
// 返回：无。routes.json 里还在用的路由都挂上了 Gin 模式。不在这张表里的路径不会命中这些模式。
func (s *Server) mountCatalog() {
	for _, rt := range s.catalog {
		if removedColumnRoute(rt.P) {
			continue
		}
		s.handle(rt.M+" "+rt.P, s.handlerFor(rt.P))
	}
}

// handlerFor chooses the family handler for a catalog path. Images, rerank, audio, moderations, videos, responses, files, realtime, and passthrough each have their own entry.
// 参数 path（string）：handler为要定位的路径。可能是 URL，也可能是字段路径。
// 返回 http.HandlerFunc（http.HandlerFunc）：请求结束时要调用的释放函数。不需要释放时可能为 nil。
// 调用：仅在 ingress.go 内使用
// 测试：无直接单测
func (s *Server) handlerFor(path string) http.HandlerFunc {
	p := strings.ToLower(path)
	switch {
	case strings.Contains(p, "/images"):
		return s.imagesContract
	case strings.Contains(p, "/rerank"):
		return s.rerankContract
	case strings.Contains(p, "/audio"):
		return s.audioContract
	case strings.Contains(p, "/moderations"):
		return s.moderationsContract
	case strings.Contains(p, "/videos"):
		return s.videosContract
	case strings.Contains(p, "/responses"):
		return s.responsesContract
	case strings.Contains(p, "/files"):
		return s.filesContract
	case strings.Contains(p, "/realtime"), strings.Contains(p, "/vertex_ai/live"):
		return s.realtimeContract
	case passthroughPattern(p):
		return s.passthroughContract
	default:
		return s.serveFamilyRoute
	}
}

// passthroughPattern reports whether this catalog path is forwarded as-is instead of entering the inference data plane.
// 参数 path（string）：passthrough模板要定位的路径。可能是 URL，也可能是字段路径。
// 返回 bool（bool）：这条目录路径按原样转发给上游，而不进入推理数据面时返回真。
// 调用：仅在 ingress.go 内使用
// 测试：无直接单测
func passthroughPattern(path string) bool {
	switch {
	case strings.HasPrefix(path, "/openai/{"), strings.HasPrefix(path, "/openai_passthrough/"):
		return true
	case strings.HasPrefix(path, "/anthropic/"), strings.HasPrefix(path, "/bedrock/"),
		strings.HasPrefix(path, "/gemini/"), strings.HasPrefix(path, "/azure/"),
		strings.HasPrefix(path, "/vertex_ai/"):
		return true
	default:
		return false
	}
}

// ginPathNames turns a catalog template into a pattern Gin can register and lists the parameter names.
// 参数 p（string）：目录里的路径模板，花括号标出参数段。
// 返回 string（string）：Gin 能注册的模式，{name} 换成 :p0 这种段。空路径或 / 得到 "/"；[]string（[]string）：每个路径段抽出的参数名。没有花括号的段是空串，整条路径没有参数时为 nil。
// 调用：仅在 ingress.go 内使用
// 测试：无直接单测
func ginPathNames(p string) (string, []string) {
	if p == "" || p == "/" {
		return "/", nil
	}
	parts := strings.Split(p, "/")
	names := make([]string, len(parts))
	for i, seg := range parts {
		if !strings.Contains(seg, "{") {
			continue
		}
		name := seg
		if i := strings.IndexByte(name, '{'); i >= 0 {
			name = name[i+1:]
		}
		if j := strings.IndexByte(name, '}'); j >= 0 {
			name = name[:j]
		}
		if c := strings.IndexByte(name, ':'); c >= 0 {
			name = name[:c]
		}
		names[i] = name
		parts[i] = ":p" + strconv.Itoa(i)
	}
	out := strings.Join(parts, "/")
	if out == "" {
		out = "/"
	}
	return out, names
}

// imagesContract is the HTTP contract for the Images family. POST generate and edit enter the data-plane images and images_edits operations, and the protocol codec calls the upstream.
// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态码和正文写在这里；r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文。
// 调用：仅在 ingress.go 内使用
// 测试：无直接单测
// 返回：无。状态码和正文写进调用方的响应。
func (s *Server) imagesContract(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost || r.Method == http.MethodPut {
		op := "images"
		if strings.Contains(r.URL.Path, "/edits") {
			op = "images_edits"
		}
		s.dataPlane(w, r, op)
		return
	}
	s.serveFamilyRoute(w, r)
}

// rerankContract is the HTTP contract for the Rerank family. POST /v1/rerank, /rerank, and /v2/rerank enter the data plane. They do not depend on a "/" catch-all to recognize the operation.
// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态码和正文写在这里；r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文。
// 调用：仅在 ingress.go 内使用
// 测试：无直接单测
// 返回：无。状态码和正文写进调用方的响应。
func (s *Server) rerankContract(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost || r.Method == http.MethodPut {
		s.dataPlane(w, r, "rerank")
		return
	}
	s.serveFamilyRoute(w, r)
}

// audioContract is the HTTP contract for the Audio family: speech, transcriptions, and translations.
// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态码和正文写在这里；r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文。
// 调用：仅在 ingress.go 内使用
// 测试：无直接单测
// 返回：无。状态码和正文写进调用方的响应。
func (s *Server) audioContract(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost || r.Method == http.MethodPut {
		p := strings.ToLower(r.URL.Path)
		op := "audio_speech"
		switch {
		case strings.Contains(p, "/translations"):
			op = "audio_translation"
		case strings.Contains(p, "/transcriptions"):
			op = "audio_transcription"
		}
		s.dataPlane(w, r, op)
		return
	}
	s.serveFamilyRoute(w, r)
}

// moderationsContract is the HTTP contract for the Moderations family.
// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态码和正文写在这里；r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文。
// 调用：仅在 ingress.go 内使用
// 测试：无直接单测
// 返回：无。状态码和正文写进调用方的响应。
func (s *Server) moderationsContract(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost || r.Method == http.MethodPut {
		s.dataPlane(w, r, "moderations")
		return
	}
	s.serveFamilyRoute(w, r)
}

// videosContract is the HTTP contract for the Videos family.
// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态码和正文写在这里；r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文。
// 调用：仅在 ingress.go 内使用
// 测试：无直接单测
// 返回：无。状态码和正文写进调用方的响应。
func (s *Server) videosContract(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost || r.Method == http.MethodPut {
		s.dataPlane(w, r, "videos")
		return
	}
	s.serveFamilyRoute(w, r)
}

// responsesContract is the alias for the Responses family besides the two main paths that are registered separately.
// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态码和正文写在这里；r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文。
// 调用：仅在 ingress.go 内使用
// 测试：无直接单测
// 返回：无。状态码和正文写进调用方的响应。
func (s *Server) responsesContract(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost || r.Method == http.MethodPut {
		s.dataPlane(w, r, "responses")
		return
	}
	s.serveFamilyRoute(w, r)
}

// filesContract is the HTTP contract for the Files family. Create, read, and delete persist file rows. The response is a file object rather than an echo of the request body.
// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态码和正文写在这里；r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文。
// 调用：仅在 ingress.go 内使用
// 测试：无直接单测
// 返回：无。状态码和正文写进调用方的响应。
func (s *Server) filesContract(w http.ResponseWriter, r *http.Request) {
	s.serveFamilyRoute(w, r)
}

// realtimeContract is the HTTP contract for the Realtime family. The client must start a WebSocket upgrade. An ordinary POST or GET gets 426 instead of a placeholder JSON body.
// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态码和正文写在这里；r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文。
// 调用：仅在 ingress.go 内使用
// 测试：无直接单测
// 返回：无。状态码和正文写进调用方的响应。
func (s *Server) realtimeContract(w http.ResponseWriter, r *http.Request) {
	// client_secrets and calls are ordinary JSON. Only the channel itself requires an upgrade.
	if strings.Contains(r.URL.Path, "/realtime/") || strings.Contains(r.URL.Path, "/live/") {
		s.serveFamilyRoute(w, r)
		return
	}
	if s.requireLLMPrincipal(w, r) == nil {
		return
	}
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		w.Header().Set("Connection", "Upgrade")
		w.Header().Set("Upgrade", "websocket")
		httpx.WriteError(w, http.StatusUpgradeRequired, "upgrade_required", "realtime requires websocket")
		return
	}
	hj, ok := w.(http.Hijacker)
	if !ok {
		httpx.WriteError(w, http.StatusUpgradeRequired, "upgrade_required", "websocket upgrade is not available on this writer")
		return
	}
	conn, buf, err := hj.Hijack()
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "upgrade_failed", err.Error())
		return
	}
	defer conn.Close()
	_, _ = buf.WriteString("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n")
	_ = buf.Flush()
}

// passthroughContract is the HTTP contract for a provider prefix that is forwarded as-is. The prefix chooses the provider. The outbound URL uses LiteLLM _join_url_paths: the remaining path is joined, and OpenAI gains /v1/. This only computes the address that would be called. It does not dial the provider.
// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态码和正文写在这里；r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文。
// 调用：仅在 ingress.go 内使用
// 测试：无直接单测
// 返回：无。状态码和正文写进调用方的响应。
func (s *Server) passthroughContract(w http.ResponseWriter, r *http.Request) {
	if s.requireLLMPrincipal(w, r) == nil {
		return
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	provider := ""
	endpoint := ""
	if len(parts) > 0 {
		provider = parts[0]
	}
	if len(parts) > 1 {
		endpoint = "/" + strings.Join(parts[1:], "/")
	}
	base := passthroughAPIBase(provider)
	upstream := ""
	if base != "" {
		upstream = llm.PassthroughURL(base, endpoint, provider)
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"object":       "passthrough",
		"provider":     provider,
		"path":         r.URL.Path,
		"method":       r.Method,
		"upstream_url": upstream,
	})
}

// passthroughAPIBase is the official root LiteLLM passthrough routes use when the environment variable is unset.
// 参数 provider（string）：供应商标识，例如 openai 或 volcengine。
// 返回 string（string）：环境变量没设时这个供应商的官方根地址。openai 是 https://api.openai.com/，anthropic 是 https://api.anthropic.com。不认识时为空串。
// 调用：仅在 ingress.go 内使用
// 测试：无直接单测
func passthroughAPIBase(provider string) string {
	switch provider {
	case "openai", "openai_passthrough":
		return "https://api.openai.com/"
	case "anthropic":
		return "https://api.anthropic.com"
	case "gemini":
		return "https://generativelanguage.googleapis.com"
	case "cohere":
		return "https://api.cohere.com"
	default:
		return ""
	}
}

// chat is the chat-completions entry and forwards to the data-plane chat operation.
// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态码和正文写在这里；r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文。
// 调用：仅在 ingress.go 内使用
// 测试：无直接单测
// 返回：无。状态码和正文写进调用方的响应。
func (s *Server) chat(w http.ResponseWriter, r *http.Request) {
	logx.Debug("process %s %s step=gateway op=chat", r.Method, r.URL.Path)
	s.dataPlane(w, r, "chat")
}

// embeddings is the embeddings entry and forwards to the data-plane embeddings operation.
// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态码和正文写在这里；r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文。
// 调用：仅在 ingress.go 内使用
// 测试：无直接单测
// 返回：无。状态码和正文写进调用方的响应。
func (s *Server) embeddings(w http.ResponseWriter, r *http.Request) {
	logx.Debug("process %s %s step=gateway op=embeddings", r.Method, r.URL.Path)
	s.dataPlane(w, r, "embeddings")
}

// completions is the text-completions entry and forwards to the data plane.
// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态码和正文写在这里；r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文。
// 调用：仅在 ingress.go 内使用
// 测试：无直接单测
// 返回：无。状态码和正文写进调用方的响应。
func (s *Server) completions(w http.ResponseWriter, r *http.Request) {
	logx.Debug("process %s %s step=gateway op=completions", r.Method, r.URL.Path)
	s.dataPlane(w, r, "completions")
}

// messages is the Anthropic Messages entry and forwards to the data plane.
// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态码和正文写在这里；r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文。
// 调用：仅在 ingress.go 内使用
// 测试：无直接单测
// 返回：无。状态码和正文写进调用方的响应。
func (s *Server) messages(w http.ResponseWriter, r *http.Request) {
	logx.Debug("process %s %s step=gateway op=messages", r.Method, r.URL.Path)
	s.dataPlane(w, r, "messages")
}

// audioTranslations is the audio-translation entry and forwards to the data plane.
// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态码和正文写在这里；r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文。
// 调用：仅在 ingress.go 内使用
// 测试：无直接单测
// 返回：无。状态码和正文写进调用方的响应。
func (s *Server) audioTranslations(w http.ResponseWriter, r *http.Request) {
	s.dataPlane(w, r, "audio_translation")
}
