package router

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/sunqirui1987/xhub/internal/config"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Group 是可直接作为 API model 使用的显式路由组；成员请求也继承组策略。
// Args 仅包含已实现的部署相对权重，不接受没有执行能力的 LiteLLM 参数。
type Group struct {
	Name     string     `json:"group_name"`
	Models   []string   `json:"models"`
	Strategy string     `json:"routing_strategy"`
	Args     *GroupArgs `json:"routing_strategy_args,omitempty"`
}

// GroupArgs 提供流量分流的表单化部署权重；缺省时所有候选权重为一。
type GroupArgs struct {
	Allocations []Allocation `json:"allocations"`
}

// ParseGroup 严格读取持久化对象，返回组或错误；用于管理接口与请求配置，无写入副作用。
func ParseGroup(raw any) (Group, error) {
	data, err := json.Marshal(raw)
	if err != nil {
		return Group{}, err
	}
	var g Group
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err = dec.Decode(&g); err != nil {
		return g, err
	}
	return g, g.Validate()
}

// Validate 校验名称、成员、策略及权重；参数为组，无外部状态，返回首个错误。
func (g Group) Validate() error {
	if g.Name == "" || g.Name != strings.TrimSpace(g.Name) || utf8.RuneCountInString(g.Name) > 64 || g.Name == "default" || (strings.Contains(g.Name, "*") || strings.IndexFunc(g.Name, unicode.IsSpace) >= 0) {
		return fmt.Errorf("group_name must be 1–64 characters, without whitespace or wildcards; default is reserved")
	}
	if len(g.Models) == 0 {
		return fmt.Errorf("select at least one model")
	}
	seen := map[string]bool{}
	for _, name := range g.Models {
		if name == "" || name != strings.TrimSpace(name) || seen[name] || strings.Contains(name, "*") {
			return fmt.Errorf("models must be nonempty, unique exact public names")
		}
		seen[name] = true
	}
	p := Policy{Strategy: g.Strategy}
	if g.Args != nil {
		if g.Strategy != "traffic-split" {
			return fmt.Errorf("routing_strategy_args require traffic-split")
		}
		p.Allocations = g.Args.Allocations
	}
	return p.Validate()
}

// ValidateGroups 对照真实目录校验名称冲突、成员占用与部署归属，供锁内保存调用；不改变目录。
func ValidateGroups(groups []Group, list []config.ModelEntry) error {
	names := map[string]bool{}
	ids := map[string]string{}
	for _, dep := range list {
		names[dep.ModelName] = true
		ids[DeploymentID(dep)] = dep.ModelName
	}
	seen := map[string]bool{}
	claimed := map[string]string{}
	for _, g := range groups {
		if err := g.Validate(); err != nil {
			return err
		}
		if names[g.Name] || seen[g.Name] {
			return fmt.Errorf("group name %q already exists or shadows a public model", g.Name)
		}
		seen[g.Name] = true
		members := map[string]bool{}
		for _, name := range g.Models {
			if !names[name] {
				return fmt.Errorf("unknown public model %q", name)
			}
			if owner := claimed[name]; owner != "" {
				return fmt.Errorf("model %q already belongs to %q", name, owner)
			}
			claimed[name] = g.Name
			members[name] = true
		}
		if g.Args != nil {
			for _, a := range g.Args.Allocations {
				if !members[ids[a.DeploymentID]] {
					return fmt.Errorf("deployment %q does not belong to group %q", a.DeploymentID, g.Name)
				}
			}
		}
	}
	return nil
}

// GroupCandidates 选择组的所有成员部署；保留真实模型名称以执行权限和计费，参数为兼容目录和组成员。
// 返回新切片，不改写上游模型、不匹配通配符；供调度及预览调用。
func GroupCandidates(list []config.ModelEntry, names []string) []config.ModelEntry {
	selected := map[string]bool{}
	for _, name := range names {
		selected[name] = true
	}
	out := []config.ModelEntry{}
	for _, dep := range list {
		if selected[dep.ModelName] && !dep.Disabled() {
			out = append(out, dep)
		}
	}
	return out
}

// candidatesForState 将显式组候选或普通模型匹配交给同一调度器；返回独立切片，无副作用。
func candidatesForState(list []config.ModelEntry, alias string, st State) []config.ModelEntry {
	if len(st.ModelNames) > 0 {
		return GroupCandidates(list, st.ModelNames)
	}
	return All(list, alias)
}
