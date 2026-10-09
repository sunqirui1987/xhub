// affinity.go pins a caller to one deployment. Chat sessions use a one-hour
// TTL so repeated turns hit the same upstream. Official task ids (video and
// other bypass creates) use a seven-day TTL and a separate key, so a later
// poll does not depend on the chat pin expiring.

package gateway

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/sunqirui1987/xhub/internal/auth"
	"github.com/sunqirui1987/xhub/internal/dataplane"
	"github.com/sunqirui1987/xhub/internal/gateway/family"
	"github.com/sunqirui1987/xhub/internal/logx"
	"github.com/sunqirui1987/xhub/internal/provider"
)

const affinityTTL = time.Hour

// PlanRoute 解析会话与部署归属，并在统一 Responses 续接时恢复临时历史。
// 参数 r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文；alias（string）：对外模型名，用来选部署和记用量；body（map[string]any）：已经解析的 JSON 对象。缺字段表示上游或调用方没有给这项；p（*auth.Principal）：已经解析的调用方，含用户、团队和密钥。
// 返回 RoutePlan：响应 ID 优先确定部署；历史按调用方、型号、入口隔离，未知或过期时 Err 非空。
// 调用：dataplane/host.go、dataplane/official.go、dataplane/serve.go
// 测试：bypass_logic_test.go、failure_log_test.go、log_completeness_test.go
func (s *Server) PlanRoute(r *http.Request, alias string, body map[string]any, p *auth.Principal) dataplane.RoutePlan {
	endpoint := ""
	if r != nil && r.URL != nil {
		endpoint = provider.EndpointForOp(family.InferenceOp(r.URL.Path))
		if hit, ok := provider.Match(r.Method, r.URL.Path, nil); ok {
			endpoint = hit.Transport.EndpointID
		}
	}
	plan := dataplane.RoutePlan{Alias: alias, Caller: callerScope(p), Endpoint: endpoint}
	if body != nil {
		if prev := strings.TrimSpace(asString(body["previous_response_id"])); prev != "" {
			if id := s.affinityGet(responsePinKey(alias, plan.Caller, prev, plan.Endpoint)); id != "" {
				plan.Pinned = id
				if plan.Endpoint == "responses" {
					raw := s.affinityGet(responseContextKey(alias, plan.Caller, prev, plan.Endpoint))
					if json.Unmarshal([]byte(raw), &plan.History) != nil || len(plan.History) == 0 {
						plan.Err = fmt.Errorf("response context is unavailable or expired; submit full history")
					}
				}
			} else {
				plan.Err = fmt.Errorf("response ownership is unknown; submit full history")
			}
			plan.Required = true
		}
	}
	plan.SessionID = sessionID(r, body)
	if plan.Pinned == "" && plan.SessionID != "" && plan.Caller != "" {
		plan.Pinned = s.affinityGet(sessionPinKey(alias, plan.Caller, plan.SessionID, plan.Endpoint))
	}
	return plan
}

// CommitRoute 保存成功请求的会话、响应归属及统一 Responses 的临时历史。
// 参数 plan（dataplane.RoutePlan）：提交路由使用的RoutePlan；deploymentID（string）：部署 id。空串表示当前没有钉住的部署；responseID（string）：提交路由使用的响应标识。空串表示调用方没有提供这项。
// 返回：无。History 随响应归属一小时过期，与原厂 store 无关；部署 id 为空时不写。
// 调用：dataplane/host.go、dataplane/serve.go
// 测试：bypass_logic_test.go、failure_log_test.go、log_completeness_test.go
func (s *Server) CommitRoute(plan dataplane.RoutePlan, deploymentID, responseID string) {
	if deploymentID == "" {
		return
	}
	if plan.SessionID != "" && plan.Caller != "" {
		s.affinitySet(sessionPinKey(plan.Alias, plan.Caller, plan.SessionID, plan.Endpoint), deploymentID)
	}
	if responseID != "" && plan.Caller != "" {
		// 先保存上下文再发布归属，避免完成事件后的下一轮读到只有归属的半成品。
		if plan.Endpoint == "responses" && len(plan.History) > 0 {
			raw, err := json.Marshal(plan.History)
			if err != nil {
				return
			}
			s.affinitySet(responseContextKey(plan.Alias, plan.Caller, responseID, plan.Endpoint), string(raw))
		}
		s.affinitySet(responsePinKey(plan.Alias, plan.Caller, responseID, plan.Endpoint), deploymentID)
	}
}

// responseContextKey 为统一 Responses 的临时历史生成隔离键。
// 参数为公开型号、调用方、响应 ID 和入口；返回无明文身份的缓存键。
// PlanRoute/CommitRoute 使用，与归属相同一小时 TTL，过期后需要完整历史。
func responseContextKey(alias, caller, responseID, endpoint string) string {
	return strings.Replace(responsePinKey(alias, caller, responseID, endpoint), "deployment_affinity:v4:response:", "response_context:v1:", 1)
}

// responsePinKey isolates continuation IDs by caller and public model.
// 参数 alias、caller、responseID：模型、调用方范围和响应 id。
// 返回：不含原始调用方凭据的缓存键。
// 调用：PlanRoute、CommitRoute。测试：affinity_test.go。
func responsePinKey(alias, caller, responseID string, endpoint ...string) string {
	raw, _ := json.Marshal(append([]string{alias, caller, responseID}, endpoint...))
	sum := sha256.Sum256(raw)
	return "deployment_affinity:v4:response:" + hex.EncodeToString(sum[:])
}

// 用完整 SHA256 哈希隔离模型、调用方与会话，避免分隔符碰撞及原始标识泄露。
// 参数 alias（string）：对外模型名；caller（string）：调用方范围，一般是密钥哈希或用户 id，用来隔离粘滞键；sessionID（string）：会话 id。相同会话应钉在同一上游部署。
// 返回 string（string）：会话粘滞的 Redis 键，含对外名、调用方哈希和会话 id。
// 调用：仅在 affinity.go 内使用
// 测试：affinity_test.go
func sessionPinKey(alias, caller, sessionID string, endpoint ...string) string {
	raw, _ := json.Marshal(append([]string{alias, caller, sessionID}, endpoint...))
	sum := sha256.Sum256(raw)
	return "deployment_affinity:v3:session:" + hex.EncodeToString(sum[:])
}

// 确定粘滞键里的调用方范围，优先密钥哈希，其次用户。
// 参数 p（*auth.Principal）：已经解析的调用方，含用户、团队和密钥。
// 返回 string（string）：调用方范围。有密钥哈希时用密钥，否则用用户。没有身份时为空串。
// 调用：仅在 affinity.go 内使用
// 测试：无直接单测
func callerScope(p *auth.Principal) string {
	if p == nil {
		return ""
	}
	if p.Hash != "" {
		return "key:" + p.Hash
	}
	if p.UserID != "" {
		return "user:" + p.UserID
	}
	return ""
}

// sessionID is the stable conversation id. The order follows sub2api: a client session wins, then a cache key, then the previous response, then a hash of the prompt prefix that stays the same across turns.
// 参数 r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文；body（map[string]any）：已经解析的 JSON 对象。缺字段表示上游或调用方没有给这项。
// 返回 string（string）：稳定的会话 id。客户端会话头优先，其次缓存键、上一次响应、提示前缀哈希。都没有时为空串。
// 调用：仅在 affinity.go 内使用
// 测试：affinity_test.go
func sessionID(r *http.Request, body map[string]any) string {
	if r != nil {
		for _, name := range []string{
			"Session-Id", "Session_id", "Conversation_id",
			"X-Session-Affinity", "X-Session-Id", "X-Opencode-Session", "X-Conversation-Id",
		} {
			if v := strings.TrimSpace(r.Header.Get(name)); v != "" {
				return v
			}
		}
	}
	if body == nil {
		return ""
	}
	if meta, ok := body["metadata"].(map[string]any); ok {
		if v := claudeSession(asString(meta["user_id"])); v != "" {
			return v
		}
		if v := firstString(meta, "session_id", "litellm_session_id", "prompt_cache_key"); v != "" {
			return v
		}
	}
	if v := firstString(body, "prompt_cache_key", "litellm_session_id"); v != "" {
		return v
	}
	if prev := strings.TrimSpace(asString(body["previous_response_id"])); strings.HasPrefix(prev, "resp_") {
		return "prev:" + prev
	}
	return contentSessionID(body)
}

// claudeSession reads the session uuid Claude Code puts in metadata.user_id.
// 参数 raw（string）：claude会话使用的原始内容。空串表示调用方没有提供这项。
// 返回 string（string）：metadata.user_id 里的 Claude Code 会话 uuid。不是这种格式或为空时返回空串。
// 调用：仅在 affinity.go 内使用
// 测试：无直接单测
func claudeSession(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "{") {
		var doc struct {
			SessionID string `json:"session_id"`
		}
		if json.Unmarshal([]byte(raw), &doc) == nil && doc.SessionID != "" {
			return doc.SessionID
		}
	}
	return ""
}

// 在请求没有会话 id 时，用模型、指令、工具和首条用户文本算出一个稳定 id。
// 参数 body（map[string]any）：已经解析的 JSON 对象。缺字段表示上游或调用方没有给这项。
// 返回 string（string）：从正文算出的会话 id。相同对话内容得到相同 id，用来钉住上游。
// 调用：仅在 affinity.go 内使用
// 测试：无直接单测
func contentSessionID(body map[string]any) string {
	var b strings.Builder
	b.WriteString(asString(body["model"]))
	b.WriteByte('\n')
	b.WriteString(asString(body["instructions"]))
	b.WriteByte('\n')
	if tools, ok := body["tools"].([]any); ok {
		for _, tool := range tools {
			m, _ := tool.(map[string]any)
			b.WriteString(asString(m["name"]))
			if fn, ok := m["function"].(map[string]any); ok {
				b.WriteString(asString(fn["name"]))
			}
			b.WriteByte(',')
		}
	}
	b.WriteByte('\n')
	b.WriteString(firstUserText(body["messages"]))
	if strings.Trim(firstUserText(body["messages"]), "\n") == "" {
		b.WriteString(firstUserText(body["input"]))
	}
	if b.Len() == 0 || strings.Trim(b.String(), "\n,") == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(b.String()))
	return "content:" + hex.EncodeToString(sum[:16])
}

// 取出第一条用户消息的文本，系统消息单独留下。
// 参数 messages（any）：对话消息。元素是带角色和内容的对象。
// 返回 string（string）：拼出的用户文本。没有文本时为空串。
// 调用：仅在 affinity.go 内使用
// 测试：无直接单测
func firstUserText(messages any) string {
	list, _ := messages.([]any)
	var system string
	for _, item := range list {
		m, _ := item.(map[string]any)
		role := asString(m["role"])
		text := messageText(m["content"])
		if (role == "system" || role == "developer") && system == "" {
			system = text
		}
		if role == "user" && text != "" {
			return system + "\n" + text
		}
	}
	return system
}

// 把消息内容收成纯文本。字符串原样返回，多段内容只拼接文本段。
// 参数 content（any）：消息内容。可以是字符串，也可以是多段内容数组。
// 返回 string（string）：拼出的用户文本。没有文本时为空串。
// 调用：仅在 affinity.go 内使用
// 测试：无直接单测
func messageText(content any) string {
	if s, ok := content.(string); ok {
		return s
	}
	parts, _ := content.([]any)
	var b strings.Builder
	for _, part := range parts {
		switch p := part.(type) {
		case string:
			b.WriteString(p)
		case map[string]any:
			if t := asString(p["text"]); t != "" {
				b.WriteString(t)
			}
		}
	}
	return b.String()
}

// 按键的顺序从优先表和备用表里取第一个非空字符串。
// 参数 m（map[string]any）：已经解析的 JSON 对象，键是上游或配置里的字段名；keys（...string）：首个字符串使用的string。
// 返回 string（string）：按字符串读出的值。不是字符串或没有该键时为空串，不 panic。
// 调用：仅在 affinity.go 内使用。
// 测试：无直接单测
func firstString(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if v := strings.TrimSpace(asString(m[key])); v != "" {
			return v
		}
	}
	return ""
}

// 把动态值当成字符串读取。不是字符串时为空串。
// 参数 v（any）：JSON 里读出的动态值。数字、字符串和对象都要接住，类型不符时按零值而不是 panic。
// 返回 string（string）：按字符串读出的值。不是字符串或没有该键时为空串，不 panic。
// 调用：仅在 affinity.go 内使用
// 测试：无直接单测
func asString(v any) string {
	s, _ := v.(string)
	return s
}

type affinityPin struct {
	deployment string
	until      time.Time
}

// 读取粘滞键。有 Redis 时读 Redis，否则读进程内表。
// 参数 key（string）：缓存或配置表的键。
// 返回 string（string）：读到的字符串。键不存在时为空串，调用方要把空串当成没有钉住。
// 调用：gateway/wire.go
// 测试：affinity_test.go
func (s *Server) affinityGet(key string) string {
	if s == nil || key == "" {
		return ""
	}
	if s.Live != nil {
		if v, ok := s.Live.GetString(context.Background(), key); ok {
			return v
		}
	}
	s.affinityMu.Lock()
	defer s.affinityMu.Unlock()
	pin, ok := s.affinity[key]
	if !ok || time.Now().After(pin.until) {
		delete(s.affinity, key)
		return ""
	}
	return pin.deployment
}

// 按聊天会话的一小时有效期写入粘滞部署。
// 参数 key（string）：缓存或配置表的键；deployment（string）：部署 id。空串表示当前没有钉住的部署。
// 返回：无。聊天会话的粘滞部署已按一小时有效期写下。
// 调用：仅在 affinity.go 内使用
// 测试：affinity_test.go
func (s *Server) affinitySet(key, deployment string) {
	s.affinitySetFor(key, deployment, affinityTTL)
}

const officialPinTTL = 7 * 24 * time.Hour

// 把官方任务 id 钉到创建时的部署，有效期七天。
// 参数 taskID（string）：官方任务 id。后续查询和计费靠它找回创建时的部署；deploymentID（string）：部署 id。空串表示当前没有钉住的部署。
// 返回：无。官方任务已钉到创建时的部署，有效期七天。
// 调用：dataplane/host.go、dataplane/official.go
// 测试：affinity_pin_test.go、bypass_logic_test.go、failure_log_test.go
func (s *Server) PinOfficial(taskID, deploymentID string) {
	s.affinitySetFor("official_task:v1:"+taskID, deploymentID, officialPinTTL)
}

// 按官方任务 id 取回创建时的部署。没有钉时为空串。
// 参数 taskID（string）：官方任务 id。后续查询和计费靠它找回创建时的部署。
// 返回 string（string）：读到的字符串。键不存在时为空串，调用方要把空串当成没有钉住。
// 调用：dataplane/host.go、dataplane/official.go
// 测试：affinity_pin_test.go、bypass_logic_test.go、failure_log_test.go
func (s *Server) OfficialDeployment(taskID string) string {
	dep := s.affinityGet("official_task:v1:" + taskID)
	if dep == "" && taskID != "" {
		// A poll for a task nobody pinned is worth a line: it is either a task
		// that expired after seven days, or an id that never came from here.
		logx.Debug("official task has no pinned deployment task=%s", taskID)
	}
	return dep
}

// 按给定的有效期写入粘滞键。键或部署为空时不做任何事。
// 参数 key（string）：缓存或配置表的键；deployment（string）：部署 id。空串表示当前没有钉住的部署；ttl（time.Duration）：一段时间。零值表示改用调用方约定的默认时长，例如会话一小时、官方任务七天。
// 返回：无。粘滞键已按给定有效期写下。有 Redis 时写 Redis，否则只留在进程内。键或部署为空时不写。
// 调用：仅在 affinity.go 内使用
// 测试：无直接单测
func (s *Server) affinitySetFor(key, deployment string, ttl time.Duration) {
	if s == nil || key == "" || deployment == "" {
		return
	}
	if s.Live != nil {
		s.Live.SetString(context.Background(), key, deployment, ttl)
	}
	s.affinityMu.Lock()
	defer s.affinityMu.Unlock()
	if s.affinity == nil {
		s.affinity = map[string]affinityPin{}
	}
	for oldKey, pin := range s.affinity {
		if time.Now().After(pin.until) {
			delete(s.affinity, oldKey)
		}
	}
	s.affinity[key] = affinityPin{deployment: deployment, until: time.Now().Add(ttl)}
}

// PinOfficialContext keeps only creation-time billing facts, with the task pin TTL.
// 参数 scope（string）：任务作用域键；facts（provider.TaskContext）：创建时的计费事实。
// 返回：无。
// 调用：数据面的官方任务创建流程。
// 测试：affinity_pin_test.go。
func (s *Server) PinOfficialContext(scope string, facts provider.TaskContext) {
	raw, err := json.Marshal(facts)
	if err == nil {
		s.affinitySetFor("official_context:v1:"+scope, string(raw), officialPinTTL)
	}
}

// OfficialContext reads creation-time facts for a pinned official task.
// 参数 scope（string）：任务作用域键。
// 返回 provider.TaskContext：保存的事实；缺失时为零值。
// 调用：数据面的官方任务轮询结算流程。
// 测试：affinity_pin_test.go。
func (s *Server) OfficialContext(scope string) provider.TaskContext {
	var facts provider.TaskContext
	_ = json.Unmarshal([]byte(s.affinityGet("official_context:v1:"+scope)), &facts)
	return facts
}
