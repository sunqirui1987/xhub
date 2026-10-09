package router

import (
	"fmt"

	"github.com/sunqirui1987/xhub/internal/config"
)

// ConfigReader 是路由配置的只读存储边界，网关传入持久化仓库；解析器不负责HTTP和身份鉴权。
type ConfigReader interface {
	ListConfig(string) (map[string]any, error)
}

// Compile 将已选身份模板、全局模型权重和回退编译为一份请求快照。
// 参数 selected 为密钥/团队/组织选出的完整模板，store 为配置库，models 为锁内复制目录。
// 返回独立策略或带Err结果；请求与预览共用，不写配置、不缓存，存储或引用失败立即拒绝。
// 未覆盖的模型回退继承全局；显式 {"模型":[]} 仅禁用对应模型与错误类型的回退。
func Compile(selected RouteSettings, store ConfigReader, models []config.ModelEntry) RouteSettings {
	if selected.Err != nil {
		return selected
	}
	// fail 将解析或存储错误附加到本次快照，调用方据此拒绝请求，不修改共享配置。
	fail := func(err error) RouteSettings { selected.Err = err; return selected }
	defaults, err := store.ListConfig("model_defaults")
	if err != nil {
		return fail(err)
	}
	selected.ModelDefaults = defaults
	groups, policies, err := TemplateRouting(selected.Settings)
	if err != nil {
		return fail(err)
	}
	selected.RoutingGroups = groups
	// 旧模板没有组字段才读取历史全局组，新模板空数组表示没有模板组。
	if _, local := selected.Settings["routing_groups"]; !local {
		legacy, err := store.ListConfig("routing_groups")
		if err != nil {
			return fail(err)
		}
		for name, raw := range legacy {
			group, err := ParseGroup(raw)
			if err != nil {
				return fail(err)
			}
			if group.Name != name {
				return fail(fmt.Errorf("invalid routing group identity"))
			}
			selected.RoutingGroups = append(selected.RoutingGroups, group)
		}
	}
	raw, err := store.ListConfig("model_fallbacks")
	if err != nil {
		return fail(err)
	}
	selected.ModelFallbacks = map[string]FallbackPolicy{}
	for name, value := range raw {
		p, err := ParseFallbackPolicy(value)
		if err != nil {
			return fail(err)
		}
		selected.ModelFallbacks[name] = p
	}
	// 只覆盖模板明确声明的主模型/类别，其他默认链仍然来自模型管理。
	for _, field := range []string{"fallbacks", "context_window_fallbacks", "content_policy_fallbacks"} {
		rows, _ := selected.Settings[field].([]any)
		for _, row := range rows {
			for name := range row.(map[string]any) {
				p := selected.ModelFallbacks[name]
				local := policies[name]
				switch field {
				case "fallbacks":
					p.Fallbacks = local.Fallbacks
				case "context_window_fallbacks":
					p.ContextWindow = local.ContextWindow
				case "content_policy_fallbacks":
					p.ContentPolicy = local.ContentPolicy
				}
				selected.ModelFallbacks[name] = p
			}
		}
	}
	if err := ValidateTemplateCatalog(selected.Settings, models); err != nil {
		return fail(err)
	}
	// 合并后的默认链与模板链也可能构成环，因此必须在同一个图上校验。
	entries := append([]config.ModelEntry(nil), models...)
	for _, g := range selected.RoutingGroups {
		entries = append(entries, config.ModelEntry{ModelName: g.Name})
	}
	if err := ValidateFallbackGraph(selected.ModelFallbacks, entries); err != nil {
		return fail(err)
	}
	return selected
}
