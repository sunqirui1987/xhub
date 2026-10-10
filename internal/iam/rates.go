package iam

import (
	"context"
	"errors"
	"github.com/sunqirui1987/xhub/internal/live"
	"sort"
)

// ErrRateAllocation 表示下级 RPM/TPM 分配总和超出父级，管理接口回滚并返回 400。
var ErrRateAllocation = errors.New("rate allocation exceeds parent limit")

// RateLimits 定义独立分钟上限；nil 不设置本层上限，零禁止调用；创建接口使用，固定分配保留父级容量。
type RateLimits struct {
	RPMLimit *int
	TPMLimit *int
}

// RatePatch 区分未提交与显式清空；供团队和成员事务更新使用。
type RatePatch struct {
	RPMLimit **int
	TPMLimit **int
}

// ValidateRates 检查数据库 INT 可表示的非负整数；参数为限额，返回业务错误或 nil。
// 调用：IAM 创建与更新；无副作用，数据库约束作为最后保护。
func ValidateRates(values ...*int) error {
	for _, v := range values {
		if v != nil && (*v < 0 || int64(*v) > 2147483647) {
			return ErrInvalid
		}
	}
	return nil
}

// RatePlan 读取调用者的唯一归属树；ctx 用于数据库查询，kind/id 标识 Key 或个人。
// 返回同一根节点下的配置和从窄到宽的调用路径；缺失归属、多团队或循环返回错误。
// 网关准入调用；只读取配置，不写分钟计数。保留兄弟及其后代以保护固定容量，排除无关组织。
func (db *DB) RatePlan(ctx context.Context, kind, id string) (live.RatePlan, error) {
	s := db.session(ctx)
	defer s.Close()
	nodes, err := quotaTree(s)
	if err != nil {
		return live.RatePlan{}, err
	}
	return buildRatePlan(nodes, kind+":"+id)
}

// buildRatePlan 将额度树转换为调用者所在分量的分钟计划；nodes 为已链接的树，target 为带层级的 ID。
// 返回含全部兄弟保留配置的计划或归属错误；RatePlan 和纯单元测试调用，无数据库/计数副作用。
// 真正没有团队的个人是合法根；声明了上级却找不到记录时必须失败，不能误判为不限额根。
func buildRatePlan(nodes map[string]*quotaNode, target string) (live.RatePlan, error) {
	if nodes[target] == nil {
		return live.RatePlan{}, ErrNotFound
	}
	path := []string{}
	seen := map[string]bool{}
	root := ""
	for key := target; key != ""; {
		if seen[key] {
			return live.RatePlan{}, ErrInvalid
		}
		seen[key] = true
		n := nodes[key]
		if n == nil || key != n.Kind+":"+n.ID {
			return live.RatePlan{}, ErrInvalid
		}
		if n.Parent == "ambiguous" {
			return live.RatePlan{}, ErrMultipleTeams
		}
		path = append(path, key)
		root = key
		key = n.Parent
	}
	// 固定兄弟的未用容量参与祖先准入，因此不能只保留调用路径；其他根不参与本次决策。
	children := map[string][]string{}
	for key, n := range nodes {
		children[n.Parent] = append(children[n.Parent], key)
	}
	ids := []string{root}
	for i := 0; i < len(ids); i++ {
		ids = append(ids, children[ids[i]]...)
	}
	sort.Strings(ids)
	indices := map[string]int{}
	for i, key := range ids {
		indices[key] = i
	}
	plan := live.RatePlan{}
	for _, key := range ids {
		n := nodes[key]
		parent := -1
		if n.Parent != "" {
			parent = indices[n.Parent]
		}
		plan.Nodes = append(plan.Nodes, live.RateNode{RateScope: live.RateScope{ID: key, Kind: n.Kind, RPM: n.RPM, TPM: n.TPM}, Parent: parent})
	}
	for _, key := range path {
		plan.Path = append(plan.Path, indices[key])
	}
	return plan, nil
}
