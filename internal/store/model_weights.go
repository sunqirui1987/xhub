package store

import (
	"encoding/json"
	"time"

	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/iam"
	"github.com/sunqirui1987/xhub/internal/router"
)

// SaveModelDirectory 原子保存部署变更并清理模型默认、历史组及客户模板里的失效分配。
// 参数 entry 非空表示创建/更新，deletedID 非空表示删除，directory 是变更后的完整目录（含配置文件部署）。
// 管理接口和启动迁移调用；两变更参数均空仅修复历史数据。失败整笔回滚，成功后清缓存，不修改内存目录。
func (s *Store) SaveModelDirectory(entry *ProxyModel, deletedID string, directory []config.ModelEntry) error {
	if s == nil {
		return nil
	}
	tx := s.Engine.NewSession()
	defer tx.Close()
	if err := tx.Begin(); err != nil {
		return err
	}
	defer tx.Rollback()
	if entry != nil {
		params, err := json.Marshal(entry.Params)
		if err != nil {
			return err
		}
		info, err := json.Marshal(entry.Info)
		if err != nil {
			return err
		}
		row := proxyModelRow{ID: entry.ID, ModelName: entry.ModelName, Params: string(params), Info: string(info), UpdatedAt: time.Now().UTC().Format(time.RFC3339)}
		n, err := tx.ID(row.ID).AllCols().Update(&row)
		if err != nil {
			return err
		}
		if n == 0 {
			if _, err = tx.Insert(&row); err != nil {
				return err
			}
		}
	}
	if deletedID != "" {
		if _, err := tx.ID(deletedID).Delete(&proxyModelRow{}); err != nil {
			return err
		}
	}
	var defaults []configRow
	if err := tx.Where("namespace = ?", "model_defaults").Find(&defaults); err != nil {
		return err
	}
	for _, row := range defaults {
		// 单部署无需解析遗留权重，连同损坏的历史分配一起删除。
		count := 0
		for _, dep := range directory {
			if dep.ModelName == row.Key {
				count++
			}
		}
		var allocations []router.Allocation
		if count > 1 {
			var weights router.DefaultWeights
			if err := json.Unmarshal([]byte(row.ValueJSON), &weights); err != nil {
				return err
			}
			allocations = router.CleanAllocations(weights.Allocations, []string{row.Key}, directory)
		}
		if len(allocations) == 0 {
			if _, err := tx.ID(coreIDs(row.Namespace, row.Key)).Delete(&configRow{}); err != nil {
				return err
			}
		} else {
			raw, err := json.Marshal(router.DefaultWeights{Allocations: allocations})
			if err != nil {
				return err
			}
			if string(raw) != row.ValueJSON {
				row.ValueJSON = string(raw)
				if _, err = tx.ID(coreIDs(row.Namespace, row.Key)).Cols("value_json").Update(&row); err != nil {
					return err
				}
			}
		}
	}
	var groups []configRow
	if err := tx.Where("namespace = ?", "routing_groups").Find(&groups); err != nil {
		return err
	}
	for _, row := range groups {
		var group map[string]any
		if err := json.Unmarshal([]byte(row.ValueJSON), &group); err != nil {
			return err
		}
		doc, err := router.CleanTemplateWeights(map[string]any{"routing_groups": []any{group}}, directory)
		if err != nil {
			return err
		}
		raw, err := json.Marshal(doc["routing_groups"].([]any)[0])
		if err != nil {
			return err
		}
		if string(raw) != row.ValueJSON {
			row.ValueJSON = string(raw)
			if _, err = tx.ID(coreIDs(row.Namespace, row.Key)).Cols("value_json").Update(&row); err != nil {
				return err
			}
		}
	}
	// 框架存储也可独立使用；身份表存在时才同步清理客户模板，仍使用同一数据库事务。
	hasTemplates, err := s.Engine.IsTableExist(new(iam.RouteTemplate))
	if err != nil {
		return err
	}
	if hasTemplates {
		var templates []iam.RouteTemplate
		// xorm 的 ForUpdate 仅支持 MySQL；项目使用 PostgreSQL，显式 SQL 在同一事务内锁行。
		if err = tx.SQL("SELECT * FROM route_templates FOR UPDATE").Find(&templates); err != nil {
			return err
		}
		for _, row := range templates {
			var doc map[string]any
			if err = json.Unmarshal([]byte(row.Body), &doc); err != nil {
				return err
			}
			clean, err := router.CleanTemplateWeights(doc, directory)
			if err != nil {
				return err
			}
			raw, err := json.Marshal(clean)
			if err != nil {
				return err
			}
			if string(raw) != row.Body {
				row.Body = string(raw)
				if _, err = tx.ID(row.ID).Cols("body").Update(&row); err != nil {
					return err
				}
			}
		}
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	s.bust(new(proxyModelRow), new(configRow))
	return nil
}
