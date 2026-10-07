// Package llm decides which URL paths may stay on the gateway process. Management routes are not part of that list.
package llm

import (
	"github.com/sunqirui1987/xhub/internal/logx"
	"net/http"
	"sync"
)

// These lists come from gateway/routes/allowlist.py.
// The gateway process keeps the model data plane: chat, embeddings, audio, files, batches, fine-tuning, responses,
// rerank, retrieval, video, search, the realtime channel, MCP, plus /health and /metrics.
// Management APIs and the dashboard UI are not served by this same filter.
//
// A broad /v1/ prefix cannot be used. It would also admit management routes such as /v1/access_group.
var pathPrefixes = []string{
	"/v1/chat/",
	"/chat/",
	"/v1/completions",
	"/completions",
	"/v1/embeddings",
	"/embeddings",
	"/v1/moderations",
	"/moderations",
	"/v1/audio/",
	"/audio/",
	"/v1/images/",
	"/images/",
	"/v1/files",
	"/files",
	"/v1/batches",
	"/batches",
	"/v1/fine_tuning/",
	"/fine_tuning/",
	"/v1/fine-tuning/",
	"/fine-tuning/",
	"/v1/responses",
	"/responses",
	"/v1/threads",
	"/threads",
	"/v1/assistants",
	"/assistants",
	"/v1/vector_stores",
	"/vector_stores",
	"/v1/indexes",
	"/v1/models",
	"/models",
	"/openai/",
	"/engines/",
	"/v1/messages",
	"/messages",
	"/v1/skills",
	"/v1/a2a/",
	"/a2a/",
	"/v1/rerank",
	"/v2/rerank",
	"/rerank",
	"/v1/ocr",
	"/ocr",
	"/v1/rag/",
	"/rag/",
	"/v1/video",
	"/v1/videos",
	"/video/",
	"/videos",
	"/v1/search",
	"/search",
	"/v1/containers",
	"/containers",
	"/v1/evals",
	"/v1/memory",
	"/queue/chat/",
	"/v1beta/",
	"/interactions",
	"/anthropic/",
	"/azure/",
	"/azure_ai/",
	"/aws/",
	"/bedrock/",
	"/comprehendmedical",
	"/cohere/",
	"/gemini/",
	"/gigachat/",
	"/google/",
	"/vertex_ai/",
	"/vertex-ai/",
	"/assemblyai/",
	"/eu.assemblyai/",
	"/langfuse/",
	"/vllm/",
	"/mistral/",
	"/groq/",
	"/voyage/",
	"/cursor/",
	"/milvus/",
	"/openai_passthrough/",
	"/{provider}/",
	"/toolset/",
	"/v1/realtime",
	"/realtime",
	"/health",
	"/metrics",
	"/watsonx",
}

var exactPaths = map[string]struct{}{
	"/":                     {},
	"/routes":               {},
	"/openapi.json":         {},
	"/docs":                 {},
	"/docs/oauth2-redirect": {},
	"/redoc":                {},
	"/test":                 {},
	"/debug/memory/summary": {},
}

// mountPaths allows only the Prometheus /metrics mount.
// The dashboard static directory is also a mount, but it is not in this table, so it is dropped.
var mountPaths = map[string]struct{}{
	"/metrics": {},
}

var logTraceOnceAllow sync.Once

// PathPrefixes returns the prefixes in the allowlist.py order so a test can compare them with the Python source.
// 参数：无。
// 返回 []string（[]string）：路径Prefixes。没有匹配时为空切片。
// 调用：仅在 allow.go 内使用
// 测试：无直接单测
func PathPrefixes() []string {
	logTraceOnceAllow.Do(func() { logx.Trace("enter llm.PathPrefixes") })

	out := make([]string, len(pathPrefixes))
	copy(out, pathPrefixes)
	return out
}

// Allow reports whether a path may stay on the gateway process. When mount is true only the mount table is checked, matching a FastAPI Mount. An ordinary route checks an exact path first, then a prefix.
//
//	An empty path is not a data-plane path. The comparison uses the path from route registration, not a URL with a query string.
//
// 参数 path（string）：允许要定位的路径。可能是 URL，也可能是字段路径；mount（bool）：为真时走挂载这一支。为假时保持原来的路径。
// 返回 bool（bool）：这条路径可以留在网关进程里处理时返回真。mount 为真时只查挂载表。
// 调用：仅在 allow.go 内使用
// 测试：无直接单测
func Allow(path string, mount bool) bool {
	if path == "" {
		return false
	}
	if mount {
		_, ok := mountPaths[path]
		return ok
	}
	if _, ok := exactPaths[path]; ok {
		return true
	}
	for _, prefix := range pathPrefixes {
		if len(path) >= len(prefix) && path[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}

// Filter drops management routes before the process serves them. The Python gateway trims the route table after startup, once registration is finished. Here the request has already reached a registeredhandler, and the same list decides whether to allow it or return 404. The effect matches: the data plane stays, and management routes such as /key/generate are not served by this filter.
// 参数 next（http.Handler）：过滤使用的Handler。
// 返回 http.Handler（http.Handler）：接住该路径的处理函数。
// 调用：仅在 allow.go 内使用
// 测试：无直接单测
func Filter(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path == "" {
			path = "/"
		}
		if !Allow(path, false) {
			http.NotFound(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}
