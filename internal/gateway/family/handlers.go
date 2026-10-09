// Package family builds management and inference response shapes by LiteLLM resource family.
package family

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/sunqirui1987/xhub/internal/catalog"
	"github.com/sunqirui1987/xhub/internal/dataplane"
	"github.com/sunqirui1987/xhub/internal/gateway/guard"
	"github.com/sunqirui1987/xhub/internal/httpx"
	"github.com/sunqirui1987/xhub/internal/logx"
	"sync"
)

var logTraceOnceHandlers sync.Once

// ServeDataPlane handles inference paths in the catalog. A recognized operation enters the data plane. The rest are resource reads and writes.
// 参数 s（Host）：处理数据Plane使用的数据面宿主；w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；r（*http.Request）：入站 HTTP 请求。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：gateway/catalog.go
// 测试：无直接单测
func ServeDataPlane(s Host, w http.ResponseWriter, r *http.Request) {
	logTraceOnceHandlers.Do(func() { logx.Trace("enter family.ServeDataPlane") })

	p := s.RequireLLM(w, r)
	if p == nil {
		return
	}
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		httpx.WriteError(w, 400, "invalid_request", "cannot read request body")
		return
	}
	path := r.URL.Path
	op := inferenceOp(path)
	if op != "" && (r.Method == http.MethodPost || r.Method == http.MethodPut) {
		raw = injectModel(path, raw)
		r.Body = io.NopCloser(bytes.NewReader(raw))
		switch op {
		case "chat", "completions", "embeddings", "messages",
			"images", "images_edits", "audio_speech", "audio_transcription", "audio_translation",
			"moderations", "rerank", "responses", "videos", "gemini":
			s.DataPlane(w, r, op)
			return
		default:
			var body map[string]any
			_ = json.Unmarshal(raw, &body)
			if body == nil {
				body = map[string]any{}
			}
			if !s.EnforceIdentityLimits(w, path, p, str(body["model"]), dataplane.EstimateTokens(body)) {
				return
			}
			writeInferenceNative(w, op, body)
			return
		}
	}
	caller := "user:" + p.UserID
	if p.Hash != "" {
		caller = "key:" + p.Hash
	}
	if p.Hash == "" && p.UserID == "" {
		httpx.WriteError(w, 403, "forbidden", "resource owner required")
		return
	}
	sum := sha256.Sum256([]byte(caller))
	resourceCRUD(s, w, r, path, raw, hex.EncodeToString(sum[:]))
}

// Responses is the Responses API entry and always uses the data-plane responses operation.
// 参数 s（Host）：Responses使用的数据面宿主；w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；r（*http.Request）：入站 HTTP 请求。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：gateway/family/mount.go
// 测试：无直接单测
func Responses(s Host, w http.ResponseWriter, r *http.Request) {
	s.DataPlane(w, r, "responses")
}

// injectModel fills model from the engines, deployments, or models segment of the URL when the body has none.
// 参数 path（string）：inject模型要定位的路径。可能是 URL，也可能是字段路径；raw（[]byte）：原始正文。可能是 JSON，也可能是 SSE，由调用方按内容解析。
// 返回 []byte（[]byte）：序列化后的 JSON 字节。失败时为 nil。
// 调用：仅在 handlers.go 内使用
// 测试：无直接单测
func injectModel(path string, raw []byte) []byte {
	var body map[string]any
	if len(raw) > 0 {
		if json.Unmarshal(raw, &body) != nil || body == nil {
			return raw
		}
	}
	if body == nil {
		body = map[string]any{}
	}
	if str(body["model"]) == "" {
		if m := modelFromPath(path); m != "" {
			body["model"] = m
		}
	}
	out, err := json.Marshal(body)
	if err != nil {
		return raw
	}
	return out
}

// modelFromPath returns the model or deployment name from the path. With none of those segments it returns an empty string.
// 参数 path（string）：URL 路径，用来匹配路由或选择错误包络。
// 返回 string（string）：路径里 engines、deployments 或 models 下一段的模型名。没有这些段时为空串。
// 调用：仅在 handlers.go 内使用
// 测试：无直接单测
func modelFromPath(path string) string {
	parts := catalog.Split(strings.TrimSuffix(path, "/"))
	for i, p := range parts {
		if (p == "engines" || p == "deployments" || p == "models") && i+1 < len(parts) {
			m := parts[i+1]
			if j := strings.Index(m, ":"); j > 0 {
				m = m[:j]
			}
			return m
		}
	}
	return ""
}

// inferenceOp classifies a path as a data-plane operation name. An unrecognized path returns an empty string and must not be treated as chat.
// 参数 path（string）：inference操作要定位的路径。可能是 URL，也可能是字段路径。
// 返回 string（string）：路径对应的数据面操作名，例如 chat 或 embedding。认不出时为空串，不能当成 chat。
// 调用：仅在 handlers.go 内使用
// 测试：无直接单测
func inferenceOp(path string) string {
	p := strings.ToLower(path)
	switch {
	case strings.Contains(p, "/chat/completions"):
		return "chat"
	case strings.Contains(p, "/embeddings"):
		return "embeddings"
	case strings.Contains(p, "/completions") && !strings.Contains(p, "chat"):
		return "completions"
	case strings.Contains(p, "/threads"):
		return ""
	case strings.Contains(p, "/messages/count_tokens"):
		return "count_tokens"
	case strings.Contains(p, "/messages"):
		return "messages"
	case strings.Contains(p, "/responses"):
		return "responses"
	case strings.Contains(p, "/images") && strings.Contains(p, "/edits"):
		return "images_edits"
	case strings.Contains(p, "/images"):
		return "images"
	case strings.Contains(p, "/audio/speech"):
		return "audio_speech"
	case strings.Contains(p, "/audio/translations"):
		return "audio_translation"
	case strings.Contains(p, "/audio/transcriptions"):
		return "audio_transcription"
	case strings.Contains(p, "/moderations"):
		return "moderations"
	case strings.Contains(p, "/rerank"):
		return "rerank"
	case strings.Contains(p, "generatecontent"), strings.Contains(p, "counttokens"):
		return "gemini"
	case strings.Contains(p, "/realtime"), strings.Contains(p, "/live"):
		return "realtime"
	case strings.Contains(p, "/videos"):
		return "videos"
	}
	return ""
}

// writeInferenceNative rejects operations without a real provider implementation.
// 参数 w、op、body：响应、操作及请求。返回：HTTP 501。
// 调用：ServeDataPlane。测试：TestUnsupportedInferenceDoesNotFabricateUsage。
func writeInferenceNative(w http.ResponseWriter, op string, body map[string]any) {
	httpx.WriteError(w, http.StatusNotImplemented, "not_implemented", op+" requires a provider implementation")
}

// resourceCRUD lists, reads, and writes a catalog resource stored as key-value JSON. files and batches return 404 when the id is missing.
// 参数 s（Host）：resourceCRUD使用的数据面宿主；w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；r（*http.Request）：入站 HTTP 请求；path（string）：resourceCRUD要定位的路径。可能是 URL，也可能是字段路径；raw（[]byte）：原始文本或 JSON 字节。
// 参数 owner（string）：已经哈希的调用方标识，隔离资源命名空间。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：仅在 handlers.go 内使用
// 测试：无直接单测
func resourceCRUD(s Host, w http.ResponseWriter, r *http.Request, path string, raw []byte, owner string) {
	kind := resourceKind(path)
	namespace := "inference_resource:v2:" + owner + ":" + kind
	var body map[string]any
	if len(bytes.TrimSpace(raw)) > 0 && (json.Unmarshal(raw, &body) != nil || body == nil) {
		httpx.WriteError(w, 400, "invalid_request", "JSON object required")
		return
	}
	if body == nil {
		body = map[string]any{}
	}
	id := resourcePathID(path)
	if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodDelete && kind != "assistants" && kind != "threads" {
		httpx.WriteError(w, 501, "not_implemented", kind+" requires a provider implementation")
		return
	}
	parts := catalog.Split(strings.TrimSuffix(path, "/"))
	if kind == "threads" {
		for i, part := range parts {
			if part == "threads" && len(parts) > i+2 {
				httpx.WriteError(w, 501, "not_implemented", "nested thread operations require a provider implementation")
				return
			}
		}
	}
	st := s.RecordStore()
	if st == nil {
		httpx.WriteError(w, 503, "unavailable", "resource store unavailable")
		return
	}
	writeErr := func(err error) {
		if errors.Is(err, sql.ErrNoRows) {
			httpx.WriteError(w, 404, "not_found", singular(kind)+" not found")
			return
		}
		logx.Error("resource store failure kind=%s err=%v", kind, err)
		httpx.WriteError(w, 500, "internal", "resource store failure")
	}
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		if id != "" {
			m, err := st.GetKV(namespace, id)
			if err != nil {
				writeErr(err)
				return
			}
			if strings.HasSuffix(path, "/content") {
				httpx.WriteError(w, 501, "not_implemented", "file content requires provider storage")
				return
			}
			httpx.WriteJSON(w, 200, m)
			return
		}
		list, err := st.ListKV(namespace)
		if err != nil {
			writeErr(err)
			return
		}
		httpx.WriteJSON(w, 200, map[string]any{"object": "list", "data": list})
	case http.MethodDelete:
		if id == "" {
			id = str(body["id"])
		}
		if _, err := st.GetKV(namespace, id); err != nil {
			writeErr(err)
			return
		}
		if err := st.DeleteKV(namespace, id); err != nil {
			writeErr(err)
			return
		}
		httpx.WriteJSON(w, 200, map[string]any{"id": id, "object": singular(kind), "deleted": true})
	default:
		var obj map[string]any
		if id != "" {
			var err error
			obj, err = st.GetKV(namespace, id)
			if err != nil {
				writeErr(err)
				return
			}
			for k, v := range body {
				if k != "id" {
					obj[k] = v
				}
			}
			if strings.Contains(path, "cancel") {
				obj["status"] = "cancelled"
			}
		} else {
			obj = nativeResource(kind, body)
			id = str(obj["id"])
		}
		b, err := json.Marshal(obj)
		if err != nil {
			httpx.WriteError(w, 400, "invalid_request", "invalid resource")
			return
		}
		if err := st.PutKV(namespace, id, string(b)); err != nil {
			writeErr(err)
			return
		}
		httpx.WriteJSON(w, 200, obj)
	}
}

// resourceKind identifies the resource collection in the request path.
// 参数 path：请求路径。
// 返回：集合名。
// 调用：resourceCRUD。
// 测试：resource_isolation_test.go。
func resourceKind(path string) string {
	p := strings.ToLower(path)
	switch {
	case strings.Contains(p, "/files"):
		return "files"
	case strings.Contains(p, "/batches"):
		return "batches"
	case strings.Contains(p, "/assistants"):
		return "assistants"
	case strings.Contains(p, "/threads"):
		return "threads"
	case strings.Contains(p, "/fine_tuning"):
		return "fine_tuning"
	case strings.Contains(p, "/containers"):
		return "containers"
	case strings.Contains(p, "/vector_store"):
		return "vector_stores"
	case strings.Contains(p, "/videos"):
		return "videos"
	case strings.Contains(p, "/search"):
		return "search"
	case strings.Contains(p, "/ocr"):
		return "ocr"
	case strings.Contains(p, "/rag"):
		return "rag"
	case strings.Contains(p, "/indexes"):
		return "indexes"
	case strings.Contains(p, "/a2a"):
		return "a2a"
	case strings.Contains(p, "/interactions"):
		return "interactions"
	case strings.Contains(p, "/mcp"):
		return "mcp"
	case strings.Contains(p, "/agents"):
		return "agents"
	case strings.Contains(p, "/skills"):
		return "skills"
	case strings.Contains(p, "/memory"):
		return "memory"
	case strings.Contains(p, "/workflows"):
		return "workflows"
	case strings.Contains(p, "/evals"):
		return "evals"
	default:
		return "resources"
	}
}

// resourcePathID reads the resource id from the path. A collection path has no id.
// 参数 path（string）：URL 路径，用来匹配路由或选择错误包络。
// 返回 string（string）：路径最后一段里的资源 id。集合名、动作名和版本段返回空串。
// 调用：仅在 handlers.go 内使用
// 测试：无直接单测
func resourcePathID(path string) string {
	parts := catalog.Split(strings.TrimSuffix(path, "/"))
	if len(parts) == 0 {
		return ""
	}
	last := parts[len(parts)-1]
	if last == "content" && len(parts) >= 2 {
		last = parts[len(parts)-2]
	}
	reserved := map[string]bool{
		"files": true, "batches": true, "assistants": true, "threads": true, "jobs": true,
		"containers": true, "vector_stores": true, "vector_store": true, "videos": true,
		"search": true, "ocr": true, "rag": true, "indexes": true, "a2a": true,
		"interactions": true, "mcp": true, "v1": true, "v2": true, "list": true,
		"ingest": true, "query": true, "discover": true, "send": true, "cancel": true,
		"content": true, "fine_tuning": true, "proxy": true, "oauth": true, "token": true,
		"authorize": true, "generations": true, "edits": true,
	}
	if reserved[last] {
		return ""
	}
	return last
}

// nativeResource fills the default fields the public JSON for this resource kind needs.
// 参数 kind（string）：分类名，用来选择限额主体、日志类型或官方端点；body（map[string]any）：已解析或原始的 JSON。
// 返回 map[string]any（map[string]any）：nativeResource的字段表。缺键表示上游或库里没有这个字段。
// 调用：仅在 handlers.go 内使用
// 测试：无直接单测
func nativeResource(kind string, body map[string]any) map[string]any {
	id := str(body["id"])
	if id == "" {
		id = singular(kind) + "_" + httpx.CallID()[:12]
	}
	now := time.Now().UTC().Unix()
	obj := map[string]any{"id": id, "object": singular(kind), "created_at": now}
	for k, v := range body {
		if k == "password" {
			continue
		}
		obj[k] = v
	}
	switch kind {
	case "files":
		obj["object"] = "file"
		if obj["filename"] == nil {
			obj["filename"] = "upload.bin"
		}
		if obj["bytes"] == nil {
			obj["bytes"] = 0
		}
		if obj["purpose"] == nil {
			obj["purpose"] = "assistants"
		}
		obj["status"] = "processed"
	case "batches":
		obj["object"] = "batch"
		obj["status"] = "validating"
		obj["input_file_id"] = body["input_file_id"]
		obj["endpoint"] = body["endpoint"]
		if _, ok := obj["output_file_id"]; !ok {
			obj["output_file_id"] = nil
		}
		if obj["request_counts"] == nil {
			obj["request_counts"] = map[string]any{"total": 0, "completed": 0, "failed": 0}
		}
	case "assistants":
		obj["object"] = "assistant"
		obj["model"] = body["model"]
		obj["name"] = body["name"]
		obj["instructions"] = body["instructions"]
		obj["tools"] = body["tools"]
	case "threads":
		obj["object"] = "thread"
		if obj["messages"] == nil {
			obj["messages"] = []any{}
		}
	case "fine_tuning":
		obj["object"] = "fine_tuning.job"
		obj["status"] = "queued"
		obj["model"] = body["model"]
		obj["training_file"] = body["training_file"]
		if _, ok := obj["fine_tuned_model"]; !ok {
			obj["fine_tuned_model"] = nil
		}
	case "containers":
		obj["object"] = "container"
		obj["name"] = body["name"]
		obj["status"] = "running"
		obj["expires_after"] = body["expires_after"]
		obj["file_ids"] = body["file_ids"]
	case "vector_stores":
		obj["object"] = "vector_store"
		obj["name"] = body["name"]
		obj["status"] = "completed"
		if obj["file_counts"] == nil {
			obj["file_counts"] = map[string]any{"in_progress": 0, "completed": 0, "failed": 0, "cancelled": 0, "total": 0}
		}
	case "videos":
		obj["object"] = "video"
		obj["status"] = "queued"
		obj["model"] = body["model"]
	case "search":
		obj["object"] = "list"
		obj["data"] = []any{}
		obj["results"] = []any{}
		if obj["usage"] == nil {
			obj["usage"] = map[string]any{"prompt_tokens": 0, "total_tokens": 0}
		}
	case "interactions":
		obj["object"] = "interaction"
		if obj["status"] == nil {
			obj["status"] = "completed"
		}
		if obj["output"] == nil {
			text := str(body["input"])
			obj["output"] = []any{map[string]any{"type": "message", "content": text}}
		}
	case "ocr":
		obj["object"] = "ocr"
		obj["text"] = ""
		obj["results"] = []any{}
		obj["usage"] = map[string]any{"prompt_tokens": 0, "total_tokens": 0}
	case "rag":
		obj["object"] = "rag"
		obj["status"] = "ok"
		obj["matches"] = []any{}
		obj["results"] = []any{}
		obj["usage"] = map[string]any{"prompt_tokens": 0, "total_tokens": 0}
	case "a2a":
		obj["id"] = id
		obj["status"] = "completed"
		obj["artifacts"] = []any{}
	case "evals":
		obj["object"] = "eval"
		obj["name"] = body["name"]
		obj["status"] = "created"
	case "indexes":
		obj["object"] = "index"
		obj["name"] = body["name"]
		obj["status"] = "ready"
		obj["results"] = []any{}
		obj["usage"] = map[string]any{"prompt_tokens": 0, "total_tokens": 0}
	}
	Freeze(kind, obj)
	return obj
}

// singular is the singular resource name for a collection name.
// 参数 kind（string）：分类名，用来选择限额主体、日志类型或官方端点。
// 返回 string（string）：集合名对应的单数资源名。files 得到 file。没有特殊规则时去掉末尾的 s。
// 调用：仅在 handlers.go 内使用
// 测试：无直接单测
func singular(kind string) string {
	switch kind {
	case "files":
		return "file"
	case "batches":
		return "batch"
	case "assistants":
		return "assistant"
	case "threads":
		return "thread"
	case "containers":
		return "container"
	case "vector_stores":
		return "vector_store"
	case "videos":
		return "video"
	case "indexes":
		return "index"
	case "fine_tuning":
		return "fine_tuning.job"
	default:
		if strings.HasSuffix(kind, "s") && kind != "status" {
			return strings.TrimSuffix(kind, "s")
		}
		return kind
	}
}

// ServeMixed answers the catalog's "mixed" paths, such as /v1/agents, /v1/skills and /v1/workflows. These were removed from the product and only remain in the catalog. They are refused rather than served: the generic store behind them is one shared key-value namespace keyed by resource kind, with no owner and no team column, so serving them let any signed-in member — and any inference key — create records that every other principal could then list. Refusing is what the classification has always described; this makes the code match. The identity is still required before the refusal, so an anonymous caller learns nothing beyond the fact that the path exists.
// 参数 s（Host）：处理混合使用的数据面宿主；w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；r（*http.Request）：入站 HTTP 请求。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：gateway/catalog.go
// 测试：无直接单测
func ServeMixed(s Host, w http.ResponseWriter, r *http.Request) {
	if s.RequireMixed(w, r) == nil {
		return
	}
	logx.Debug("process %s %s step=catalog class=mixed retired=true", r.Method, r.URL.Path)
	httpx.WriteError(w, http.StatusGone, "removed", "this endpoint was removed and is not available")
}

// ServeMgmt handles management catalog paths. It requires a management identity, then reads or writes by resource kind.
// 参数 s（Host）：处理Mgmt使用的数据面宿主；w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；r（*http.Request）：入站 HTTP 请求。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：gateway/catalog.go
// 测试：无直接单测
func ServeMgmt(s Host, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	if guard.Manage(s, w, r) {
		return
	}
	writeCatalogPersist(s, w, r, readMap(r))
}

// writeCatalogPersist lists, reads, and writes catalog rows in the key-value table. Credential secrets are masked, and JSON-RPC or token paths return their own JSON.
// 参数 s（Host）：写入目录Persist使用的数据面宿主；w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；r（*http.Request）：入站 HTTP 请求；body（map[string]any）：已解析或原始的 JSON。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：仅在 handlers.go 内使用
// 测试：无直接单测
func writeCatalogPersist(s Host, w http.ResponseWriter, r *http.Request, body map[string]any) {
	path := r.URL.Path
	if _, ok := body["jsonrpc"]; ok {
		httpx.WriteJSON(w, 200, map[string]any{
			"jsonrpc": "2.0",
			"id":      body["id"],
			"result":  map[string]any{"tools": []any{}},
		})
		return
	}
	if (strings.Contains(path, "/oauth/token") || strings.HasSuffix(path, "/token")) && !strings.Contains(path, "count") {
		httpx.WriteJSON(w, 200, map[string]any{
			"access_token": "at-" + httpx.CallID()[:12],
			"token_type":   "Bearer",
			"expires_in":   3600,
		})
		return
	}
	kind, action, id := parseMgmt(r.Method, path, body)
	if id == "" || ((action == "update" || action == "delete") && (id == kind || id == singular(kind))) {
		if alt := str(body[idField(kind)]); alt != "" {
			id = alt
		} else if alt := str(body["id"]); alt != "" && id == "" {
			id = alt
		}
	}
	switch action {
	case "list":
		list, _ := s.RecordStore().ListKV(kind)
		if list == nil {
			list = []map[string]any{}
		}
		for i := range list {
			Freeze(kind, list[i])
		}
		httpx.WriteJSON(w, 200, catalogListBody(kind, path, redactCredentialList(kind, list, true)))
	case "info":
		if id == "" {
			httpx.WriteJSON(w, 200, map[string]any{"object": "list", "data": []any{}})
			return
		}
		m, err := s.RecordStore().GetKV(kind, id)
		if err != nil {
			m = map[string]any{"id": id, idField(kind): id, "object": singular(kind)}
		}
		Freeze(kind, m)
		httpx.WriteJSON(w, 200, redactCredentialObject(kind, m, true))
	case "delete":
		ids := idsFrom(body, kind+"s", idField(kind))
		if id != "" {
			ids = append(ids, id)
		}
		n := 0
		for _, x := range ids {
			if s.RecordStore().DeleteKV(kind, x) == nil {
				n++
			}
		}
		httpx.WriteJSON(w, 200, map[string]any{"deleted": n, idField(kind): id, "id": id})
	case "update":
		if id == "" {
			id = singular(kind) + "_" + httpx.CallID()[:12]
		}
		m, err := s.RecordStore().GetKV(kind, id)
		if err != nil {
			m = map[string]any{"id": id, idField(kind): id, "created_at": time.Now().UTC().Format(time.RFC3339)}
		}
		if kind == "credentials" || kind == "credential" {
			mergeCredentialPatch(m, body)
		} else {
			for k, v := range body {
				if k == "password" {
					continue
				}
				m[k] = v
			}
		}
		m["id"] = id
		m[idField(kind)] = id
		Freeze(kind, m)
		b, _ := json.Marshal(m)
		_ = s.RecordStore().PutKV(kind, id, string(b))
		httpx.WriteJSON(w, 200, redactCredentialObject(kind, m, true))
	default:
		if id == "" && kind == "memory" {
			if k := str(body["key"]); k != "" {
				id = k
			}
		}
		if id == "" {
			id = singular(kind) + "_" + httpx.CallID()[:12]
		}
		obj := map[string]any{}
		for k, v := range body {
			if k == "password" {
				continue
			}
			obj[k] = v
		}
		for _, nest := range []string{"search_tool", "prompt", "guardrail", "policy", "skill", "vector_store"} {
			inner, ok := obj[nest].(map[string]any)
			if !ok {
				continue
			}
			for ik, iv := range inner {
				if obj[ik] == nil {
					obj[ik] = iv
				}
			}
		}
		obj["id"] = id
		obj[idField(kind)] = id
		if obj["created_at"] == nil {
			obj["created_at"] = time.Now().UTC().Format(time.RFC3339)
		}
		if alias := aliasField(kind); str(obj[alias]) == "" {
			if n := str(body["name"]); n != "" {
				obj[alias] = n
			}
		}
		Freeze(kind, obj)
		b, _ := json.Marshal(obj)
		_ = s.RecordStore().PutKV(kind, id, string(b))
		httpx.WriteJSON(w, 200, redactCredentialObject(kind, obj, false))
	}
}

// mergeCredentialPatch merges a credential the way LiteLLM update_db_credential does. Incoming credential_values overlay the existing fields. A masked secret that contains a run of * does not overwritethe original, or an edit form that sends the mask back would store the real secret as asterisks.
// 参数 dst（map[string]any）：合并凭据Patch读到的 JSON 对象。缺键表示没有该字段；body（map[string]any）：已经解析的 JSON 对象。缺字段表示上游或调用方没有给这项。
// 返回：无。正文里的普通字段已覆盖到凭据上。password 和 credential_values 不在这里整段覆盖。
// 调用：仅在 handlers.go 内使用
// 测试：无直接单测
func mergeCredentialPatch(dst, body map[string]any) {
	for k, v := range body {
		if k == "password" || k == "credential_values" {
			continue
		}
		dst[k] = v
	}
	incoming, ok := body["credential_values"].(map[string]any)
	if !ok {
		return
	}
	cur, _ := dst["credential_values"].(map[string]any)
	if cur == nil {
		cur = map[string]any{}
	}
	for k, v := range incoming {
		if isSensitiveCredentialKey(k) {
			s, _ := v.(string)
			if strings.TrimSpace(s) == "" || strings.Contains(s, "**") {
				continue
			}
		}
		cur[k] = v
	}
	dst["credential_values"] = cur
}

// redactCredentialObject masks secret fields on a credential object. When showValues is false the original text is not returned.
// 参数 kind（string）：分类名，用来选择限额主体、日志类型或官方端点；obj（map[string]any）：打码凭据对象读到的 JSON 对象。缺键表示没有该字段；showValues（bool）：为真时走showValues这一支。为假时保持原来的路径。
// 返回 map[string]any（map[string]any）：打码凭据对象的字段表。缺键表示上游或库里没有这个字段。
// 调用：仅在 handlers.go 内使用
// 测试：无直接单测
func redactCredentialObject(kind string, obj map[string]any, showValues bool) map[string]any {
	if kind != "credentials" && kind != "credential" {
		return obj
	}
	vals, ok := obj["credential_values"].(map[string]any)
	if !ok {
		return obj
	}
	out := make(map[string]any, len(obj))
	for k, v := range obj {
		if k == "credential_values" {
			continue
		}
		out[k] = v
	}
	if showValues {
		out["credential_values"] = maskCredentialValues(vals)
	}
	return out
}

// redactCredentialList masks each credential in a list.
// 参数 kind（string）：分类名，用来选择限额主体、日志类型或官方端点；list（[]map[string]any）：打码凭据列表使用的map[string]any；showValues（bool）：为真时走showValues这一支。为假时保持原来的路径。
// 返回 []map[string]any（[]map[string]any）：一组map[string]any。没有匹配时为空切片，不是 nil 分页。
// 调用：仅在 handlers.go 内使用
// 测试：无直接单测
func redactCredentialList(kind string, list []map[string]any, showValues bool) []map[string]any {
	if kind != "credentials" && kind != "credential" {
		return list
	}
	out := make([]map[string]any, len(list))
	for i, m := range list {
		out[i] = redactCredentialObject(kind, m, showValues)
	}
	return out
}

// isSensitiveCredentialKey reports whether a field name looks like a key, token, or password. Those fields are masked by default.
// 参数 k（string）：是否敏感凭据密钥使用的k。空串表示调用方没有提供这项。
// 返回 bool（bool）：字段名看起来像密钥、令牌或口令时返回真。这类字段默认打码。
// 调用：仅在 handlers.go 内使用
// 测试：无直接单测
func isSensitiveCredentialKey(k string) bool {
	l := strings.ToLower(k)
	for _, w := range []string{"authorization", "token", "key", "secret", "password", "passwd", "credential"} {
		if strings.Contains(l, w) {
			return true
		}
	}
	return false
}

// maskCredentialValues 把凭据对象里像密钥、令牌或口令的字段换成掩码，避免明文出现在响应里。
// 参数 in（map[string]any）：mask凭据Values读到的 JSON 对象。缺键表示没有该字段。
// 返回 map[string]any（map[string]any）：mask凭据Values的字段表。缺键表示上游或库里没有这个字段。
// 调用：仅在 handlers.go 内使用
// 测试：无直接单测
func maskCredentialValues(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		s, ok := v.(string)
		if !ok || !isSensitiveCredentialKey(k) {
			out[k] = v
			continue
		}
		out[k] = maskSecret(s)
	}
	return out
}

// maskSecret keeps a few characters at each end of a secret. An empty string stays empty.
// 参数 v（string）：mask秘密使用的值。空串表示调用方没有提供这项。
// 返回 string（string）：固定文本 "*****"。
// 调用：仅在 handlers.go 内使用
// 测试：无直接单测
func maskSecret(v string) string {
	const unmasked = 4
	if len(v) <= unmasked {
		return "*****"
	}
	head := unmasked / 2
	return v[:head] + strings.Repeat("*", len(v)-unmasked) + v[len(v)-head:]
}

// Freeze fills fields the public contract for this resource family requires. An existing value is not overwritten.
// 参数 kind（string）：分类名，用来选择限额主体、日志类型或官方端点；obj（map[string]any）：Freeze读到的 JSON 对象。缺键表示没有该字段。
// 返回：无。这个资源族对外必须有的字段已补上。已经有值的字段不覆盖，password 会被删掉。
// 调用：gateway/access.go
// 测试：无直接单测
func Freeze(kind string, obj map[string]any) {
	delete(obj, "password")
	now := time.Now().UTC().Format(time.RFC3339)
	if obj["id"] == nil {
		obj["id"] = singular(kind) + "_" + httpx.CallID()[:12]
	}
	id := str(obj["id"])
	if obj["created_at"] == nil {
		obj["created_at"] = now
	}
	f := idField(kind)
	if obj[f] == nil {
		obj[f] = id
	}
	emptyUsage := map[string]any{"prompt_tokens": 0, "total_tokens": 0}
	switch kind {
	case "interactions":
		setDefault(obj, "status", "completed")
		setDefault(obj, "output", []any{})
	case "search", "ocr", "rag", "indexes":
		setDefault(obj, "results", []any{})
		setDefault(obj, "usage", emptyUsage)
		if kind == "ocr" {
			setDefault(obj, "text", "")
		}
		if kind == "indexes" {
			setDefault(obj, "object", "index")
			setDefault(obj, "name", "")
			setDefault(obj, "status", "ready")
		}
	case "vector_stores", "vector_store":
		setDefault(obj, "object", "vector_store")
		setDefault(obj, "name", "")
		setDefault(obj, "status", "completed")
		setDefault(obj, "file_counts", map[string]any{"in_progress": 0, "completed": 0, "failed": 0, "cancelled": 0, "total": 0})
		setDefault(obj, "vector_store_id", id)
		setDefault(obj, "vector_store_name", obj["name"])
	case "batches":
		setDefault(obj, "object", "batch")
		setDefault(obj, "status", "validating")
		if _, ok := obj["output_file_id"]; !ok {
			obj["output_file_id"] = nil
		}
		setDefault(obj, "request_counts", map[string]any{"total": 0, "completed": 0, "failed": 0})
		setDefault(obj, "endpoint", "")
	case "files":
		setDefault(obj, "object", "file")
		setDefault(obj, "status", "processed")
		setDefault(obj, "filename", "upload.bin")
		setDefault(obj, "purpose", "assistants")
		setDefault(obj, "bytes", 0)
	case "fine_tuning":
		setDefault(obj, "object", "fine_tuning.job")
		setDefault(obj, "status", "queued")
		if _, ok := obj["fine_tuned_model"]; !ok {
			obj["fine_tuned_model"] = nil
		}
	case "containers":
		setDefault(obj, "object", "container")
		setDefault(obj, "status", "running")
		setDefault(obj, "name", "")
	case "assistants":
		setDefault(obj, "object", "assistant")
	case "threads":
		setDefault(obj, "object", "thread")
		setDefault(obj, "messages", []any{})
	case "videos":
		setDefault(obj, "object", "video")
		setDefault(obj, "status", "queued")
	case "a2a":
		setDefault(obj, "status", "completed")
		setDefault(obj, "artifacts", []any{})
	case "evals":
		setDefault(obj, "object", "eval")
		setDefault(obj, "name", "")
		setDefault(obj, "status", "created")
	case "prompts":
		setDefault(obj, "version", 1)
		setDefault(obj, "prompt_id", id)
	case "workflows":
		setDefault(obj, "run_id", id)
		setDefault(obj, "status", "completed")
		setDefault(obj, "events", []any{})
		setDefault(obj, "messages", []any{})
	case "agents":
		setDefault(obj, "agent_id", id)
		setDefault(obj, "agent_name", "")
		setDefault(obj, "litellm_params", map[string]any{})
	case "skills":
		setDefault(obj, "skill_id", id)
		setDefault(obj, "name", "")
		setDefault(obj, "source", "")
	case "memory":
		setDefault(obj, "key", id)
		setDefault(obj, "value", "")
	case "access_group", "access_groups":
		setDefault(obj, "access_group_id", id)
		if str(obj["access_group_name"]) == "" {
			if n := str(obj["name"]); n != "" {
				obj["access_group_name"] = n
			} else {
				obj["access_group_name"] = id
			}
		}
		setDefault(obj, "description", "")
		setDefault(obj, "models", []any{})
		setDefault(obj, "access_model_names", []any{})
		setDefault(obj, "access_mcp_server_ids", []any{})
		setDefault(obj, "access_agent_ids", []any{})
		setDefault(obj, "assigned_key_ids", []any{})
		setDefault(obj, "assigned_team_ids", []any{})
		setDefault(obj, "access_mcp_servers", []any{})
		setDefault(obj, "access_agents", []any{})
		setDefault(obj, "assigned_keys", []any{})
		setDefault(obj, "assigned_teams", []any{})
		setDefault(obj, "updated_at", obj["created_at"])
		if _, ok := obj["created_by"]; !ok {
			obj["created_by"] = nil
		}
		if _, ok := obj["updated_by"]; !ok {
			obj["updated_by"] = nil
		}
		if _, ok := obj["budget_id"]; !ok {
			obj["budget_id"] = nil
		}
	case "policies", "policy":
		setDefault(obj, "policy_id", id)
		setDefault(obj, "policy_name", "")
		setDefault(obj, "version", 1)
		setDefault(obj, "status", "active")
	case "tags", "tag":
		if n := str(obj["tag_name"]); n != "" {
			obj["name"] = n
		}
		setDefault(obj, "name", id)
		setDefault(obj, "tag_name", obj["name"])
		setDefault(obj, "spend", 0)
	case "credentials", "credential":
		setDefault(obj, "credential_name", id)
		setDefault(obj, "credential_info", map[string]any{})
	case "guardrails", "apply_guardrail":
		setDefault(obj, "guardrail_id", id)
		setDefault(obj, "guardrail_name", "")
		setDefault(obj, "litellm_params", map[string]any{})
	case "customer", "end_user":
		setDefault(obj, "user_id", id)
		setDefault(obj, "spend", 0)
		setDefault(obj, "blocked", false)
		if _, ok := obj["max_budget"]; !ok {
			obj["max_budget"] = nil
		}
	case "invitation":
		setDefault(obj, "user_id", "")
		setDefault(obj, "is_accepted", false)
		setDefault(obj, "expires", now)
		setDefault(obj, "expires_at", obj["expires"])
		if _, ok := obj["accepted_at"]; !ok {
			obj["accepted_at"] = nil
		}
		setDefault(obj, "created_by", "")
		setDefault(obj, "updated_at", now)
		setDefault(obj, "updated_by", "")
		setDefault(obj, "has_user_setup_sso", false)
	case "mcp", "mcp-servers", "mcp_servers", "server":
		setDefault(obj, "server_id", id)
		setDefault(obj, "server_name", "")
		setDefault(obj, "url", "")
		setDefault(obj, "transport", "sse")
		setDefault(obj, "status", "ok")
		setDefault(obj, "tools", []any{})
	case "search_tools", "search-tools", "search_tool":
		setDefault(obj, "search_tool_id", id)
		setDefault(obj, "search_tool_name", "")
	case "scim", "Users", "Groups":
		setDefault(obj, "schemas", []any{"urn:ietf:params:scim:schemas:core:2.0:User"})
		setDefault(obj, "userName", "")
		setDefault(obj, "meta", map[string]any{"resourceType": "User"})
		setDefault(obj, "Resources", []any{})
		setDefault(obj, "totalResults", 0)
		setDefault(obj, "Resources", []any{})
		setDefault(obj, "totalResults", 0)
	case "jwt":
		setDefault(obj, "jwt_claim_name", "")
		setDefault(obj, "jwt_claim_value", "")
	case "config":
		setDefault(obj, "status", "ok")
		setDefault(obj, "version", ProxyVersion)
	case "debug", "memory-usage":
		setDefault(obj, "rss_mb", 0)
		setDefault(obj, "heap", 0)
		setDefault(obj, "asyncio_tasks", 0)
	case "callback", "callbacks":
		setDefault(obj, "callbacks", []any{})
		setDefault(obj, "success_callback", []any{})
		setDefault(obj, "failure_callback", []any{})
	case "compliance", "vantage":
		setDefault(obj, "status", "ok")
		setDefault(obj, "exported_count", 0)
	case "onboarding", "onboard":
		setDefault(obj, "token", "sess-"+httpx.CallID()[:8])
		setDefault(obj, "user_id", id)
	case "utils", "utils/token_counter", "token_counter":
		setDefault(obj, "total_tokens", 8)
	case "allowed_ips", "allowed-ips":
		setDefault(obj, "allowed_ips", []any{})
	case "audit":
		setDefault(obj, "action", "created")
		setDefault(obj, "table_name", "")
		setDefault(obj, "object_id", id)
		setDefault(obj, "changed_by", "")
		setDefault(obj, "changed_by_api_key", "")
		setDefault(obj, "before_value", map[string]any{})
		setDefault(obj, "updated_values", map[string]any{})
		setDefault(obj, "updated_at", obj["created_at"])
	case "email":
		setDefault(obj, "event_settings", []any{})
	case "alerting":
		setDefault(obj, "alerting_threshold", 0)
	case "schedule", "schedules", "ops_schedules":
		setDefault(obj, "enabled", true)
		setDefault(obj, "last_run", nil)
		setDefault(obj, "next_run", nil)
	case "claude-code", "plugins":
		setDefault(obj, "name", "")
		setDefault(obj, "version", "1")
		setDefault(obj, "enabled", true)
	case "budget", "budgets":
		setDefault(obj, "budget_id", id)
		if _, ok := obj["max_budget"]; !ok {
			obj["max_budget"] = nil
		}
		if _, ok := obj["soft_budget"]; !ok {
			obj["soft_budget"] = nil
		}
		if _, ok := obj["tpm_limit"]; !ok {
			obj["tpm_limit"] = nil
		}
		if _, ok := obj["rpm_limit"]; !ok {
			obj["rpm_limit"] = nil
		}
		if _, ok := obj["budget_duration"]; !ok {
			obj["budget_duration"] = nil
		}
		if _, ok := obj["budget_reset_at"]; !ok {
			obj["budget_reset_at"] = nil
		}
	}
}

// setDefault writes a default only when the key is absent.
// 参数 obj（map[string]any）：写入Default读到的 JSON 对象。缺键表示没有该字段；key（string）：上游或调用方的密钥。空串表示还不能转发或还没有密钥；val（any）：写入Default接到的动态值。类型在函数体内收窄。
// 返回：无。键不存在时写入了默认值。已有值时不改。
// 调用：仅在 handlers.go 内使用
// 测试：无直接单测
func setDefault(obj map[string]any, key string, val any) {
	if obj[key] == nil {
		obj[key] = val
	}
}

// parseMgmt reads the resource kind, action, and id from the method and path.
// 参数 method（string）：HTTP 方法，例如 GET 或 POST；path（string）：解析Mgmt要定位的路径。可能是 URL，也可能是字段路径；body（map[string]any）：已解析或原始的 JSON。
// 返回 kind（string）：固定文本 "misc"；action（string）：固定文本 "misc"；id（string）：固定文本 "misc"。
// 调用：仅在 handlers.go 内使用
// 测试：无直接单测
func parseMgmt(method, path string, body map[string]any) (kind, action, id string) {
	parts := catalog.Split(strings.TrimSuffix(path, "/"))
	filtered := []string{}
	for _, p := range parts {
		if p == "v1" || p == "v2" || p == "v1beta" || p == "api" {
			continue
		}
		filtered = append(filtered, p)
	}
	if len(filtered) == 0 {
		return "misc", "list", ""
	}
	kind = filtered[0]
	last := filtered[len(filtered)-1]
	if last == "runs" {
		if method == http.MethodPost || method == http.MethodPut {
			return kind, "new", ""
		}
		return kind, "list", ""
	}
	actions := map[string]bool{"new": true, "list": true, "info": true, "update": true, "delete": true, "register": true}
	switch {
	case actions[last]:
		action = last
		if last == "register" {
			action = "new"
		}
		if len(filtered) >= 3 && !actions[filtered[len(filtered)-2]] {
			id = filtered[len(filtered)-2]
		}
	case method == http.MethodDelete:
		action = "delete"
		id = last
	case method == http.MethodPatch || method == http.MethodPut:
		action = "update"
		id = last
	case method == http.MethodGet || method == http.MethodHead:
		if last == kind || last == "ui" || strings.Contains(last, "settings") || catalogCollectionLast(last) || (kind == "mcp" && (last == "status" || last == "user-env-vars")) || (strings.Contains(path, "/daily/activity") && (last == "activity" || last == "aggregated")) {
			action = "list"
		} else if len(filtered) == 1 {
			action = "list"
		} else {
			action = "info"
			id = last
		}
	default:
		if last == kind {
			action = "new"
		} else if last == "approve" || last == "reject" {
			action = "update"
			if len(filtered) >= 2 {
				id = filtered[len(filtered)-2]
			}
		} else {
			action = "new"
			if last != kind {
				id = last
			}
		}
	}
	if id == kind || actions[id] || catalogCollectionLast(id) {
		id = ""
	}
	if qid := ""; body != nil {
		if id == "" {
			id = str(body[idField(kind)])
		}
		_ = qid
	}
	return kind, action, id
}

// catalogCollectionLast reports whether the last path segment names a collection rather than a concrete id.
// 参数 last（string）：目录CollectionLast使用的last。空串表示调用方没有提供这项。
// 返回 bool（bool）：路径最后一段是集合名而不是具体 id 时返回真。
// 调用：仅在 handlers.go 内使用
// 测试：无直接单测
func catalogCollectionLast(last string) bool {
	switch last {
	case "server", "servers", "plugins", "health", "access_groups", "submissions", "available_providers",
		"toolset", "toolsets", "fields", "discover", "templates", "overview", "logs", "configs",
		"budgets", "users", "end_users":
		return true
	}
	return false
}

// catalogListBody wraps a list as the JSON for this resource kind. Some kinds need paging fields and some are a plain array.
// 参数 kind（string）：分类名，用来选择限额主体、日志类型或官方端点；path（string）：目录列表正文要定位的路径。可能是 URL，也可能是字段路径；list（[]map[string]any）：目录列表正文使用的map[string]any。
// 返回 any（any）：目录列表正文的结果。具体类型由调用方断言。
// 调用：仅在 handlers.go 内使用
// 测试：无直接单测
func catalogListBody(kind, path string, list []map[string]any) any {
	if list == nil {
		list = []map[string]any{}
	}
	p := strings.ToLower(path)
	switch {
	case strings.HasPrefix(p, "/management/v1/"):
		return pagedListBody(path, list, 1, 50)
	case strings.Contains(p, "/get/") && strings.Contains(p, "settings"):
		return map[string]any{"values": map[string]any{}, "field_schema": map[string]any{}}
	case strings.Contains(p, "/coordination_redis"):
		return map[string]any{"values": map[string]any{}, "fields": []any{}, "source": nil}
	case strings.Contains(p, "/mcp/access_groups"):
		return map[string]any{"access_groups": list}
	case strings.Contains(p, "/mcp/server/submissions"):
		return map[string]any{
			"total": len(list), "pending_review": 0, "active": 0, "rejected": 0, "items": list,
		}
	case strings.Contains(p, "/mcp/discover"):
		return map[string]any{"servers": list, "categories": []any{}}
	case strings.Contains(p, "/mcp/"):
		return list
	case kind == "access_group" || kind == "access_groups" || kind == "unified_access_group":
		return list
	case strings.Contains(p, "/gateway/daily/activity"):
		return map[string]any{
			"total_successful_requests": 0, "total_failed_requests": 0,
			"by_date": []any{}, "by_route": []any{},
		}
	case strings.Contains(p, "/daily/activity"):
		return map[string]any{
			"results": []any{},
			"metadata": map[string]any{
				"total_spend": 0, "total_prompt_tokens": 0, "total_completion_tokens": 0,
				"total_tokens": 0, "total_api_requests": 0, "total_successful_requests": 0,
				"total_failed_requests": 0, "total_cache_read_input_tokens": 0,
				"total_cache_creation_input_tokens": 0, "total_pages": 1, "has_more": false,
			},
		}
	case strings.Contains(p, "/callbacks/configs"):
		return list
	case strings.Contains(p, "/alerting/settings"):
		return list
	case kind == "tag" || kind == "tags":
		return list
	case kind == "agents":
		return list
	case kind == "memory":
		return map[string]any{"object": "list", "data": list, "memories": list, "total": len(list)}
	case kind == "workflows" || strings.Contains(p, "/workflows/runs"):
		return map[string]any{"object": "list", "data": list, "runs": list, "events": []any{}, "messages": []any{}}
	case strings.Contains(p, "available_providers"):
		return map[string]any{
			"providers": []any{
				map[string]any{"provider_name": "tavily", "ui_friendly_name": "Tavily"},
				map[string]any{"provider_name": "perplexity", "ui_friendly_name": "Perplexity"},
				map[string]any{"provider_name": "serper", "ui_friendly_name": "Serper"},
			},
		}
	case strings.Contains(p, "/guardrails/usage/overview"):
		return map[string]any{
			"chart": []any{}, "passRate": 100, "rows": []any{},
			"totalBlocked": 0, "totalCost": nil, "totalRequests": 0,
			"totalUntrackedUsageUnits": map[string]any{}, "totalUsageUnits": map[string]any{},
		}
	case strings.Contains(p, "/guardrails/usage/logs"):
		return map[string]any{"object": "list", "data": list, "logs": list}
	case strings.Contains(p, "/guardrails/ui/add_guardrail_settings"):
		return map[string]any{
			"supported_entities":     []any{},
			"supported_actions":      []any{"MASK", "BLOCK"},
			"supported_modes":        []any{"pre_call", "during_call", "post_call"},
			"pii_entity_categories":  []any{},
			"guardrail_provider_map": map[string]any{"Presidio": "presidio", "Custom": "custom"},
		}
	case strings.Contains(p, "/guardrails/ui/provider_specific_params"):
		return map[string]any{
			"Presidio": map[string]any{},
			"Custom":   map[string]any{},
		}
	case kind == "guardrails":
		return map[string]any{"object": "list", "data": list, "guardrails": list}
	case strings.Contains(p, "/policy/templates"):
		return list
	case strings.Contains(p, "/policies/attachments"):
		return map[string]any{"object": "list", "data": list, "attachments": list}
	case kind == "policies" || kind == "policy":
		return map[string]any{"object": "list", "data": list, "policies": list}
	case kind == "prompts":
		return map[string]any{"object": "list", "data": list, "prompts": list}
	case kind == "search_tools" || kind == "search-tools" || kind == "search_tool":
		return map[string]any{"object": "list", "data": list, "search_tools": list}
	case kind == "vector_store" || kind == "vector_stores":
		return map[string]any{"object": "list", "data": list}
	case kind == "credentials" || kind == "credential":
		return map[string]any{"object": "list", "data": list, "credentials": list}
	case kind == "claude-code" || strings.Contains(p, "/claude-code/plugins"):
		return map[string]any{"plugins": list, "count": len(list)}
	case kind == "tool":
		return map[string]any{"object": "list", "data": list, "tools": list}
	default:
		out := map[string]any{"object": "list", "data": list}
		out[kind] = list
		if !strings.HasSuffix(kind, "s") {
			out[kind+"s"] = list
		}
		return out
	}
}

// idField is the field name this resource uses to identify a row.
// 参数 kind（string）：分类名，用来选择限额主体、日志类型或官方端点。
// 返回 string（string）：这种资源用来标识一行的字段名，例如 credential_name、user_id。没有专门名字时用单数名加 _id。
// 调用：仅在 handlers.go 内使用
// 测试：无直接单测
func idField(kind string) string {
	switch kind {
	case "credentials", "credential":
		return "credential_name"
	case "guardrails":
		return "guardrail_id"
	case "prompts":
		return "prompt_id"
	case "tags", "tag":
		return "name"
	case "access_group", "access_groups":
		return "access_group_id"
	case "skills":
		return "skill_id"
	case "agents":
		return "agent_id"
	case "memory":
		return "key"
	case "workflows":
		return "run_id"
	case "evals":
		return "eval_id"
	case "search_tools", "search-tools", "search_tool":
		return "search_tool_id"
	case "mcp-servers", "mcp_servers", "mcp":
		return "server_id"
	case "policies", "policy":
		return "policy_id"
	case "customer", "end_user":
		return "user_id"
	case "invitation":
		return "id"
	case "vector_store":
		return "vector_store_id"
	default:
		return singular(kind) + "_id"
	}
}

// aliasField 是这种资源的显示名字段。没有单独约定时用单数名加 _alias。
// 参数 kind（string）：分类名，用来选择限额主体、日志类型或官方端点。
// 返回 string（string）：显示名字段名。guardrails 是 guardrail_name，没有单独约定时是单数名加 _alias。
// 调用：仅在 handlers.go 内使用
// 测试：无直接单测
func aliasField(kind string) string {
	switch kind {
	case "guardrails":
		return "guardrail_name"
	case "prompts":
		return "prompt_id"
	case "agents":
		return "agent_name"
	case "credentials":
		return "credential_name"
	case "tags":
		return "name"
	case "access_group", "access_groups":
		return "access_group_name"
	default:
		return singular(kind) + "_alias"
	}
}

// pagedListBody slices by page and size and includes the total.
// 参数 path（string）：paged列表正文要定位的路径。可能是 URL，也可能是字段路径；list（[]map[string]any）：paged列表正文使用的map[string]any；page（int）：页码，从 1 开始；size（int）：最多返回的条数。零或负数表示用默认页大小。
// 返回 map[string]any（map[string]any）：给响应或报表用的 JSON 对象。键是前端已经约定的字段，缺键表示这项没有数据。
// 调用：仅在 handlers.go 内使用
// 测试：无直接单测
func pagedListBody(path string, list []map[string]any, page, size int) map[string]any {
	if list == nil {
		list = []map[string]any{}
	}
	if size < 1 {
		size = 50
	}
	if page < 1 {
		page = 1
	}
	total := len(list)
	pages := total / size
	if total%size != 0 {
		pages++
	}
	if pages < 1 {
		pages = 1
	}
	data := sliceMaps(list, page, size)
	self := fmt.Sprintf("%s?page=%d&page_size=%d", path, page, size)
	links := map[string]any{
		"self":  self,
		"first": fmt.Sprintf("%s?page=1&page_size=%d", path, size),
		"last":  fmt.Sprintf("%s?page=%d&page_size=%d", path, pages, size),
		"next":  nil,
		"prev":  nil,
	}
	if page < pages {
		links["next"] = fmt.Sprintf("%s?page=%d&page_size=%d", path, page+1, size)
	}
	if page > 1 {
		links["prev"] = fmt.Sprintf("%s?page=%d&page_size=%d", path, page-1, size)
	}
	return map[string]any{
		"data":  data,
		"meta":  map[string]any{"page": page, "page_size": size, "total_count": total, "total_pages": pages},
		"links": links,
	}
}
