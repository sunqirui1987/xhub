// Package keys creates, lists, updates, and deletes virtual keys. The plaintext appears only in the create or rotate response.
package keys

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/sunqirui1987/xhub/internal/auth"
	"github.com/sunqirui1987/xhub/internal/authz"
	"github.com/sunqirui1987/xhub/internal/gateway/templateauth"
	"github.com/sunqirui1987/xhub/internal/httpx"
	"github.com/sunqirui1987/xhub/internal/iam"
	"github.com/sunqirui1987/xhub/internal/logx"
)

var logTraceOnceGenerate sync.Once

// Generate 创建虚拟密钥，明文仅在本次响应返回；个人密钥可不绑定团队，指定团队时仍校验成员关系，服务密钥要求团队管理权限。
// 参数 s（Host）：Generate使用的数据面宿主；w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；r（*http.Request）：入站 HTTP 请求。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：gateway/keys/mount.go
// 测试：key_input_test.go、regression/personal_keys_test.go。
func Generate(s Host, w http.ResponseWriter, r *http.Request) {
	logTraceOnceGenerate.Do(func() { logx.Trace("enter keys.Generate") })

	httpx.SetCallID(w, httpx.CallID())
	p := s.RequireUser(w, r)
	if p == nil {
		return
	}
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && err != io.EOF {
		httpx.WriteError(w, 400, "invalid_request", "invalid json")
		return
	}
	if body == nil {
		body = map[string]any{}
	}
	in, err := keyInputFrom(body, p.UserID)
	if err != nil {
		httpx.WriteError(w, 400, "invalid_request", err.Error())
		return
	}
	obj := authz.Object{Type: authz.ObjectKey, TeamID: in.TeamID, ProjectID: in.ProjectID,
		OwnerType: in.OwnerType, OwnerUserID: in.UserID}
	if err := s.Authorize(r, p, authz.ActionKeyCreate, obj); err != nil {
		s.WriteAuthz(w, r, err)
		return
	}
	if err := templateauth.Selection(s, r, p, str(body["route_template_id"])); err != nil {
		s.WriteAuthz(w, r, err)
		return
	}
	k, plain, err := s.Identity().CreateKey(r.Context(), actorOf(p), in)
	if err != nil {
		s.WriteIAMError(w, r, err)
		return
	}
	httpx.WriteJSON(w, 200, Response(*k, plain, true))
}

// List 列出授权范围内的密钥；scope=personal 时仅查询登录用户本人名下的个人密钥，管理员也不例外。
// 参数 s（Host）：列出使用的数据面宿主；w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；r（*http.Request）：入站 HTTP 请求。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：gateway/keys/mount.go、gateway/models/list.go、gateway/models/mount.go、gateway/prefs/mount.go
// 测试：personal_test.go、regression/personal_keys_test.go、regression/pagination_test.go。查询先按权限在 SQL 收窄，再应用客户端筛选和分页，不返回明文。
func List(s Host, w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	p := s.RequireUser(w, r)
	if p == nil {
		return
	}
	sc, err := s.KeysScope(r, p)
	if err != nil {
		s.WriteAuthz(w, r, err)
		return
	}
	filter := keyFilter(sc)
	personal := r.URL.Query().Get("scope") == "personal"
	if personal {
		// 用户范围取自已认证身份，不能相信 user_id、创建者或团队筛选来决定归属。
		filter = personalFilter(filter, p)
	}
	rows, err := s.Identity().ListKeys(r.Context(), filter)
	if err != nil {
		s.WriteIAMError(w, r, err)
		return
	}
	rows, total, page, size, pages, err := personalPage(rows, r.URL.Query())
	if err != nil {
		httpx.WriteError(w, 400, "invalid_request", err.Error())
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, k := range rows {
		item := Response(k, "", false)
		if personal {
			// 个人列表用公开记录 ID 导航详情，遮罩前缀不是有效查询标识。
			item["token"] = k.ID
		}
		out = append(out, item)
	}
	httpx.WriteJSON(w, 200, map[string]any{
		"keys":         out,
		"total_count":  total,
		"current_page": page,
		"total_pages":  pages,
		"size":         size,
	})
}

// Info 读取单个虚拟密钥；个人页面的 scope=personal 额外验证归属，拒绝其他用户及服务密钥。
// 参数 s（Host）：信息使用的数据面宿主；w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；r（*http.Request）：入站 HTTP 请求。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：dataplane/log.go、dataplane/serve.go、gateway/engine.go、gateway/keys/mount.go
// 测试：personal_test.go、regression/personal_keys_test.go；越界详情返回 404，不泄露密钥存在性。
func Info(s Host, w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	p := s.RequireUser(w, r)
	if p == nil {
		return
	}
	token := r.URL.Query().Get("key")
	if r.Method == http.MethodPost {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if v, ok := body["key"].(string); ok {
			token = v
		}
	}
	if token == "" {
		httpx.WriteError(w, 400, "invalid_request", "key required")
		return
	}
	k, err := lookupKey(s, r, token)
	if err != nil {
		s.WriteIAMError(w, r, err)
		return
	}
	if r.URL.Query().Get("scope") == "personal" && !personalKeyVisible(p, k) {
		s.WriteAuthz(w, r, authz.ErrNotFound)
		return
	}
	// The guard re-reads the row, so a key the caller may not see is not found
	// rather than forbidden: existence itself is not disclosed.
	if err := s.Authorize(r, p, authz.ActionKeyRead, authz.Object{Type: authz.ObjectKey, ID: k.ID}); err != nil {
		s.WriteAuthz(w, r, err)
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"info": Response(*k, "", false)})
}

// Delete removes virtual keys. The plaintext can no longer call inference.
// 参数 s（Host）：删除使用的数据面宿主；w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；r（*http.Request）：入站 HTTP 请求。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：gateway/keys/mount.go、gateway/models/admin.go、gateway/models/mount.go、iam/keys.go
// 测试：无直接单测
func Delete(s Host, w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	p := s.RequireUser(w, r)
	if p == nil {
		return
	}
	ids, ok := authorizedKeys(s, w, r, p, authz.ActionKeyDelete)
	if !ok {
		return
	}
	n := 0
	for _, id := range ids {
		if err := s.Identity().DeleteKey(r.Context(), actorOf(p), id); err != nil {
			s.WriteIAMError(w, r, err)
			return
		}
		n++
	}
	httpx.WriteJSON(w, 200, map[string]any{"deleted": n})
}

// Block marks a virtual key blocked.
// 参数 s（Host）：拦截使用的数据面宿主；w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；r（*http.Request）：入站 HTTP 请求。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：gateway/keys/mount.go、gateway/models/admin.go、gateway/models/mount.go
// 测试：无直接单测
func Block(s Host, w http.ResponseWriter, r *http.Request) {
	setBlocked(s, w, r, true)
}

// Unblock clears the blocked flag.
// 参数 s（Host）：Unblock使用的数据面宿主；w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；r（*http.Request）：入站 HTTP 请求。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：gateway/keys/mount.go、gateway/models/admin.go、gateway/models/mount.go
// 测试：无直接单测
func Unblock(s Host, w http.ResponseWriter, r *http.Request) {
	setBlocked(s, w, r, false)
}

// setBlocked sets or clears the blocked status. A key the caller may not write returns 404.
// 参数 s（Host）：写入Blocked使用的数据面宿主；w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；r（*http.Request）：入站 HTTP 请求；blocked（bool）：为真时走blocked这一支。为假时保持原来的路径。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：仅在 generate.go 内使用。
// 测试：无直接单测
func setBlocked(s Host, w http.ResponseWriter, r *http.Request, blocked bool) {
	httpx.SetCallID(w, httpx.CallID())
	p := s.RequireUser(w, r)
	if p == nil {
		return
	}
	var body struct {
		Key string `json:"key"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body.Key == "" {
		httpx.WriteError(w, 400, "invalid_request", "key required")
		return
	}
	k, err := lookupKey(s, r, body.Key)
	if err != nil {
		s.WriteIAMError(w, r, err)
		return
	}
	if err := s.Authorize(r, p, authz.ActionKeyWrite, authz.Object{Type: authz.ObjectKey, ID: k.ID}); err != nil {
		s.WriteAuthz(w, r, err)
		return
	}
	status := iam.StatusActive
	if blocked {
		status = iam.StatusBlocked
	}
	if _, err := s.Identity().SetKeyStatus(r.Context(), actorOf(p), k.ID, status); err != nil {
		s.WriteIAMError(w, r, err)
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"blocked": blocked})
}

// Update changes a virtual key's name, narrowing, and limits. The plaintext stays the same, and a field the caller left out keeps its value.
// 参数 s（Host）：更新使用的数据面宿主；w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；r（*http.Request）：入站 HTTP 请求。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：gateway/keys/mount.go、gateway/models/admin.go、gateway/models/mount.go、gateway/prefs/mount.go
// 测试：无直接单测
func Update(s Host, w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	p := s.RequireUser(w, r)
	if p == nil {
		return
	}
	var body map[string]any
	_ = json.NewDecoder(r.Body).Decode(&body)
	token := str(body["key"])
	if token == "" {
		httpx.WriteError(w, 400, "invalid_request", "key required")
		return
	}
	k, err := lookupKey(s, r, token)
	if err != nil {
		s.WriteIAMError(w, r, err)
		return
	}
	if err := s.Authorize(r, p, authz.ActionKeyWrite, authz.Object{Type: authz.ObjectKey, ID: k.ID}); err != nil {
		s.WriteAuthz(w, r, err)
		return
	}
	if err := templateauth.Selection(s, r, p, str(body["route_template_id"])); err != nil {
		s.WriteAuthz(w, r, err)
		return
	}
	out, err := s.Identity().UpdateKey(r.Context(), actorOf(p), k.ID, patchFrom(k, body))
	if err != nil {
		s.WriteIAMError(w, r, err)
		return
	}
	httpx.WriteJSON(w, 200, Response(*out, "", false))
}

// Response is the public JSON for a virtual key. The plaintext is omitted when includePlain is false; the stored hash never appears, because the hash is what authentication compares and publishing it would leak the credential.
// 参数 k（iam.Key）：密钥行，含哈希、限额和归属，不含明文；plain（string）：明文口令或明文密钥。只在创建和轮换时出现，不会写入日志；includePlain（bool）：为真时响应里带上明文密钥。明文只在创建或轮换时出现。
// 返回 map[string]any（map[string]any）：给响应或报表用的 JSON 对象。键是前端已经约定的字段，缺键表示这项没有数据。
// 调用：gateway/keys/admin.go
// 测试：无直接单测
func Response(k iam.Key, plain string, includePlain bool) map[string]any {
	created := k.CreatedAt.UTC().Format(time.RFC3339)
	if k.CreatedAt.IsZero() {
		created = time.Now().UTC().Format(time.RFC3339)
	}
	updated := created
	if !k.UpdatedAt.IsZero() {
		updated = k.UpdatedAt.UTC().Format(time.RFC3339)
	}
	var expires any
	if k.ExpiresAt != nil {
		expires = k.ExpiresAt.UTC().Format(time.RFC3339)
	}
	var lastActive any
	if k.LastUsedAt != nil {
		lastActive = k.LastUsedAt.UTC().Format(time.RFC3339)
	}
	token := k.KeyPrefix + "..."
	if includePlain {
		token = plain
	}
	m := map[string]any{
		"token_id":   k.ID,
		"token":      token,
		"key_name":   k.Name,
		"key_alias":  k.Name,
		"key_prefix": k.KeyPrefix,
		"owner_type": k.OwnerType,
		"user_id":    emptyNil(deref(k.UserID)),
		"team_id":    emptyNil(k.TeamID),
		"project_id": emptyNil(deref(k.ProjectID)),
		"created_by": emptyNil(deref(k.CreatedBy)),
		"models":     nonNilStrings(k.Models),
		"max_budget": floatJSON(k.MaxBudget),
		// The router template this key selects. Without it the key's edit screen
		// shows an empty field, which reads as "selects nothing" - and an
		// operator who had set one would see their choice apparently lost, then
		// save over it.
		"route_template_id": emptyNil(deref(k.RouteTemplateID)),
		"spend":             k.Spend,
		"tpm_limit":         intJSON(k.TPMLimit),
		"rpm_limit":         intJSON(k.RPMLimit),
		"status":            k.Status,
		"blocked":           k.Status == iam.StatusBlocked,
		"expires":           expires,
		"last_active":       lastActive,
		"created_at":        created,
		"updated_at":        updated,
	}
	if includePlain {
		m["key"] = plain
	}
	return m
}

// actionKeys resolves the key tokens in a body and returns the IDs the caller is permitted to act on. A token that names nothing the caller may see, or one that is refused, has already produced the response and returns ok false.
// 参数 s（Host）：authorized密钥使用的数据面宿主；w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；r（*http.Request）：入站 HTTP 请求；p（*auth.Principal）：已经鉴权的调用方，含用户、团队、密钥哈希和别名；action（authz.Action）：要判定的动作，例如读日志、管理密钥或发起推理。
// 返回 []string（[]string）：调用方有权操作的密钥 id。有一个看不见或被拒绝时为 nil；bool（bool）：正文里的密钥令牌都解析成功、且调用方有权操作时返回真。有一个看不见或被拒绝时响应已写好并返回假。
// 调用：仅在 generate.go 内使用
// 测试：无直接单测
func authorizedKeys(s Host, w http.ResponseWriter, r *http.Request, p *auth.Principal, action authz.Action) ([]string, bool) {
	var body struct {
		Keys []string `json:"keys"`
		Key  string   `json:"key"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	tokens := body.Keys
	if body.Key != "" {
		tokens = append(tokens, body.Key)
	}
	if len(tokens) == 0 {
		httpx.WriteError(w, 400, "invalid_request", "key required")
		return nil, false
	}
	ids := make([]string, 0, len(tokens))
	for _, token := range tokens {
		k, err := lookupKey(s, r, token)
		if err != nil {
			s.WriteIAMError(w, r, err)
			return nil, false
		}
		if err := s.Authorize(r, p, action, authz.Object{Type: authz.ObjectKey, ID: k.ID}); err != nil {
			s.WriteAuthz(w, r, err)
			return nil, false
		}
		ids = append(ids, k.ID)
	}
	return ids, true
}

// lookupKey finds the key a token names. The token may be a plaintext credential, the stored hash, or the key ID, because the dashboard carries the ID while the LiteLLM-compatible clients carry the plaintext.
// 参数 s（Host）：查找密钥使用的数据面宿主；r（*http.Request）：入站 HTTP 请求；token（string）：签名或调用方带来的令牌。不会写入响应正文。
// 返回 *iam.Key（*iam.Key）：这个令牌对应的密钥。明文、已存哈希或密钥 id 都能对上。对不上时为 nil；error（error）：库读失败或没有这条密钥。nil 表示查到了。
// 调用：gateway/keys/admin.go
// 测试：无直接单测
func lookupKey(s Host, r *http.Request, token string) (*iam.Key, error) {
	ctx := r.Context()
	if k, err := s.Identity().KeyByHash(ctx, hashKeyToken(token)); err == nil {
		return k, nil
	} else if err != iam.ErrNotFound {
		return nil, err
	}
	return s.Identity().GetKey(ctx, token)
}

// keyFilter turns a key-listing scope into the store's filter. Every branch is fail-closed: a scope that names no row shape matches nothing rather than everything.
// 参数 sc（*authz.Scope）：密钥过滤使用的权限范围。
// 返回 KeyFilter（iam.KeyFilter）：按范围种类收成的密钥过滤条件。不认识的范围匹配不到任何行。
// 调用：gateway/keys/admin.go
// 测试：无直接单测
func keyFilter(sc *authz.Scope) iam.KeyFilter {
	switch sc.Kind {
	case authz.ScopeAllKeys:
		return iam.KeyFilter{}
	case authz.ScopeTeamKeys:
		return iam.KeyFilter{OwnOrService: true, UserID: sc.UserID, TeamIDs: sc.TeamIDs}
	default:
		if len(sc.KeyIDs) > 0 {
			return iam.KeyFilter{KeyIDs: sc.KeyIDs}
		}
		if sc.UserID == "" {
			return iam.KeyFilter{KeyIDs: []string{}}
		}
		return iam.KeyFilter{OwnerType: iam.OwnerPersonal, UserID: sc.UserID}
	}
}

// keyInputFrom 解析创建请求，默认创建调用方个人密钥；个人密钥可省略团队，服务密钥和项目密钥必须指定团队。
// 参数 body（map[string]any）：已经解析的 JSON 对象。缺字段表示上游或调用方没有给这项；defaultUserID（string）：密钥输入来源使用的default用户标识。空串表示调用方没有提供这项。
// 返回 KeyInput（iam.KeyInput）：从正文读出的新密钥。属主没写时默认是调用方的个人密钥；error（error）：属主或必填项不合法。nil 表示可以创建。
// 调用：Generate、gateway/keys/admin.go；不产生持久化副作用，非法归属或期限返回错误。
// 测试：key_input_test.go。
func keyInputFrom(body map[string]any, defaultUserID string) (iam.KeyInput, error) {
	in := iam.KeyInput{
		OwnerType: str(body["owner_type"]),
		UserID:    str(body["user_id"]),
		TeamID:    str(body["team_id"]),
		ProjectID: str(body["project_id"]),
		Name:      str(body["key_alias"]),
		Models:    keyModels(body["models"]),
		MaxBudget: parseFloat(body["max_budget"]),
		TPMLimit:  parseInt(body["tpm_limit"]),
		RPMLimit:  parseInt(body["rpm_limit"]),
		// The selection must be read here as well as on update. It was only on
		// update, so a key created with a template silently got none and the
		// first request through it routed by the team's settings instead -
		// a difference nothing in the console showed.
		RouteTemplateID: routeTemplatePatch(body),
	}
	if in.Name == "" {
		in.Name = str(body["key_name"])
	}
	if in.OwnerType == "" {
		in.OwnerType = iam.OwnerPersonal
		if in.UserID == "" {
			in.UserID = defaultUserID
		}
	}
	if in.OwnerType == iam.OwnerPersonal && in.UserID == "" {
		return in, errUserRequired
	}
	if in.OwnerType == iam.OwnerService {
		in.UserID = ""
	}
	if in.OwnerType != iam.OwnerPersonal && in.OwnerType != iam.OwnerService {
		return in, errString("invalid owner_type")
	}
	if in.TeamID == "" && (in.OwnerType == iam.OwnerService || in.ProjectID != "") {
		return in, errTeamRequired
	}
	exp, err := iam.ParseExpiry(str(body["duration"]))
	if err != nil {
		return in, err
	}
	in.ExpiresAt = exp
	return in, nil
}

// keyModels 将控制台“全部模型”选项转成继承范围，不把选项标识误存为模型名称。
// 参数 raw 为请求 models；返回具体模型名单，全选返回空名单；供创建和更新解析使用，无副作用。
// 团队密钥仍由 IAM 与团队/项目模型范围取交集，因此全选不会扩大团队授权。
func keyModels(raw any) []string {
	models := stringList(raw)
	for _, model := range models {
		if model == "all-proxy-models" || model == "all-team-models" {
			return []string{}
		}
	}
	return models
}

// patchFrom 将部分请求合并到已有密钥；缺字段保留原值，“全部模型”转成继承当前授权范围。
// 参数 cur（*iam.Key）：密钥行，含哈希、限额和归属，不含明文；body（map[string]any）：已解析或原始的 JSON。
// 返回 KeyInput（iam.KeyInput）：在已存密钥上套用正文里出现的字段。正文没写的字段保持原值，不会被写成空。
// 调用：gateway/keys/admin.go
// 测试：key_input_test.go。
func patchFrom(cur *iam.Key, body map[string]any) iam.KeyInput {
	in := iam.KeyInput{Name: cur.Name, Models: cur.Models,
		MaxBudget: cur.MaxBudget, TPMLimit: cur.TPMLimit, RPMLimit: cur.RPMLimit}
	in.RouteTemplateID = routeTemplatePatch(body)
	if v, ok := body["key_alias"].(string); ok {
		in.Name = v
	}
	if _, ok := body["models"]; ok {
		in.Models = keyModels(body["models"])
	}
	if _, ok := body["max_budget"]; ok {
		in.MaxBudget = parseFloat(body["max_budget"])
	}
	if _, ok := body["tpm_limit"]; ok {
		in.TPMLimit = parseInt(body["tpm_limit"])
	}
	if _, ok := body["rpm_limit"]; ok {
		in.RPMLimit = parseInt(body["rpm_limit"])
	}
	if _, ok := body["duration"]; ok {
		if exp, err := iam.ParseExpiry(str(body["duration"])); err == nil {
			in.ExpiresAt = exp
		}
	}
	return in
}

// actorOf is the audit actor for a key-level write.
// 参数 p（*auth.Principal）：已经解析的调用方，含用户、团队和密钥。
// 返回 Actor（iam.Actor）：密钥写入的审计操作者。ID 是用户 id，Kind 是身份种类。
// 调用：gateway/keys/admin.go。
// 测试：无直接单测
func actorOf(p *auth.Principal) iam.Actor {
	return iam.Actor{ID: p.UserID, Kind: string(p.Kind)}
}

// nonNilStrings keeps the JSON list shape stable: a key with no narrowing emits an empty list rather than null.
// 参数 in（[]string）：内列表。空切片表示没有可处理的项。
// 返回 []string（[]string）：非空字符串。没有匹配时为 nil 或空切片，调用方按长度判断。
// 调用：仅在 generate.go 内使用。
// 测试：无直接单测
func nonNilStrings(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
}

var (
	errUserRequired = errString("user_id required for a personal key")
	errTeamRequired = errString("team_id required")
)

// errString is a plain request error, so a bad body is a 400 and not a 500.
type errString string

// 实现 error 接口，返回写进日志或 HTTP 错误体的文本。
// 参数：无。
// 返回 string（string）：error 接口的文本，给日志和 HTTP 错误体使用。
// 调用：生成密钥的处理函数在缺少用户或团队时返回它，经 error 接口读取。
// 测试：无直接单测
func (e errString) Error() string { return string(e) }

// routeTemplatePatch reads the template selection out of a key body.
//
// It is a two-level optional: an absent field leaves the current selection
// alone, and an explicit null or empty string clears it, which puts the key back
// on inheriting from its team. Reading it into a plain string would collapse
// "not sent" into "clear it", and every unrelated key edit would drop the
// selection.
//
// 参数 body map string any 已经解析的 JSON 对象。
// 返回 **string 字段不在正文里时为 nil，在正文里时指向内层指针，空值表示清除。
// 调用: keyInputFrom、patchFrom。
// 测试 无直接单测
func routeTemplatePatch(body map[string]any) **string {
	raw, ok := body["route_template_id"]
	if !ok {
		return nil
	}
	var inner *string
	if value := strings.TrimSpace(str(raw)); value != "" {
		inner = &value
	}
	return &inner
}
