// 判断模型名是否落在密钥或实体的允许列表里。空列表表示不限制。
package llm

import "net/http"

// 这些名单来自 gateway/routes/allowlist.py。
// 网关进程只保留模型数据面：聊天、向量、音频、文件、批次、微调、responses、
// 重排、检索、视频、搜索、实时通道、MCP，以及 /health 和 /metrics。
// 管理接口和界面不走同一条进程。
//
// 不能用笼统的 /v1/ 前缀。那样会把 /v1/access_group 这类管理路由也放进来。
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

// mountPaths 只放行 Prometheus 的 /metrics 挂载。
// 界面的静态目录也是挂载，但不在这张表里，所以会被丢掉。
var mountPaths = map[string]struct{}{
	"/metrics": {},
}

// PathPrefixes 按 allowlist.py 里的顺序返回前缀。测试用它和 Python 源码逐项比较。
func PathPrefixes() []string {
	out := make([]string, len(pathPrefixes))
	copy(out, pathPrefixes)
	return out
}

// Allow 判断一条路径能不能留在网关进程上。
//
// mount 为真时只看挂载表，对应 FastAPI 的 Mount。普通路由先看精确路径，
// 再看前缀。path 为空时不算数据面。比较用的是路由注册时的 path，不是带查询串的 URL。
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

// Filter 在进程对外服务之前丢掉管理路由。
//
// Python 网关是在应用启动、路由注册完成之后再裁剪路由表。这里请求已经走到
// 注册好的处理器上，用同一份名单决定放行还是 404，效果相同：数据面留下，
// /key/generate 这类管理接口不出现在网关进程上。
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
