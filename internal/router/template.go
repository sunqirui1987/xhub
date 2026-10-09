package router

import (
	"fmt"
	"github.com/sunqirui1987/xhub/internal/config"
	"strings"
)

// TemplateRouting 解析模板内的组和三类 LiteLLM 形式回退表，返回独立对象或字段错误。
// 保存和请求解析共用；成员占用、重复主模型、空目标和跨类别循环均拒绝，无持久化副作用。
func TemplateRouting(doc map[string]any) ([]Group, map[string]FallbackPolicy, error) {
	groups := []Group{}
	policies := map[string]FallbackPolicy{}
	names, claimed := map[string]bool{}, map[string]bool{}
	if raw, exists := doc["routing_groups"]; exists {
		rows, ok := raw.([]any)
		if !ok {
			return nil, nil, fmt.Errorf("routing_groups must be an array")
		}
		for _, row := range rows {
			g, err := ParseGroup(row)
			if err != nil {
				return nil, nil, err
			}
			if names[g.Name] {
				return nil, nil, fmt.Errorf("duplicate routing group %q", g.Name)
			}
			names[g.Name] = true
			for _, member := range g.Models {
				if claimed[member] {
					return nil, nil, fmt.Errorf("model %q belongs to multiple groups", member)
				}
				claimed[member] = true
			}
			groups = append(groups, g)
		}
	}
	for _, field := range []string{"fallbacks", "context_window_fallbacks", "content_policy_fallbacks"} {
		raw, exists := doc[field]
		if !exists {
			continue
		}
		rows, ok := raw.([]any)
		if !ok {
			return nil, nil, fmt.Errorf("%s must be an array", field)
		}
		seen := map[string]bool{}
		for _, row := range rows {
			mapping, ok := row.(map[string]any)
			if !ok || len(mapping) != 1 {
				return nil, nil, fmt.Errorf("%s entries must map one primary model to targets", field)
			}
			for name, targets := range mapping {
				if name == "" || name != strings.TrimSpace(name) || seen[name] {
					return nil, nil, fmt.Errorf("invalid or duplicate fallback primary %q", name)
				}
				seen[name] = true
				if _, ok := targets.([]any); !ok {
					return nil, nil, fmt.Errorf("%s targets must be an array", field)
				}
				parsed, err := ParseFallbackPolicy(map[string]any{field: targets})
				if err != nil {
					return nil, nil, err
				}
				p := policies[name]
				switch field {
				case "fallbacks":
					p.Fallbacks = parsed.Fallbacks
				case "context_window_fallbacks":
					p.ContextWindow = parsed.ContextWindow
				case "content_policy_fallbacks":
					p.ContentPolicy = parsed.ContentPolicy
				}
				policies[name] = p
			}
		}
	}
	// 结构校验使用正文出现的节点判环；真实目录存在性在保存边界和数据面再次检查。
	graphNames := map[string]bool{}
	for name, p := range policies {
		graphNames[name] = true
		for _, targets := range [][]string{p.Fallbacks, p.ContextWindow, p.ContentPolicy} {
			for _, target := range targets {
				graphNames[target] = true
			}
		}
	}
	entries := []config.ModelEntry{}
	for name := range graphNames {
		entries = append(entries, config.ModelEntry{ModelName: name})
	}
	if err := ValidateFallbackGraph(policies, entries); err != nil {
		return nil, nil, err
	}
	return groups, policies, nil
}

// ValidateTemplateCatalog 对照实时模型目录检查模板组、权重和回退引用；保存及请求调用，无写入。
// 组名可作为回退主模型及目标；组成员必须是确切公开模型，禁止组内嵌组。
func ValidateTemplateCatalog(doc map[string]any, models []config.ModelEntry) error {
	// 显式权重必须属于该公开模型，防止跨模型部署 ID 被保存后静默失效。
	if _, exists := doc["model_routes"]; exists {
		rules, err := modelRouteRules(doc)
		if err != nil {
			return err
		}
		for _, rule := range rules {
			if raw, custom := rule["allocations"]; custom {
				p, err := ParsePolicy(map[string]any{"strategy": rule["strategy"], "allocations": raw})
				if err != nil {
					return err
				}
				if err := p.ValidateDeployments(models, rule["model"].(string)); err != nil {
					return err
				}
			}
		}
	}
	groups, policies, err := TemplateRouting(doc)
	if err != nil {
		return err
	}
	if err = ValidateGroups(groups, models); err != nil {
		return err
	}
	entries := append([]config.ModelEntry(nil), models...)
	names := map[string]bool{}
	for _, dep := range entries {
		names[dep.ModelName] = true
	}
	for _, g := range groups {
		entries = append(entries, config.ModelEntry{ModelName: g.Name})
		names[g.Name] = true
	}
	for name := range policies {
		if !names[name] {
			return fmt.Errorf("unknown fallback primary %q", name)
		}
	}
	return ValidateFallbackGraph(policies, entries)
}
