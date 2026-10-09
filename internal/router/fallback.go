package router

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/sunqirui1987/xhub/internal/config"
)

// FallbackPolicy 保存公开模型的三类有序回退目标，空列表表示不跨模型回退。
type FallbackPolicy struct {
	Fallbacks     []string `json:"fallbacks"`
	ContextWindow []string `json:"context_window_fallbacks"`
	ContentPolicy []string `json:"content_policy_fallbacks"`
}

// ParseFallbackPolicy 严格解析管理接口或持久化对象，返回策略或格式错误。
// 参数 raw 为 JSON 对象；调用：管理和数据面；拒绝未知字段、空名称与重复目标，无副作用。
func ParseFallbackPolicy(raw any) (FallbackPolicy, error) {
	data, err := json.Marshal(raw)
	if err != nil {
		return FallbackPolicy{}, err
	}
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return FallbackPolicy{}, fmt.Errorf("fallback policy must be an object")
	}
	var p FallbackPolicy
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		return p, err
	}
	for _, targets := range [][]string{p.Fallbacks, p.ContextWindow, p.ContentPolicy} {
		if len(targets) > 32 {
			return p, fmt.Errorf("at most 32 fallback targets per error type")
		}
		seen := map[string]bool{}
		for _, target := range targets {
			if strings.TrimSpace(target) == "" || strings.TrimSpace(target) != target || seen[target] {
				return p, fmt.Errorf("fallback targets must be nonempty, trimmed and unique")
			}
			seen[target] = true
		}
	}
	return p, nil
}

// Targets 按 general、context 或 content 返回只读有序目标，未知类型返回空。
// 调用：数据面队列；不改变配置，也不隐式使用另一错误类型的配置。
func (p FallbackPolicy) Targets(kind string) []string {
	switch kind {
	case "general":
		return p.Fallbacks
	case "context":
		return p.ContextWindow
	case "content":
		return p.ContentPolicy
	}
	return nil
}

// ValidateFallbackGraph 校验完整回退图，参数为策略和部署目录，返回悬空或循环错误。
// 调用：管理员保存；跨类型统一判环以避免无限回退，不修改输入或存储。
func ValidateFallbackGraph(policies map[string]FallbackPolicy, models []config.ModelEntry) error {
	names := map[string]bool{}
	for _, dep := range models {
		names[dep.ModelName] = true
	}
	states := map[string]int{}
	var visit func(string) error
	visit = func(name string) error {
		if states[name] == 1 {
			return fmt.Errorf("fallback cycle involving %q", name)
		}
		if states[name] == 2 {
			return nil
		}
		states[name] = 1
		p := policies[name]
		for _, targets := range [][]string{p.Fallbacks, p.ContextWindow, p.ContentPolicy} {
			for _, target := range targets {
				if !names[target] {
					return fmt.Errorf("fallback public model %q not found", target)
				}
				if err := visit(target); err != nil {
					return err
				}
			}
		}
		states[name] = 2
		return nil
	}
	for name := range policies {
		if names[name] {
			if err := visit(name); err != nil {
				return err
			}
		}
	}
	return nil
}
