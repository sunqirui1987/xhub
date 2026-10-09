package router

import "github.com/sunqirui1987/xhub/internal/config"

// Schedule 共用实际请求调度：先匹配、健康/权重过滤，再处理粘性，最后推进策略。
// 参数为已兼容筛选的部署、公开型号、策略、隔离状态及粘性 ID；返回尝试列表。
// 调用：统一与原生入口；有效粘性命中不推进分流计数，冷却或零份额部署不参与粘性。
func Schedule(list []config.ModelEntry, alias, strategy string, st State, pinned string) []config.ModelEntry {
	pool := All(list, alias)
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

// Available 共用健康与模板权重筛选，不排序、不推进计数器。
// 参数 pool：兼容候选；strategy：已解析策略；st：只读状态。返回可选部署。
// 调用：真实调度和预览；非加权策略保持全部冷却时的既有回退规则。
func Available(pool []config.ModelEntry, strategy string, st State) []config.ModelEntry {
	if IsSplitStrategy(strategy) {
		return splitCandidates(pool, st)
	}
	return openCandidates(pool, st)
}
