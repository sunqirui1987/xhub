package router

import (
	"fmt"
	"math"
	"strings"
	"sync"

	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/iam"
	"github.com/sunqirui1987/xhub/internal/logx"
)

var logTraceOnceRouteSettings sync.Once

// RouteSettings 表示一次请求使用的模板、路由组和模型管理默认分配。
// 策略优先级为模板模型规则、模板路由组、历史模板默认、ModelDefaults；模板回退按主模型和类别覆盖全局默认。
type RouteSettings struct {
	Settings       map[string]any
	ModelDefaults  map[string]any
	ModelFallbacks map[string]FallbackPolicy
	RoutingGroups  []Group
	Policy         Policy
	Err            error
	TemplateID     string
	TemplateName   string
	// Source 为 key、team、organization、selected、draft 或 builtin。
	Source string
	// RuleSource 为 template-model、template-default、routing-group 或 model-default，供日志和预览解释命中来源。
	RuleSource string
}

// ScopeRef 表示模板继承链中的一个身份范围。
type ScopeRef struct{ Kind, ID string }

// BuiltinSource 表示当前身份没有绑定模板，直接使用模型管理默认分配。
const BuiltinSource = "builtin"

// ScopeLookup 提供范围绑定与模板读取，供请求和控制台共用同一解析顺序。
type ScopeLookup interface {
	TemplateFor(kind, id string) string
	Load(id string) *iam.RouteTemplate
}

// BuiltinDocument 返回新模板的公共执行参数。模型规则默认为空，不自动复制模型目录。
func BuiltinDocument() map[string]any {
	return map[string]any{
		"model_routes": []any{},
		"retry_policy": map[string]any{
			"max_attempts": 1, "timeout_seconds": 60, "failure_threshold": 3, "cooldown_seconds": 0,
		},
	}
}

// BuiltinSettings 返回未绑定模板时的设置；所有模型均走模型管理里的默认分配。
func BuiltinSettings() RouteSettings {
	return RouteSettings{Settings: BuiltinDocument(), Source: BuiltinSource, RuleSource: "model-default"}
}

// Resolve 按密钥、团队、组织的顺序选择第一份完整模板；均未绑定时使用模型管理默认分配。
func Resolve(lookup ScopeLookup, chain ...ScopeRef) RouteSettings {
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
			if row := lookup.Load(id); row != nil {
				return RouteSettings{Settings: row.Settings(), TemplateID: row.ID, TemplateName: row.Name, Source: scope.Kind}
			}
			break
		}
	}
	return BuiltinSettings()
}

// RequestChain 构造由窄到宽的模板选择链，空 ID 不进入结果。
func RequestChain(keyID, teamID, orgID string) []ScopeRef {
	chain := make([]ScopeRef, 0, 3)
	for _, scope := range []ScopeRef{{Kind: "key", ID: keyID}, {Kind: "team", ID: teamID}, {Kind: "organization", ID: orgID}} {
		if scope.ID != "" {
			chain = append(chain, scope)
		}
	}
	return chain
}

// Strategy 返回已解析策略。调用 ForModel 前使用模型管理默认的按权重随机。
func (r RouteSettings) Strategy() string {
	if r.Policy.Strategy != "" {
		return r.Policy.Strategy
	}
	return "traffic-split"
}

// ForModel 解析公开模型或组名的策略；参数为请求 model，返回独立设置或包含解析错误的设置。
// 请求及预览共用：模板模型规则优先于路由组和模型权重；旧文档的模板默认策略仅兼容读取。组内 traffic-split
// 使用组权重，未设置部署默认为 1；模板显式权重优先于实时模型默认，不修改原始文档。
func (r RouteSettings) ForModel(modelName string) RouteSettings {
	if r.Err != nil {
		return r
	}
	rules, err := modelRouteRules(r.Settings)
	if err != nil {
		r.Err = err
		return r
	}
	strategy := "traffic-split"
	r.RuleSource = "model-default"
	if raw, exists := r.Settings["routing_strategy"]; exists {
		value, ok := raw.(string)
		if !ok || ValidateStrategy(value) != nil {
			r.Err = fmt.Errorf("invalid routing_strategy")
			return r
		}
		strategy = value
		r.RuleSource = "template-default"
	}
	for _, group := range r.RoutingGroups {
		matched := group.Name == modelName
		for _, member := range group.Models {
			matched = matched || member == modelName
		}
		if matched {
			strategy = group.Strategy
			r.RuleSource = "routing-group"
			break
		}
	}
	var custom *Policy
	for _, rule := range rules {
		if strings.TrimSpace(rule["model"].(string)) == modelName {
			strategy = strings.TrimSpace(rule["strategy"].(string))
			r.RuleSource = "template-model"
			if _, exists := rule["allocations"]; exists {
				// modelRouteRules 已严格校验相同字段；这里仅构造请求的独立策略，不读写默认权重。
				p, _ := ParsePolicy(map[string]any{"strategy": strategy, "allocations": rule["allocations"]})
				custom = &p
			}
			break
		}
	}
	r.Policy = Policy{Strategy: strategy}
	if custom != nil {
		r.Policy = *custom
	} else if strategy == "traffic-split" && r.RuleSource != "routing-group" {
		if raw, ok := r.ModelDefaults[modelName]; ok {
			r.Policy, r.Err = ParseDefaultWeights(raw)
		}
	}
	if r.RuleSource == "routing-group" {
		for _, group := range r.RoutingGroups {
			matched := group.Name == modelName
			for _, member := range group.Models {
				matched = matched || member == modelName
			}
			if matched && group.Args != nil {
				r.Policy.Allocations = group.Args.Allocations
				break
			}
		}
	}
	return r
}

// Candidates 返回请求模型或显式组的真实部署，供预览使用；不排序、不改变模型身份。
func (r RouteSettings) Candidates(list []config.ModelEntry, name string) []config.ModelEntry {
	for _, group := range r.RoutingGroups {
		if group.Name == name {
			return GroupCandidates(list, group.Models)
		}
	}
	return All(list, name)
}

// Schedule 为数据面统一展开显式组并使用解析后的策略与权重；成员名请求只匹配该成员。
// 参数为兼容候选、请求名、运行状态及会话部署；返回尝试顺序，保留权限和计费身份。
func (r RouteSettings) Schedule(list []config.ModelEntry, name string, state State, pinned string) []config.ModelEntry {
	state.Allocations = r.Policy.Shares()
	for _, group := range r.RoutingGroups {
		if group.Name == name {
			state.ModelNames = group.Models
			break
		}
	}
	return Schedule(list, name, r.Strategy(), state, pinned)
}

// ValidateRouteTemplateDocument 校验保存或预览的模板文档，返回首个字段错误，无写入副作用。
// routing_strategy 为可选模板默认；未提供时保留旧模板的模型默认继承语义。
// 模型规则可保存独立部署权重；不支持执行的字段必须拒绝，防止保存成功却无效。
func ValidateRouteTemplateDocument(settings map[string]any) error {
	allowed := map[string]bool{"model_routes": true, "retry_policy": true, "routing_strategy": true, "routing_groups": true, "fallbacks": true, "context_window_fallbacks": true, "content_policy_fallbacks": true}
	for key := range settings {
		if !allowed[key] {
			return fmt.Errorf("unsupported route template field %q", key)
		}
	}
	if raw, exists := settings["routing_strategy"]; exists {
		strategy, ok := raw.(string)
		if !ok || ValidateStrategy(strategy) != nil {
			return fmt.Errorf("invalid routing_strategy")
		}
	}
	if _, err := modelRouteRules(settings); err != nil {
		return err
	}
	policy, err := retryPolicy(settings)
	if err != nil {
		return err
	}
	for _, field := range []struct {
		name              string
		integer, positive bool
	}{{"max_attempts", true, true}, {"timeout_seconds", false, true}, {"failure_threshold", true, false}, {"cooldown_seconds", false, false}} {
		number, valid := numberIn(policy[field.name])
		if !valid || number < 0 || (field.positive && number == 0) || (field.integer && math.Trunc(number) != number) {
			return fmt.Errorf("invalid retry_policy.%s", field.name)
		}
	}
	_, _, err = TemplateRouting(settings)
	return err
}

// modelRouteRules 严格解析公开模型覆盖表；模型名必须唯一，规则包含 model、strategy 和可选 allocations；省略继承默认，空数组为均等权重。
func modelRouteRules(settings map[string]any) ([]map[string]any, error) {
	raw, exists := settings["model_routes"]
	if !exists {
		return nil, fmt.Errorf("model_routes is required")
	}
	items, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("model_routes must be an array")
	}
	rows := make([]map[string]any, 0, len(items))
	seen := map[string]bool{}
	for i, item := range items {
		rule, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("model_routes[%d] must be an object", i)
		}
		for key := range rule {
			if key != "model" && key != "strategy" && key != "allocations" {
				return nil, fmt.Errorf("model_routes[%d] contains unsupported field %q", i, key)
			}
		}
		name, nameOK := rule["model"].(string)
		name = strings.TrimSpace(name)
		if !nameOK || name == "" {
			return nil, fmt.Errorf("model_routes[%d].model is required", i)
		}
		if seen[name] {
			return nil, fmt.Errorf("model_routes has duplicate model %q", name)
		}
		seen[name] = true
		strategy, strategyOK := rule["strategy"].(string)
		strategy = strings.TrimSpace(strategy)
		if !strategyOK || strategy == "" {
			return nil, fmt.Errorf("model_routes[%d].strategy is required", i)
		}
		if err := ValidateStrategy(strategy); err != nil {
			return nil, fmt.Errorf("model_routes[%d].strategy: %w", i, err)
		}
		normalized := map[string]any{"model": name, "strategy": strategy}
		if raw, exists := rule["allocations"]; exists {
			if _, ok := raw.([]any); !ok || strategy != "traffic-split" {
				return nil, fmt.Errorf("model_routes[%d].allocations requires traffic-split and an array", i)
			}
			for _, entry := range raw.([]any) {
				allocation, ok := entry.(map[string]any)
				if !ok {
					return nil, fmt.Errorf("model_routes[%d].allocations must contain objects", i)
				}
				if _, valid := numberIn(allocation["weight"]); !valid {
					return nil, fmt.Errorf("model_routes[%d].allocations.weight must be a number", i)
				}
			}
			if _, err := ParsePolicy(map[string]any{"strategy": strategy, "allocations": raw}); err != nil {
				return nil, fmt.Errorf("model_routes[%d]: %w", i, err)
			}
			normalized["allocations"] = raw
		}
		rows = append(rows, normalized)
	}
	return rows, nil
}

// retryPolicy 返回严格的失败重试对象；四个字段必须完整存在，避免模板语义依赖隐式合并。
func retryPolicy(settings map[string]any) (map[string]any, error) {
	raw, exists := settings["retry_policy"]
	if !exists {
		return nil, fmt.Errorf("retry_policy is required")
	}
	policy, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("retry_policy must be an object")
	}
	allowed := map[string]bool{"max_attempts": true, "timeout_seconds": true, "failure_threshold": true, "cooldown_seconds": true}
	for key := range policy {
		if !allowed[key] {
			return nil, fmt.Errorf("retry_policy contains unsupported field %q", key)
		}
	}
	for key := range allowed {
		if _, exists := policy[key]; !exists {
			return nil, fmt.Errorf("retry_policy.%s is required", key)
		}
	}
	return policy, nil
}

// Retries 返回每条部署的总尝试次数；缺失或小于一时使用 1。
func (r RouteSettings) Retries() int {
	policy, _ := r.Settings["retry_policy"].(map[string]any)
	if n, ok := numberIn(policy["max_attempts"]); ok && n >= 1 {
		return int(n)
	}
	return 1
}

// TimeoutSeconds 返回单次上游调用超时秒数；无效值使用 60。
func (r RouteSettings) TimeoutSeconds() float64 {
	policy, _ := r.Settings["retry_policy"].(map[string]any)
	if n, ok := numberIn(policy["timeout_seconds"]); ok && n > 0 {
		return n
	}
	return 60
}

// AllowedFails 返回部署进入冷静期前允许的失败次数；缺失时使用 3。
func (r RouteSettings) AllowedFails() int {
	policy, _ := r.Settings["retry_policy"].(map[string]any)
	if n, ok := numberIn(policy["failure_threshold"]); ok {
		return int(n)
	}
	return 3
}

// CooldownSeconds 返回部署冷静期秒数；缺失时使用 0。
func (r RouteSettings) CooldownSeconds() float64 {
	policy, _ := r.Settings["retry_policy"].(map[string]any)
	n, _ := numberIn(policy["cooldown_seconds"])
	return n
}

// numberIn 读取 JSON 或 Go 数字，拒绝字符串及非有限浮点数。
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
