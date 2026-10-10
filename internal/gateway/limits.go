// limits.go is the gate in front of dataplane.Serve. dataPlane is the wrapper
// the catalog handlers call. Budget, RPM, and TPM checks live in this file
// because they need the key, the team, and Redis. The send loop does not.

package gateway

import (
	"context"
	"errors"
	"github.com/sunqirui1987/xhub/internal/provider"
	"maps"
	"net/http"
	"strings"
	"time"

	"github.com/sunqirui1987/xhub/internal/auth"
	"github.com/sunqirui1987/xhub/internal/authz"
	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/dataplane"
	modelaccess "github.com/sunqirui1987/xhub/internal/gateway/models"
	"github.com/sunqirui1987/xhub/internal/httpx"
	"github.com/sunqirui1987/xhub/internal/iam"
	"github.com/sunqirui1987/xhub/internal/live"
	"github.com/sunqirui1987/xhub/internal/llm"
	"github.com/sunqirui1987/xhub/internal/logx"
	"sync"
)

var logTraceOnceLimits sync.Once

// dataPlane hands this inference call to dataplane.Serve. Identity, budget, and the upstream loop are not reimplemented in this wrapper.
// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态码和正文写在这里；r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文；op（string）：操作名或 call_type，写入用量行并选择协议。
// 调用：gateway/ingress.go、gateway/wire.go
// 测试：dial_log_test.go、guardrail_block_test.go
// 返回：无。推理的状态码和正文由数据面写进 w，这里不再包一层。
func (s *Server) dataPlane(w http.ResponseWriter, r *http.Request, op string) {
	logTraceOnceLimits.Do(func() { logx.Trace("enter gateway.dataPlane") })
	logx.Debug("process %s %s step=dataplane op=%s", r.Method, r.URL.Path, op)
	dataplane.Serve(s, w, r, op)
}

// estimateTokens calls dataplane.EstimateTokens so budget and TPM checks have an upper bound.
// 参数 body（map[string]any）：已经解析的 JSON 对象。缺字段表示上游或调用方没有给这项。
// 返回 int（int）：从 JSON 或查询参数转成的整数。类型不符或缺失时为 0，不 panic。
// 调用：仅在 limits.go 内使用
// 测试：无直接单测
func estimateTokens(body map[string]any) int { return dataplane.EstimateTokens(body) }

// withCredential fills deployment parameters from the credential store using the deployment's credential name. With no name the deployment is returned unchanged.
var (
	errCredentialUnavailable = errors.New("credential_unavailable")
	errCredentialInvalid     = errors.New("credential_invalid")
)

// withCredential 读取当前连接目录并校验模型能力，再补上 api_key 和 api_base。
// 没有连接时使用部署的内联目录；元数据独立复制，不写入上游参数。
// 参数 dep（config.ModelEntry）：含上游模型、执行传输、连接名或内联连接的部署。
// 返回：填好 api_key 和 api_base 的副本。凭证库不可用或凭证无效时返回错误。
// 调用：gateway/wire.go
// 测试：credential_catalog_test.go、regression/supplier_catalog_test.go。
func (s *Server) withCredential(dep config.ModelEntry) (config.ModelEntry, error) {
	name := dep.ParamString("litellm_credential_name", "")
	var values map[string]any
	catalogID, _ := dep.ModelInfo["catalog_id"].(string)
	if name != "" {
		if s.Store == nil {
			return dep, errCredentialUnavailable
		}
		rec, err := s.Store.GetKV("credentials", name)
		if err != nil {
			logx.Error("credential lookup failed name=%s err=%v", name, err)
			return dep, errCredentialUnavailable
		} else {
			meta, _ := rec["credential_info"].(map[string]any)
			catalogID, _ = meta["catalog_id"].(string)
			values, _ = rec["credential_values"].(map[string]any)
			if values == nil {
				logx.Error("credential invalid name=%s reason=missing credential_values", name)
				return dep, errCredentialInvalid
			}
		}
	}
	// 每次请求读取当前目录，防止连接编辑后沿用旧能力；元数据不进入上游参数。
	if err := provider.ValidateCatalogBinding(catalogID, dep); err != nil {
		return dep, errCredentialInvalid
	}
	out := dep
	out.ModelInfo = maps.Clone(dep.ModelInfo)
	if out.ModelInfo == nil {
		out.ModelInfo = map[string]any{}
	}
	out.ModelInfo["catalog_id"] = catalogID
	out.LiteLLMParams = llm.Hydrate(dep.LiteLLMParams, values)
	return out, nil
}

// enforceIdentityLimits checks the model allow-list, budget, and rate. On rejection it has already written the response and returns false. The budget is checked from the narrowest scope outwards, so the
//
//	refusal names the scope that is actually exhausted: the owner user, then the key, its project, its team, and finally the team's organization. Each scope compares its stored spend plus the spend still
//	hot in Redis against its ceiling.
//
// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；path（string）：enforceIdentityLimits要定位的路径。可能是 URL，也可能是字段路径；p（*auth.Principal）：已经鉴权的调用方，含用户、团队、密钥哈希和别名；alias（string）：对外模型名；est（int）：enforceIdentityLimits使用的整数。零表示没有这项或尚未计数。
// 返回 bool（bool）：模型允许名单、预算和速率都通过时返回真。拒绝时响应已经写好，并返回假。
// 调用：gateway/wire.go
// 测试：无直接单测
func (s *Server) enforceIdentityLimits(w http.ResponseWriter, path string, p *auth.Principal, alias string, est int) bool {
	if s.IAM == nil {
		httpx.WriteTypedError(w, path, http.StatusServiceUnavailable, "authz_unavailable", "authorization is temporarily unavailable")
		return false
	}
	ctx := context.Background()
	if p.Kind == authz.KindSession {
		user, err := s.IAM.GetUser(ctx, p.UserID)
		if err != nil {
			httpx.WriteTypedError(w, path, 401, "invalid_request_error", "user not found")
			return false
		}
		if !user.Active() {
			httpx.WriteTypedError(w, path, 401, "invalid_request_error", "user not found or blocked")
			return false
		}
		if overBudget(user.MaxBudget, user.Spend, s.hotSpendRef("user", user.ID)) {
			httpx.WriteTypedError(w, path, 429, "budget_exceeded", "User budget has been exceeded")
			return false
		}
		// A session has no key row, so the chain that resolves a template for a
		// key does not run for it. The console's playground is a session caller,
		// and it would otherwise always route by the platform default no matter
		// what its team selected.
		s.resolveSessionRouteTemplate(ctx, p)
	}
	if p.Key != nil {
		if err := s.keyBudgetOK(ctx, p); err != nil {
			budgetRefusal(w, path, err)
			return false
		}
	}
	if alias != "" && !modelaccess.AllowsModel(s, ctx, p, p.TeamID, alias) {
		httpx.WriteTypedError(w, path, 401, "invalid_request_error", "model not in allowed model list")
		return false
	}
	return p.Key == nil || s.enforceRateLimits(w, path, p, est)
}

// resolveSessionRouteTemplate records the router template a console session runs
// under, on the same principal fields the key path fills.
//
// A session has memberships rather than one team. The rule for choosing is the
// one the usage row already uses: a caller in exactly one team is filed there,
// and a caller in several is left alone rather than guessed at. Applying a
// template from an arbitrarily picked team would route one person's requests by
// another team's settings, which is worse than falling back to the platform
// default - the fallback is at least a setting somebody chose deliberately.
//
// The team row is read here rather than reusing the memberships list because the
// memberships carry the organization but not the team's template column.
// 参数 ctx（context.Context）：上下文，取消时停止；p（*auth.Principal）：已经解析的会话调用方。
// 返回：无。写进 Principal 的 OrgID、RouteTemplateID 和 RouteTemplateSource。
// 调用：enforceIdentityLimits。
// 测试：route_settings_test.go
func (s *Server) resolveSessionRouteTemplate(ctx context.Context, p *auth.Principal) {
	memberships, err := s.IAM.MemberTeams(ctx, p.UserID)
	if err != nil || len(memberships) != 1 {
		return
	}
	team, err := s.IAM.GetTeam(ctx, memberships[0].TeamID)
	if err != nil || team == nil {
		return
	}
	p.TeamID = team.ID
	p.OrgID = team.OrganizationID
	if team.RouteTemplateID != nil {
		p.RouteTemplateID = strings.TrimSpace(*team.RouteTemplateID)
		p.RouteTemplateSource = "team"
	}
	if p.RouteTemplateID != "" {
		return
	}
	org, err := s.IAM.GetOrg(ctx, team.OrganizationID)
	if err != nil || org == nil || org.RouteTemplateID == nil {
		return
	}
	p.RouteTemplateID = strings.TrimSpace(*org.RouteTemplateID)
	p.RouteTemplateSource = "organization"
}

// keyBudgetOK 沿密钥归属链检查实时额度；独立个人密钥检查用户与密钥额度，团队密钥继续检查项目、团队和组织。
// 无团队时保留密钥选择的路由模板，否则使用平台默认；指定的父级丢失仍返回错误，不能跳过额度。
// 同时记录团队所属组织及路由模板；按密钥、团队、组织顺序选择第一个明确配置，复用额度查询避免重复读取。
// 参数 ctx（context.Context）：上下文，取消或超时时停止后续工作；p（*auth.Principal）：已经解析的调用方，含用户、团队和密钥。
// 返回 error（error）：失败原因，nil 表示这一步成功。
// 调用：仅在 limits.go 内使用
// 测试：limits_personal_test.go、regression/teamless_keys_test.go。
func (s *Server) keyBudgetOK(ctx context.Context, p *auth.Principal) error {
	k, err := s.IAM.GetKey(ctx, p.KeyID)
	if err != nil {
		return errKeyGone
	}
	p.Key = k
	if k.Status != iam.StatusActive {
		return errKeyUnusable
	}
	if k.ExpiresAt != nil && !time.Now().Before(*k.ExpiresAt) {
		return errKeyUnusable
	}
	// The key is the narrowest scope, so its selection is tried first and the
	// levels below only fill in when it names none.
	if k.RouteTemplateID != nil {
		p.RouteTemplateID = strings.TrimSpace(*k.RouteTemplateID)
		p.RouteTemplateSource = "key"
	}
	if k.UserID != nil {
		owner, err := s.IAM.GetUser(ctx, *k.UserID)
		if err != nil {
			return errKeyGone
		}
		if !owner.Active() {
			return errKeyUnusable
		}
		if overBudget(owner.MaxBudget, owner.Spend, s.hotSpendRef("user", owner.ID)) {
			return errBudget{scope: "User"}
		}
	}
	if overBudget(k.MaxBudget, k.Spend, s.hotSpendRef("key", p.Hash)) {
		return errBudget{scope: "Key"}
	}
	if k.ProjectID != nil && *k.ProjectID != "" {
		project, err := s.IAM.GetProject(ctx, *k.ProjectID)
		if err != nil {
			return errKeyGone
		}
		if overBudget(project.MaxBudget, project.Spend, s.hotSpendRef("project", project.ID)) {
			return errBudget{scope: "Project"}
		}
	}
	if k.TeamID == "" {
		if k.OwnerType != iam.OwnerPersonal || k.UserID == nil || k.ProjectID != nil {
			return errKeyUnusable
		}
		return nil
	}
	team, err := s.IAM.GetTeam(ctx, k.TeamID)
	if err != nil {
		return errKeyGone
	}
	if overBudget(team.MaxBudget, team.Spend, s.hotSpendRef("team", team.ID)) {
		return errBudget{scope: "Team"}
	}
	p.OrgID = team.OrganizationID
	// The team is the next scope up. A key that named no template leaves the
	// field empty, so this only takes effect when the key named none.
	if p.RouteTemplateID == "" && team.RouteTemplateID != nil {
		p.RouteTemplateID = strings.TrimSpace(*team.RouteTemplateID)
		p.RouteTemplateSource = "team"
	}
	org, err := s.IAM.GetOrg(ctx, team.OrganizationID)
	if err != nil {
		return errKeyGone
	}
	if overBudget(org.MaxBudget, org.Spend, s.hotSpendRef("org", org.ID)) {
		return errBudget{scope: "Organization"}
	}
	// And the organization is the widest scope in the chain.
	if p.RouteTemplateID == "" && org.RouteTemplateID != nil {
		p.RouteTemplateID = strings.TrimSpace(*org.RouteTemplateID)
		p.RouteTemplateSource = "organization"
	}
	return nil
}

// errKeyGone and errKeyUnusable separate "the credential no longer resolves" from
// "a scope is over budget", because only the first is the caller's problem.
var (
	errKeyGone     = errors.New("key not found")
	errKeyUnusable = errors.New("key blocked or expired")
)

// errBudget names the exhausted scope. The message mirrors LiteLLM's wording,
// which keeps the "<Scope> budget has been exceeded" text the console shows.
type errBudget struct{ scope string }

// 实现 error 接口，返回写进日志或 HTTP 错误体的文本。
// 参数：无。
// 返回 string（string）：error 接口的文本，给日志和 HTTP 错误体使用。
// 调用：预算检查在某个范围用尽时返回它，budgetRefusal 读取这段文本。
// 测试：无直接单测
func (e errBudget) Error() string { return e.scope + " budget has been exceeded" }

// budgetRefusal writes the response for a failed ownership walk.
// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态码和正文写在这里；path（string）：预算Refusal要定位的路径。可能是 URL，也可能是字段路径；err（error）：失败原因，nil 表示这一步成功。
// 返回：无。预算用尽时写 429 和 JSON 错误。
// 调用：仅在 limits.go 内使用
// 测试：无直接单测
func budgetRefusal(w http.ResponseWriter, path string, err error) {
	var over errBudget
	if errors.As(err, &over) {
		httpx.WriteTypedError(w, path, 429, "budget_exceeded", over.Error())
		return
	}
	httpx.WriteTypedError(w, path, 401, "invalid_request_error", err.Error())
}

// overBudget reports a scope that has reached its ceiling. Without a ceiling the scope is unlimited, and stored plus hot spend is what the caller has used.
// 参数 ceiling（*float64）：over预算使用的float64；spent（float64）：over预算使用的小数。0 表示没有费用或尚未计价；hot（float64）：over预算使用的小数。0 表示没有费用或尚未计价。
// 返回 bool（bool）：这个主体的花费已经达到上限时返回真。没有上限时视为不限。
// 调用：仅在 limits.go 内使用
// 测试：无直接单测
func overBudget(ceiling *float64, spent, hot float64) bool {
	return ceiling != nil && spent+hot >= *ceiling
}

// hotSpend is spend still sitting in Redis. Without Redis it is 0 and the budget check uses PostgreSQL only.
// 参数 id（string）：热花费使用的主键。空串表示调用方没有指定记录。
// 返回 float64（float64）：热花费。缺失时为 0，不要把它理解成免费除非调用方另有约定。
// 调用：仅在 limits.go 内使用
// 测试：无直接单测
func (s *Server) hotSpend(id string) float64 {
	if s.Live == nil {
		return 0
	}
	return s.Live.HotSpend(id)
}

// 读取这个主体还没刷进数据库的热花费。没有 Redis 时为 0。
// 参数 kind（string）：分类名，用来选择限额主体、日志类型或官方端点；id（string）：热花费引用使用的主键。空串表示调用方没有指定记录。
// 返回 float64（float64）：热花费引用。没有计数或类型不符时为 0。
// 调用：仅在 limits.go 内使用
// 测试：无直接单测
func (s *Server) hotSpendRef(kind, id string) float64 {
	if s.Live == nil {
		return 0
	}
	return s.Live.HotSpend(live.SpendRef(kind, id))
}

// enforceRateLimits uses the Redis minute bucket when Redis is set, otherwise a process-local sliding window. Over the limit it writes 429 and returns false.
// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；path（string）：enforce单价Limits要定位的路径。可能是 URL，也可能是字段路径；p（*auth.Principal）：已经鉴权的调用方，含用户、团队、密钥哈希和别名；est（int）：enforce单价Limits使用的整数。零表示没有这项或尚未计数。
// 返回 bool（bool）：RPM 和 TPM 都在限额内时返回真。超限时写 429 并返回假。
// 调用：仅在 limits.go 内使用
// 测试：无直接单测
func (s *Server) enforceRateLimits(w http.ResponseWriter, path string, p *auth.Principal, est int) bool {
	if p.Key == nil {
		return true
	}
	if s.Live != nil {
		return s.enforceRedisRateLimits(w, path, p, est)
	}
	now := time.Now()
	win := now.Add(-time.Minute)
	s.mu.Lock()
	defer s.mu.Unlock()
	hash := p.Hash
	if p.Key.RPMLimit != nil {
		var keep []time.Time
		for _, t := range s.rpmHits[hash] {
			if t.After(win) {
				keep = append(keep, t)
			}
		}
		s.rpmHits[hash] = keep
		if int64(len(keep)) >= int64(*p.Key.RPMLimit) {
			httpx.WriteTypedError(w, path, 429, "rate_limit", "rpm_limit exceeded")
			return false
		}
		s.rpmHits[hash] = append(keep, now)
	}
	if p.Key.TPMLimit != nil {
		var keep []tokHit
		sum := 0
		for _, h := range s.tpmHits[hash] {
			if h.t.After(win) {
				keep = append(keep, h)
				sum += h.n
			}
		}
		s.tpmHits[hash] = keep
		if *p.Key.TPMLimit == 0 || int64(sum+est) > int64(*p.Key.TPMLimit) {
			httpx.WriteTypedError(w, path, 429, "rate_limit", "tpm_limit exceeded")
			return false
		}
		s.tpmHits[hash] = append(keep, tokHit{t: now, n: est})
	}
	return true
}

// enforceRedisRateLimits checks RPM and TPM against the Redis minute bucket. A limit of 0 is treated as already exceeded.
// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；path（string）：enforceRedis单价Limits要定位的路径。可能是 URL，也可能是字段路径；p（*auth.Principal）：已经鉴权的调用方，含用户、团队、密钥哈希和别名；est（int）：enforceRedis单价Limits使用的整数。零表示没有这项或尚未计数。
// 返回 bool（bool）：Redis 分钟桶里的 RPM 和 TPM 都未超限时返回真。限额为 0 视为已经超限。
// 调用：仅在 limits.go 内使用
// 测试：无直接单测
func (s *Server) enforceRedisRateLimits(w http.ResponseWriter, path string, p *auth.Principal, est int) bool {
	if p.Key.RPMLimit != nil {
		n, err := s.Live.HitRPM(p.Hash)
		if err != nil {
			httpx.WriteTypedError(w, path, http.StatusServiceUnavailable, "rate_limit_unavailable", "rate limiting is temporarily unavailable")
			return false
		}
		if *p.Key.RPMLimit == 0 || n > int64(*p.Key.RPMLimit) {
			httpx.WriteTypedError(w, path, 429, "rate_limit", "rpm_limit exceeded")
			return false
		}
	}
	if p.Key.TPMLimit != nil {
		n, err := s.Live.HitTPM(p.Hash, est)
		if err != nil {
			httpx.WriteTypedError(w, path, http.StatusServiceUnavailable, "rate_limit_unavailable", "rate limiting is temporarily unavailable")
			return false
		}
		if *p.Key.TPMLimit == 0 || n > int64(*p.Key.TPMLimit) {
			httpx.WriteTypedError(w, path, 429, "rate_limit", "tpm_limit exceeded")
			return false
		}
	}
	return true
}
