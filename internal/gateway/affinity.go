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
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/sunqirui1987/xhub/internal/auth"
	"github.com/sunqirui1987/xhub/internal/dataplane"
	"github.com/sunqirui1987/xhub/internal/logx"
)

// legacyClaudeUserID is Claude Code's older metadata.user_id:
// user_{device}_account_{account}_session_{uuid}.
var legacyClaudeUserID = regexp.MustCompile(`^user_[a-fA-F0-9]{64}_account_[a-fA-F0-9-]*_session_([a-fA-F0-9-]{36})$`)

const affinityTTL = time.Hour

// PlanRoute resolves the session and any deployment already pinned to it. previous_response_id wins, then an explicit session, then a stable prompt prefix.
// 参数 r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文；alias（string）：对外模型名，用来选部署和记用量；body（map[string]any）：已经解析的 JSON 对象。缺字段表示上游或调用方没有给这项；p（*auth.Principal）：已经解析的调用方，含用户、团队和密钥。
// 返回 RoutePlan（dataplane.RoutePlan）：这次请求的会话和已经钉住的部署。previous_response_id 优先，其次显式会话，再其次提示前缀。没有钉时 Pinned 为空。
// 调用：dataplane/host.go、dataplane/official.go、dataplane/serve.go
// 测试：bypass_logic_test.go、failure_log_test.go、log_completeness_test.go
func (s *Server) PlanRoute(r *http.Request, alias string, body map[string]any, p *auth.Principal) dataplane.RoutePlan {
	plan := dataplane.RoutePlan{Alias: alias, Caller: callerScope(p)}
	if body != nil {
		if prev := strings.TrimSpace(asString(body["previous_response_id"])); prev != "" {
			if id := s.affinityGet("deployment_affinity:v1:response:" + prev); id != "" {
				plan.Pinned = id
			}
		}
	}
	plan.SessionID = sessionID(r, body)
	if plan.Pinned == "" && plan.SessionID != "" && plan.Caller != "" {
		plan.Pinned = s.affinityGet(sessionPinKey(alias, plan.Caller, plan.SessionID))
	}
	return plan
}

// CommitRoute remembers which deployment served this session and, when the response has an id, which deployment produced it.
// 参数 plan（dataplane.RoutePlan）：提交路由使用的RoutePlan；deploymentID（string）：部署 id。空串表示当前没有钉住的部署；responseID（string）：提交路由使用的响应标识。空串表示调用方没有提供这项。
// 返回：无。这次会话用过的部署已记下。响应带 id 时，产出它的部署也记下。部署 id 为空时不写。
// 调用：dataplane/host.go、dataplane/serve.go
// 测试：bypass_logic_test.go、failure_log_test.go、log_completeness_test.go
func (s *Server) CommitRoute(plan dataplane.RoutePlan, deploymentID, responseID string) {
	if deploymentID == "" {
		return
	}
	if plan.SessionID != "" && plan.Caller != "" {
		s.affinitySet(sessionPinKey(plan.Alias, plan.Caller, plan.SessionID), deploymentID)
	}
	if responseID != "" {
		s.affinitySet("deployment_affinity:v1:response:"+responseID, deploymentID)
	}
}

// 拼出会话粘滞的 Redis 键。调用方只取哈希前 8 字节，避免键里出现完整密钥。
// 参数 alias（string）：对外模型名；caller（string）：调用方范围，一般是密钥哈希或用户 id，用来隔离粘滞键；sessionID（string）：会话 id。相同会话应钉在同一上游部署。
// 返回 string（string）：会话粘滞的 Redis 键，含对外名、调用方哈希和会话 id。
// 调用：仅在 affinity.go 内使用
// 测试：affinity_test.go
func sessionPinKey(alias, caller, sessionID string) string {
	sum := sha256.Sum256([]byte(caller))
	return "deployment_affinity:v1:session:" + alias + ":" + hex.EncodeToString(sum[:8]) + ":" + sessionID
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
	if m := legacyClaudeUserID.FindStringSubmatch(raw); len(m) == 2 {
		return m[1]
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

// 判断这个官方任务是否已经记过用量。
// 参数 taskID（string）：官方任务 id。后续查询和计费靠它找回创建时的部署。
// 返回 bool（bool）：这个官方任务已经记过一次用量时为真。没有标记时为假，下一次带用量的请求才记账。
// 调用：dataplane/host.go、dataplane/official.go
// 测试：affinity_pin_test.go、bypass_logic_test.go、failure_log_test.go
func (s *Server) OfficialBilled(taskID string) bool {
	return s.affinityGet("official_billed:v1:"+taskID) != ""
}

// 标记这个官方任务已经记过用量，避免创建和后续查询重复扣费。
// 参数 taskID（string）：官方任务 id。后续查询和计费靠它找回创建时的部署。
// 返回：无。这个官方任务已标成扣过一次费，有效期与任务钉相同。
// 调用：dataplane/host.go、dataplane/official.go
// 测试：affinity_pin_test.go、bypass_logic_test.go、failure_log_test.go
func (s *Server) MarkOfficialBilled(taskID string) {
	s.affinitySetFor("official_billed:v1:"+taskID, "1", officialPinTTL)
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
	s.affinity[key] = affinityPin{deployment: deployment, until: time.Now().Add(ttl)}
}
