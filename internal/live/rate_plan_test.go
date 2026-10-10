package live

import (
	"fmt"
	"testing"
)

// TestRatePlanAllOrders 穷举小容量完整四层树的全部调用顺序，验证固定 2+2 与共享 1 的充分性和上界。
// 前置纯内存配置；期望来自各叶分配，不复用实现算法。容量内都放行，任一池多一次都拒绝且没有副作用，无需清理。
func TestRatePlanAllOrders(t *testing.T) {
	for _, tokens := range []bool{false, true} {
		t.Run(fmt.Sprint(tokens), func(t *testing.T) {
			total, fixed := 5, 2
			plan := RatePlan{Nodes: []RateNode{
				{RateScope: RateScope{ID: "org/a", Kind: "org"}, Parent: -1},
				{RateScope: RateScope{ID: "team/a", Kind: "team"}, Parent: 0},
				{RateScope: RateScope{ID: "user/a", Kind: "user"}, Parent: 1},
				{RateScope: RateScope{ID: "key/a", Kind: "key"}, Parent: 2},
				{RateScope: RateScope{ID: "user/b", Kind: "user"}, Parent: 1},
				{RateScope: RateScope{ID: "key/b", Kind: "key"}, Parent: 4},
				{RateScope: RateScope{ID: "user/shared", Kind: "user"}, Parent: 1},
				{RateScope: RateScope{ID: "key/shared", Kind: "key"}, Parent: 6},
			}}
			if tokens {
				plan.Nodes[0].TPM = &total
				plan.Nodes[3].TPM = &fixed
				plan.Nodes[4].TPM = &fixed
			} else {
				plan.Nodes[0].RPM = &total
				plan.Nodes[3].RPM = &fixed
				plan.Nodes[4].RPM = &fixed
			}
			paths := [][]int{{3, 2, 1, 0}, {5, 4, 1, 0}, {7, 6, 1, 0}}
			capacities := []int{2, 2, 1}
			orders := 0
			// 每个前缀都尝试三种下一请求，包含超限失败分支；仅沿容量内的分支继续枚举。
			var visit func([]int, []int64, []int64)
			visit = func(used []int, rpm, tpm []int64) {
				if used[0]+used[1]+used[2] == total {
					orders++
				}
				for candidate, path := range paths {
					plan.Path = path
					rejected := CheckRatePlan(plan, rpm, tpm, 1)
					if used[candidate] == capacities[candidate] {
						if rejected == "" {
							t.Fatalf("已用%v，第%d池多一次仍放行", used, candidate)
						}
						continue
					}
					if rejected != "" {
						t.Fatalf("已用%v，第%d池保留容量被挤占: %s", used, candidate, rejected)
					}
					nextRPM, nextTPM := append([]int64(nil), rpm...), append([]int64(nil), tpm...)
					for _, i := range path {
						nextRPM[i]++
						nextTPM[i]++
					}
					used[candidate]++
					visit(used, nextRPM, nextTPM)
					used[candidate]--
				}
			}
			visit(make([]int, 3), make([]int64, 8), make([]int64, 8))
			// 5!/(2!×2!×1!) = 30，枚举总数必须匹配，否则可能漏掉最坏调用次序。
			if orders != 30 {
				t.Fatalf("完整顺序数 %d，预期 30", orders)
			}
		})
	}
}
