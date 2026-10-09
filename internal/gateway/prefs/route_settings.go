package prefs

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/sunqirui1987/xhub/internal/provider"
	"math"
	"strings"
	"sync"

	"github.com/sunqirui1987/xhub/internal/iam"
	"github.com/sunqirui1987/xhub/internal/logx"
	"github.com/sunqirui1987/xhub/internal/router"
)

var logTraceOnceRouteSettings sync.Once

// RouteSettings is the router settings one request runs under, and where they
// came from.
//
// It is the single answer to "which settings apply to this call". Everything in
// the request path that used to read the process-global document - the strategy,
// the retry count, the timeout, and the cooldown thresholds -
// reads this instead, so a scope that selected a template and a scope that did
// not are routed and billed by the same code with different inputs.
//
// Settings is a whole document, not a merge. See Resolve for why.
type RouteSettings struct {
	Settings map[string]any
	Err      error
	// TemplateID and TemplateName are empty when the platform default applies.
	TemplateID   string
	TemplateName string
	// Source is where the settings came from: "key", "team", "organization" or
	// "platform". The console shows it, because a scope that selected nothing and
	// a scope whose parent selected something look identical without it.
	Source string
	// RuleSource 说明命中的策略层级，不包含凭据。
	RuleSource string
}

// ScopeRef names one level of the inheritance chain.
type ScopeRef struct {
	// Kind is "key", "team" or "organization". Anything else is ignored, because
	// the name reaches SQL in the store.
	Kind string
	// ID is the row in that scope. An empty id means this level is not part of
	// the request, which is how an anonymous or teamless caller is expressed.
	ID string
}

// PlatformSource is the Source value for a request that selected no template.
const PlatformSource = "platform"

// ScopeLookup is what the resolver needs in order to walk the chain. It is an
// interface so the request path can pass a per-request cache rather than hitting
// the database once per level per attempt.
type ScopeLookup interface {
	// TemplateFor returns the router template id one scope selects, or an empty
	// string when it selects none.
	//
	// An unknown scope or id returns an empty string rather than an error: a
	// deleted row means "selects nothing", which is the same state as never
	// having chosen, and the two must not be distinguished by the caller.
	// 参数 kind（string）：key、team 或 organization；id（string）：那一行。
	// 返回 string（string）：选用的模板 id，没选时为空串。
	// 调用：prefs.Resolve。
	// 测试：route_settings_test.go
	TemplateFor(kind, id string) string
	// Load returns one template by id, or nil when it is gone.
	// 参数 id（string）：模板 id。
	// 返回 *iam.RouteTemplate（*iam.RouteTemplate）：这一行，已经被删时为 nil。
	// 调用：prefs.Resolve。
	// 测试：route_settings_test.go
	Load(id string) *iam.RouteTemplate
}

// Resolve picks the router settings for one request from an ordered chain and
// reports where they came from.
//
// chain runs from the narrowest scope outward, and the FIRST scope that selects a
// template wins outright:
//
//	key > team > organization > platform default
//
// The winning template's document IS the result - it is not stitched field by
// field with anything. A merge would have to answer "whose num_retries wins when
// the organization's template and the team's both set one", "whose weights do
// these percentages normalise against", and "what is strategy A composed with
// strategy B" - questions with no good answers, whose answers also change a
// team's behaviour whenever the level above it is edited. Replacing has a rule
// that fits in one sentence: the settings that apply are the ones the winning
// scope chose.
//
// That rule puts a requirement on how templates are written, and the create path
// meets it: a new template is seeded from the platform default rather than from
// an empty document, so every template in the table is a complete document. A
// template that omits num_retries means one attempt; omitting timeout means
// sixty seconds, matching the typed readers below.
//
// A scope that selects nothing inherits from the level above rather than falling
// straight to the platform default. That is what makes "the organization picked a
// template, its teams did not" behave the way an operator expects.
//
// The platform default is the merged global document the gateway already keeps.
// It is not a row in the template table: a copy would give the same settings two
// homes, and whichever one the console displayed would become a lie.
//
// 参数 platform（map[string]any）：平台默认那一份设置，通常是 MergedRouter；lookup（ScopeLookup）：按范围取模板 id 并加载模板；chain（...ScopeRef）：由窄到宽的继承链。
// 返回 RouteSettings（RouteSettings）：这次请求生效的设置和来源。任何一层都没有选时返回平台默认。
// 调用：gateway 的请求路径、gateway/identity 的绑定接口。
// 测试：route_settings_test.go
func Resolve(platform map[string]any, lookup ScopeLookup, chain ...ScopeRef) RouteSettings {
	logTraceOnceRouteSettings.Do(func() { logx.Trace("enter prefs.Resolve") })

	if lookup != nil {
		for _, scope := range chain {
			if scope.ID == "" {
				continue
			}
			id := lookup.TemplateFor(scope.Kind, scope.ID)
			if id == "" {
				continue
			}
			row := lookup.Load(id)
			if row != nil {
				return RouteSettings{
					Settings:     row.Settings(),
					TemplateID:   row.ID,
					TemplateName: row.Name,
					Source:       scope.Kind,
				}
			}
			// The template was deleted between reading the selection and reading
			// the template. Treating this level as selecting nothing is the state
			// the operator left it in; continuing to a wider level would silently
			// pick a different template than the one they deleted.
			break
		}
	}
	return RouteSettings{Settings: platform, Source: PlatformSource}
}

// RequestChain is the inheritance chain for one request, narrowest first.
//
// One function for both callers of Resolve, so the request path and the console
// cannot disagree about the order. A missing id is simply not part of the chain.
// 参数 keyID（string）：密钥 id；teamID（string）：团队 id；orgID（string）：组织 id。
// 返回 []ScopeRef（[]ScopeRef）：由窄到宽的链，缺的那些不在里面。
// 调用：gateway 的请求路径、gateway/identity 的绑定接口。
// 测试：route_settings_test.go
func RequestChain(keyID, teamID, orgID string) []ScopeRef {
	chain := make([]ScopeRef, 0, 3)
	for _, scope := range []ScopeRef{{Kind: "key", ID: keyID}, {Kind: "team", ID: teamID}, {Kind: "organization", ID: orgID}} {
		if scope.ID != "" {
			chain = append(chain, scope)
		}
	}
	return chain
}

// PlatformSettings is the platform-default document as a RouteSettings value.
// It exists so a caller with no lookup at all - a path with no database, or a
// test - still expresses "the platform default applies" the same way.
// 参数 settings（map[string]any）：平台默认那一份设置。
// 返回 RouteSettings（RouteSettings）：来源标成 platform 的那一份。
// 调用：gateway 的请求路径在拿不到身份库时。
// 测试：route_settings_test.go
func PlatformSettings(settings map[string]any) RouteSettings {
	return RouteSettings{Settings: settings, Source: PlatformSource}
}

// The typed readers below are the only place a settings document is turned into
// the values the request path uses. They live here rather than in the request
// path so that "what does this document say" has one answer: the same defaults
// and the same accepted spellings whether the document came from a template or
// from the platform baseline.
//
// Each falls back to the value that was in effect before templates existed, so a
// document that does not mention a key behaves exactly as the gateway did when
// the key did not exist. That is what keeps "no template selected" identical to
// the behaviour from before this feature.

// Strategy is the routing strategy to order deployments with.
// 参数：无。
// 返回 string（string）：策略名。文档没写时是 simple-shuffle，和 Base 的默认一致。
// 调用：gateway 的请求路径。
// 测试：route_settings_test.go
func (r RouteSettings) Strategy() string {
	if v, ok := r.Settings["routing_strategy"].(string); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return "simple-shuffle"
}

// ForModel returns the routing policy for one exact requested public model. A
// matching model_routing row replaces only routing_strategy and
// routing_strategy_args; retries, timeouts, cooldown settings, and unknown
// fields continue to come from the already-resolved whole template document.
// The source document is never mutated.
// 参数 modelName（string）：请求中的公开模型名。
// 返回 RouteSettings：应用精确模型规则后的设置或原设置。
// 调用：数据面选路前读取设置。
// 测试：route_settings_test.go

// ForEndpoint 解析模型与入口策略，优先入口覆盖、模型默认、模板默认。
// 参数 modelName、endpointID：公开模型及目录 ID；返回设置或 Err，不修改源文档。
// 调用：统一、原生和预览；重试、超时和冷却保持模板公共设置。
func (r RouteSettings) ForEndpoint(modelName, endpointID string) RouteSettings {
	r.RuleSource = "template-default"
	if r.Err != nil {
		return r
	}
	rules, _, err := modelRoutingRules(r.Settings)
	if err != nil {
		r.Err = err
		return r
	}
	var selected map[string]any
	for _, rule := range rules {
		name, _ := rule["model_name"].(string)
		endpoint, _ := rule["endpoint_id"].(string)
		if strings.TrimSpace(name) != modelName {
			continue
		}
		if strings.TrimSpace(endpoint) == "" && selected == nil {
			selected = rule
		}
		if endpointID != "" && strings.TrimSpace(endpoint) == endpointID {
			selected = rule
			break
		}
	}
	if selected == nil {
		return r
	}
	settings := make(map[string]any, len(r.Settings))
	for key, value := range r.Settings {
		settings[key] = value
	}
	settings["routing_strategy"] = strings.TrimSpace(selected["routing_strategy"].(string))
	args, _ := selected["routing_strategy_args"].(map[string]any)
	if args == nil {
		args = map[string]any{}
	}
	settings["routing_strategy_args"] = args
	r.Settings = settings
	r.RuleSource = "model-default"
	if endpoint, _ := selected["endpoint_id"].(string); strings.TrimSpace(endpoint) != "" {
		r.RuleSource = "model-endpoint"
	}
	return r
}

// CursorScope 按模板内容版本、公开模型和用户入口隔离轮询状态。
// 参数为调用上下文；返回摘要，不读凭据、不推进计数。调用：共用调度。
func (r RouteSettings) CursorScope(modelName, endpointID string) string {
	raw, _ := json.Marshal([]any{r.TemplateID, r.Settings, modelName, endpointID})
	return fmt.Sprintf("%x", sha256.Sum256(raw))
}

// ValidateModelRoutingDocument validates the optional exact-name model routing
// rules. Unknown fields elsewhere in the document are deliberately ignored.
// 参数 settings（map[string]any）：完整路由文档。
// 返回 error：model_routing 结构无效时返回错误。
// 调用：模板和平台配置写入验证。
// 测试：route_settings_test.go
func ValidateModelRoutingDocument(settings map[string]any) error {
	_, _, err := modelRoutingRules(settings)
	return err
}

// modelRoutingRules parses the optional per-model policy rows.
// 参数 settings（map[string]any）：完整路由文档。
// 返回规则列表、字段是否存在及错误。
// 调用：ForModel、ValidateModelRoutingDocument。
// 测试：route_settings_test.go
func modelRoutingRules(settings map[string]any) ([]map[string]any, bool, error) {
	if settings == nil {
		return nil, false, nil
	}
	raw, present := settings["model_routing"]
	if !present || raw == nil {
		return nil, present, nil
	}
	items, ok := raw.([]any)
	if !ok {
		return nil, true, fmt.Errorf("model_routing must be an array")
	}
	rules := make([]map[string]any, 0, len(items))
	seen := map[string]bool{}
	for i, item := range items {
		rule, ok := item.(map[string]any)
		if !ok {
			return nil, true, fmt.Errorf("model_routing[%d] must be an object", i)
		}
		name, ok := rule["model_name"].(string)
		name = strings.TrimSpace(name)
		if !ok || name == "" {
			return nil, true, fmt.Errorf("model_routing[%d].model_name is required", i)
		}
		endpoint := ""
		if value, exists := rule["endpoint_id"]; exists {
			var valid bool
			endpoint, valid = value.(string)
			endpoint = strings.TrimSpace(endpoint)
			if !valid || !provider.KnownEndpoint(endpoint) {
				return nil, true, fmt.Errorf("model_routing[%d].endpoint_id is not registered", i)
			}
		}
		key := name + "\x00" + endpoint
		if seen[key] {
			return nil, true, fmt.Errorf("model_routing has duplicate model and endpoint %q", key)
		}
		seen[key] = true
		strategy, ok := rule["routing_strategy"].(string)
		strategy = strings.TrimSpace(strategy)
		if !ok || strategy == "" {
			return nil, true, fmt.Errorf("model_routing[%d].routing_strategy is required", i)
		}
		if err := router.ValidateStrategy(strategy); err != nil {
			return nil, true, fmt.Errorf("model_routing[%d].routing_strategy: %w", i, err)
		}
		if args, exists := rule["routing_strategy_args"]; exists && args != nil {
			if _, ok := args.(map[string]any); !ok {
				return nil, true, fmt.Errorf("model_routing[%d].routing_strategy_args must be an object", i)
			}
		}
		rules = append(rules, rule)
	}
	return rules, true, nil
}

// Retries is how many times one deployment is tried before the next is used.
// A value below one becomes one: a request that is never attempted cannot
// succeed, and the caller would see a 502 for what is really a settings mistake.
// 参数：无。
// 返回 int（int）：每条部署的尝试次数。文档没写或写小于 1 时是 1。
// 调用：gateway 的请求路径。
// 测试：route_settings_test.go
func (r RouteSettings) Retries() int {
	if n, ok := numberIn(r.Settings["num_retries"]); ok && n >= 1 {
		return int(n)
	}
	return 1
}

// TimeoutSeconds is how long one upstream call may take.
// 参数：无。
// 返回 float64（float64）：秒数。文档没写或不是正数时是 60，和 Base 的默认一致。
// 调用：gateway 的请求路径。
// 测试：route_settings_test.go
func (r RouteSettings) TimeoutSeconds() float64 {
	if n, ok := numberIn(r.Settings["timeout"]); ok && n > 0 {
		return n
	}
	return 60
}

// AllowedFails is how many failures a deployment may collect before it is put
// into cooldown. Zero or less disables cooldown entirely, which is the contract
// RecordFailure already reads.
// 参数：无。
// 返回 int（int）：允许的失败次数。文档没写时是 3，和 Base 的默认一致。
// 调用：gateway/noteFailure。
// 测试：route_settings_test.go
func (r RouteSettings) AllowedFails() int {
	if n, ok := numberIn(r.Settings["allowed_fails"]); ok {
		return int(n)
	}
	return 3
}

// CooldownSeconds is how long a deployment stays out after it trips the failure
// count. Zero does not mean "no cooldown": the existing contract reads a
// non-positive value as one minute, and that contract is kept here.
// 参数：无。
// 返回 float64（float64）：秒数。文档没写时是 0。
// 调用：gateway/noteFailure。
// 测试：route_settings_test.go
func (r RouteSettings) CooldownSeconds() float64 {
	n, _ := numberIn(r.Settings["cooldown_time"])
	return n
}

// WeightOverrides reads the traffic shares stored on this document.
//
// They live under routing_strategy_args.weights, which is part of the same
// router settings JSON the console edits. Two shapes are accepted, because the
// form writes a list and a hand-edited file may write a map:
//
//	{"weights": [{"deployment_id": "model_123", "weight": 70}]}
//	{"weights": {"deployment:model_123": 70}}
//
// Rows require deployment_id or pricing_id. A missing or empty list means
// "use weight 1 for every deployment". Zero disables a deployment. Negative or non-finite
// weights are ignored.
// 参数：无。
// 返回 map[string]float64（map[string]float64）：部署 id 到份额。文档没写份额时为 nil。
// 调用：dataplane/serve.go，只在策略是按权重分流时。
// 测试：route_settings_test.go
func (r RouteSettings) WeightOverrides() map[string]float64 {
	args, _ := r.Settings["routing_strategy_args"].(map[string]any)
	if args == nil {
		return nil
	}
	out := map[string]float64{}
	switch raw := args["weights"].(type) {
	case []any:
		for _, item := range raw {
			row, ok := item.(map[string]any)
			if !ok {
				continue
			}
			weight, ok := numberIn(row["weight"])
			if !ok || weight < 0 {
				continue
			}
			if id, _ := row["deployment_id"].(string); strings.TrimSpace(id) != "" {
				out["deployment:"+strings.TrimSpace(id)] = weight
			} else if id, _ := row["pricing_id"].(string); strings.TrimSpace(id) != "" {
				out["pricing:"+strings.TrimSpace(id)] = weight
			}
		}
	case map[string]any:
		for id, value := range raw {
			weight, ok := numberIn(value)
			id = strings.TrimSpace(id)
			if !ok || weight < 0 || (!strings.HasPrefix(id, "deployment:") && !strings.HasPrefix(id, "pricing:")) {
				continue
			}
			if (strings.HasPrefix(id, "deployment:") || strings.HasPrefix(id, "pricing:")) && strings.TrimSpace(strings.SplitN(id, ":", 2)[1]) == "" {
				continue
			}
			out[id] = weight
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// numberIn reads a JSON number out of a settings document. The document arrives
// from JSON, so an integer may be a float64; the YAML baseline may have either.
//
// A quoted number is not accepted. That is a mistake worth seeing as the default
// rather than as a silently different timeout, and the settings page writes
// numbers rather than strings.
// 参数 v（any）：文档里的一个值。
// 返回 float64（float64）：读到的小数；bool（bool）：这个值是一个数字时为真。
// 调用：本文件的各个类型化读取。
// 测试：route_settings_test.go
func numberIn(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, !math.IsNaN(n) && !math.IsInf(n, 0)
	case float32:
		v := float64(n)
		return v, !math.IsNaN(v) && !math.IsInf(v, 0)
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	default:
		return 0, false
	}
}
