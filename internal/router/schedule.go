package router

import "github.com/sunqirui1987/xhub/internal/config"

// Schedule 为适配与官方数据面生成本次请求的部署尝试顺序。
// 参数 list 是完整部署目录，alias 是公开模型或路由组名，strategy 是已校验策略，
// st 是运行状态，pinned 是会话固定的运行身份；返回已过滤、排序的新切片。
// 粘性只可置顶兼容、未冷却且正权重的候选；本函数不修改部署目录或共享配置。
func Schedule(list []config.ModelEntry, alias, strategy string, st State, pinned string) []config.ModelEntry {
	pool := candidatesForState(list, alias, st)
	pool = Available(pool, strategy, st)
	if pinned != "" && !st.Cooldown[pinned] {
		for i, e := range pool {
			if CooldownID(e) == pinned {
				out := make([]config.ModelEntry, 0, len(pool))
				out = append(out, e)
				out = append(out, pool[:i]...)
				out = append(out, pool[i+1:]...)
				return out
			}
		}
	}
	return Order(pool, alias, strategy, st)
}

// Available 按策略过滤当前可选部署，不排序也不抽样。
// 参数 pool 是已匹配候选，strategy 是已校验策略，st 是运行状态；返回新切片。
// traffic-split 还排除非正或非法权重，其他策略只排除冷却部署；没有健康候选时返回空。
func Available(pool []config.ModelEntry, strategy string, st State) []config.ModelEntry {
	if IsSplitStrategy(strategy) {
		return splitCandidates(pool, st)
	}
	return openCandidates(pool, st)
}
