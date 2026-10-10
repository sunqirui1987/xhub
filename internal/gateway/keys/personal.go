package keys

import (
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/sunqirui1987/xhub/internal/auth"
	"github.com/sunqirui1987/xhub/internal/authz"
	"github.com/sunqirui1987/xhub/internal/iam"
)

// personalFilter 将现有权限与本人归属取交集，供个人列表使用。
// 参数 base 为已授权过滤器，p 为认证身份；返回仅含本人个人密钥的 SQL 过滤器。
// 无身份、非会话身份均匹配空集；不读客户端 user_id，不扩大原有密钥 ID 限制。
func personalFilter(base iam.KeyFilter, p *auth.Principal) iam.KeyFilter {
	if p == nil || p.Kind != authz.KindSession || p.UserID == "" {
		return iam.KeyFilter{KeyIDs: []string{}}
	}
	if base.OwnOrService {
		// 原团队范围是“本人或服务密钥”，团队限制仅针对服务分支，不能套到本人分支。
		base.TeamIDs = nil
	}
	base.OwnOrService = false
	base.UserID = p.UserID
	base.OwnerType = iam.OwnerPersonal
	return base
}

// personalKeyVisible 判断个人详情是否属于当前会话用户。
// 参数 p 为认证身份、k 为数据库密钥；返回可见性，空身份、空记录及服务密钥均拒绝。
// 调用场景：个人列表深链接的详情读取；无副作用。
func personalKeyVisible(p *auth.Principal, k *iam.Key) bool {
	return p != nil && p.Kind == authz.KindSession && p.UserID != "" && k.OwnedBy(p.UserID)
}

// personalPage 在已通过 SQL 隔离的个人记录内筛选、排序及分页。
// 参数 rows 为授权后的数据库记录，q 为 URL 条件；返回当前页、总数、页码、页大小、总页数及错误。
// 调用场景：List 的个人查询；非法分页或排序返回错误，越界页为空；不修改调用方切片。
func personalPage(rows []iam.Key, q url.Values) ([]iam.Key, int, int, int, int, error) {
	page, size := 1, 50
	for name, target := range map[string]*int{"page": &page, "size": &size} {
		if raw := q.Get(name); raw != "" {
			n, err := strconv.Atoi(raw)
			if err != nil || n < 1 || (name == "size" && n > 100) || (name == "page" && n > 100000) {
				return nil, 0, 0, 0, 0, fmt.Errorf("invalid %s", name)
			}
			*target = n
		}
	}
	field := q.Get("sort_by")
	if field == "" {
		field = "created_at"
	}
	switch field {
	case "created_at", "updated_at", "key_alias", "token", "spend", "max_budget", "budget_remaining", "budget_utilization":
	default:
		return nil, 0, 0, 0, 0, fmt.Errorf("invalid sort_by")
	}
	order := q.Get("sort_order")
	if order == "" {
		order = "desc"
	}
	if order != "asc" && order != "desc" {
		return nil, 0, 0, 0, 0, fmt.Errorf("invalid sort_order")
	}
	filtered := make([]iam.Key, 0, len(rows))
	for _, k := range rows {
		if (q.Get("team_id") != "" && q.Get("team_id") != k.TeamID) ||
			(q.Get("user_id") != "" && (k.UserID == nil || q.Get("user_id") != *k.UserID)) ||
			(q.Get("key_hash") != "" && q.Get("key_hash") != k.ID) {
			continue
		}
		search := strings.ToLower(strings.TrimSpace(q.Get("search")))
		if search != "" && !strings.Contains(strings.ToLower(k.Name), search) && !strings.Contains(strings.ToLower(k.ID), search) {
			continue
		}
		filtered = append(filtered, k)
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		a, b := filtered[i], filtered[j]
		cmp := strings.Compare(a.ID, b.ID)
		switch field {
		case "key_alias":
			cmp = strings.Compare(a.Name, b.Name)
		case "created_at":
			cmp = a.CreatedAt.Compare(b.CreatedAt)
		case "updated_at":
			cmp = a.UpdatedAt.Compare(b.UpdatedAt)
		case "spend", "max_budget", "budget_remaining", "budget_utilization":
			x, y := personalSortValue(a, field), personalSortValue(b, field)
			if x < y {
				cmp = -1
			} else if x > y {
				cmp = 1
			} else {
				cmp = 0
			}
		}
		if cmp == 0 {
			cmp = strings.Compare(a.ID, b.ID)
		}
		if order == "asc" {
			return cmp < 0
		}
		return cmp > 0
	})
	total := len(filtered)
	pages := (total + size - 1) / size
	if pages == 0 {
		pages = 1
	}
	start := (page - 1) * size
	if start >= total {
		return []iam.Key{}, total, page, size, pages, nil
	}
	end := start + size
	if end > total {
		end = total
	}
	return filtered[start:end], total, page, size, pages, nil
}

// personalSortValue 提供个人列表预算列排序值，参数为记录和字段名，返回数值。
// 供 personalPage 使用；无限预算按零处理，零预算不除零；无副作用。
func personalSortValue(k iam.Key, field string) float64 {
	budget := 0.0
	if k.MaxBudget != nil {
		budget = *k.MaxBudget
	}
	switch field {
	case "spend":
		return k.Spend
	case "budget_remaining":
		return budget - k.Spend
	case "budget_utilization":
		if budget > 0 {
			return k.Spend / budget
		}
		return 0
	default:
		return budget
	}
}
