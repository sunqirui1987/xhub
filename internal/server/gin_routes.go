package server

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/sunqirui1987/xhub/internal/httpx"
	"github.com/sunqirui1987/xhub/internal/llm"
)

// handle 把一条「方法 + 路径」挂到 Gin。
//
// 路径里的 {param} 按段收成 :pN，同一位置的参数名一致，避免 Gin 的通配符冲突。
// 已经挂过的方法+模式直接跳过，因此专用 handler 先注册时不会被 catalog 覆盖。
func (s *Server) handle(pattern string, h http.HandlerFunc) {
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
		// 专用 handler 用 PathValue("key") 读路径参数。Gin 的参数名是段序号，这里写回原名。
		for i, name := range names {
			if name == "" {
				continue
			}
			if v := c.Param("p" + strconv.Itoa(i)); v != "" {
				c.Request.SetPathValue(name, v)
			}
		}
		h(c.Writer, c.Request)
	})
}

// mountCatalog 为 routes.json 里的每一条路由注册 Gin 模式。
// 未出现在这张表里的路径不会命中这些模式，由 NoRoute 返回 404。
func (s *Server) mountCatalog() {
	for _, rt := range s.catalog {
		if removedColumnRoute(rt.P) {
			continue
		}
		s.handle(rt.M+" "+rt.P, s.handlerFor(rt.P))
	}
}

// handlerFor 按 catalog 路径选择家族处理函数。
// 图像、重排、音频、审核、视频、responses、文件、realtime、透传都有自己的入口。
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

// imagesContract 是 Images 家族的 HTTP 契约。
// POST 生成与编辑进入数据面的 images / images_edits 操作，由协议编解码打上游。
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

// rerankContract 是 Rerank 家族的 HTTP 契约。
// POST /v1/rerank、/rerank、/v2/rerank 进入数据面，不再依赖 "/" 兜底才认得出操作。
func (s *Server) rerankContract(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost || r.Method == http.MethodPut {
		s.dataPlane(w, r, "rerank")
		return
	}
	s.serveFamilyRoute(w, r)
}

// audioContract 是 Audio 家族的 HTTP 契约：speech、transcriptions、translations。
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

// moderationsContract 是 Moderations 家族的 HTTP 契约。
func (s *Server) moderationsContract(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost || r.Method == http.MethodPut {
		s.dataPlane(w, r, "moderations")
		return
	}
	s.serveFamilyRoute(w, r)
}

// videosContract 是 Videos 家族的 HTTP 契约。
func (s *Server) videosContract(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost || r.Method == http.MethodPut {
		s.dataPlane(w, r, "videos")
		return
	}
	s.serveFamilyRoute(w, r)
}

// responsesContract 是 Responses 家族除已专用注册的两条主路径之外的别名。
func (s *Server) responsesContract(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost || r.Method == http.MethodPut {
		s.dataPlane(w, r, "responses")
		return
	}
	s.serveFamilyRoute(w, r)
}

// filesContract 是 Files 家族的 HTTP 契约。
// 创建、读取、删除都落在 SQLite 的 files 行上，响应是 file 对象而不是请求体回显。
func (s *Server) filesContract(w http.ResponseWriter, r *http.Request) {
	s.serveFamilyRoute(w, r)
}

// realtimeContract 是 Realtime 家族的 HTTP 契约。
// 客户端必须发起 WebSocket 升级。普通 POST/GET 得到 426，而不是一段占位 JSON。
func (s *Server) realtimeContract(w http.ResponseWriter, r *http.Request) {
	// client_secrets、calls 是普通 JSON。只有通道本身要求升级。
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

// passthroughContract 是供应原样转发前缀的 HTTP 契约。
// 前缀决定供应商。出站 URL 用 LiteLLM 的 _join_url_paths：接上剩余路径，OpenAI 补 /v1/。
// 这里只算出将要访问的地址，不向厂商拨号。
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

// passthroughAPIBase 是环境变量未设置时 LiteLLM 透传路由使用的官方根地址。
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
