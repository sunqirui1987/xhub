package live

// RateNode 定义限流树；Parent 为父节点数组下标，-1 为根，配置来自 IAM 唯一归属树。
type RateNode struct {
	RateScope
	Parent int
}

// RatePlan 包含计数树和调用路径；Path 从 API Key/个人到根，保留固定兄弟容量。
type RatePlan struct {
	Nodes []RateNode
	Path  []int
}

// CheckRatePlan 校验请求能否使用已有分配；plan 是 IAM 构造的无环树和叶到根路径，
// rpm/tpm 按 Nodes 下标提供当前分钟用量，est 是入站 Token 估算，负值按零处理。
// 返回最先拒绝的“层级 字段”或空串；本地计数器和 Redis 对照测试调用，不修改计划或计数。
// 前置条件：用量数组与 Nodes 等长，路径及父下标有效；零 TPM 即使 est=0 也拒绝。
func CheckRatePlan(plan RatePlan, rpm, tpm []int64, est int) string {
	for _, tokens := range []bool{false, true} {
		usage := rpm
		delta := int64(1)
		field := "rpm_limit"
		if tokens {
			usage = tpm
			delta = int64(max(0, est))
			field = "tpm_limit"
		}
		children := make([][]int, len(plan.Nodes))
		for i, n := range plan.Nodes {
			if n.Parent >= 0 {
				children[n.Parent] = append(children[n.Parent], i)
			}
		}
		var reserve func(int) int64
		// 固定节点向父级保留 limit-used；共享节点不新增分配，只透传固定后代的剩余。
		// 已计入父级的历史用量不退回，所以不应再次把已消费部分算成保留容量。
		reserve = func(i int) int64 {
			n := plan.Nodes[i]
			limit := n.RPM
			if tokens {
				limit = n.TPM
			}
			if limit != nil {
				return max(0, int64(*limit)-usage[i])
			}
			sum := int64(0)
			for _, c := range children[i] {
				sum += reserve(c)
			}
			return sum
		}
		child := -1
		credit := int64(0)
		// credit 是调用路径内最近固定节点的未用分配，穿过共享层后仍属于本请求。
		// 在祖先处从当前子树保留中扣除此 credit，防止请求被自己的保留额拦截；
		// 其他兄弟保留不扣除，从而任意请求次序都不能抢走兄弟固定容量。
		for _, i := range plan.Path {
			n := plan.Nodes[i]
			limit := n.RPM
			if tokens {
				limit = n.TPM
			}
			if limit != nil {
				reserved := int64(0)
				for _, c := range children[i] {
					amount := reserve(c)
					if c == child {
						amount = max(0, amount-credit)
					}
					reserved += amount
				}
				// 放行条件：已有总用量 + 不能由本请求使用的保留额 + 本次增量 ≤ 上限。
				// 只读检查全部维度，调用方须在同一锁或脚本内统一提交，保证拒绝不扣量。
				if *limit == 0 || usage[i]+reserved+delta > int64(*limit) {
					return n.Kind + " " + field
				}
				credit = max(0, int64(*limit)-usage[i])
			}
			child = i
		}
	}
	return ""
}
