package router

import (
	"encoding/json"
	"fmt"

	"github.com/sunqirui1987/xhub/internal/config"
)

// CleanAllocations 根据完整目录清理一份分配；参数 rows 为历史分配，names 为公开模型集合。
// 模型管理事务及启动修复调用；返回新分配。关闭的部署仍属于目录，不因健康/协议筛选提升零权重。
// 零或单部署不保留权重；多部署删除悬空引用后若全零则恢复均等分配，避免留下不可保存的配置。
func CleanAllocations(rows []Allocation, names []string, directory []config.ModelEntry) []Allocation {
	selected := map[string]bool{}
	for _, name := range names {
		selected[name] = true
	}
	ids := map[string]bool{}
	count := 0
	for _, dep := range directory {
		if selected[dep.ModelName] {
			count++
			ids[DeploymentID(dep)] = true
		}
	}
	if count <= 1 {
		return nil
	}
	var out []Allocation
	positive := false
	for _, row := range rows {
		if ids[row.DeploymentID] {
			out = append(out, row)
			positive = positive || row.Weight > 0
		}
	}
	// 未配置的现存部署默认权重为1；只有完整配置且剩余全零时才需要恢复默认。
	if len(out) == count && !positive {
		return nil
	}
	return out
}

// CleanTemplateWeights 清理模板里按模型及路由组保存的部署分配，保留策略、回退和其他字段。
// 参数 doc 为持久化正文、directory 为变更后的完整目录；返回独立正文或结构错误，无外部副作用。
// 事务调用，避免删部署后模板仍引用旧ID；单模型规则只在多个同名部署时保留分配。
func CleanTemplateWeights(doc map[string]any, directory []config.ModelEntry) (map[string]any, error) {
	raw, err := json.Marshal(doc)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err = json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	for _, field := range []string{"model_routes", "routing_groups"} {
		items, exists := out[field]
		if !exists {
			continue
		}
		rows, ok := items.([]any)
		if !ok {
			return nil, fmt.Errorf("invalid %s", field)
		}
		for _, value := range rows {
			row, ok := value.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("invalid %s entry", field)
			}
			container := row
			names := []string{}
			if field == "model_routes" {
				name, _ := row["model"].(string)
				names = append(names, name)
			} else {
				modelNames, ok := row["models"].([]any)
				if !ok {
					return nil, fmt.Errorf("invalid routing_groups models")
				}
				for _, name := range modelNames {
					s, ok := name.(string)
					if !ok || s == "" {
						return nil, fmt.Errorf("invalid routing_groups model name")
					}
					names = append(names, s)
				}
				container, _ = row["routing_strategy_args"].(map[string]any)
			}
			if container == nil {
				continue
			}
			allocations, exists := container["allocations"]
			if !exists {
				continue
			}
			data, err := json.Marshal(allocations)
			if err != nil {
				return nil, err
			}
			var parsed []Allocation
			if err = json.Unmarshal(data, &parsed); err != nil {
				return nil, err
			}
			clean := CleanAllocations(parsed, names, directory)
			if len(clean) == 0 {
				delete(container, "allocations")
			} else {
				container["allocations"] = clean
			}
		}
	}
	return out, nil
}
