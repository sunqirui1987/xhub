package live

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestRateAdmissionRedis 使用真实临时 Redis 验证原子四层窗口、拒绝不扣量、动态设置、分钟恢复及失败；testRedis 清理进程。
func TestRateAdmissionRedis(t *testing.T) {
	c := testRedis(t)
	now := time.Unix(120, 0)
	one, zero := 1, 0
	scopes := []RateScope{{ID: "key/a", Kind: "key", RPM: &one, TPM: &zero}, {ID: "user/a", Kind: "user", RPM: &one}, {ID: "team/a", Kind: "team"}, {ID: "org/a", Kind: "organization"}}
	if got, err := c.AdmitRates(scopes, 1, now); err != nil || got != "key tpm_limit" {
		t.Fatalf("TPM 拒绝: %s %v", got, err)
	}
	scopes[0].TPM = nil
	if got, err := c.AdmitRates(scopes, 1, now); err != nil || got != "" {
		t.Fatalf("拒绝占用 RPM: %s %v", got, err)
	}
	scopes[0].ID = "key/b"
	if got, err := c.AdmitRates(scopes, 1, now); err != nil || got != "user rpm_limit" {
		t.Fatalf("跨 Key 个人汇总: %s %v", got, err)
	}
	if got, err := c.AdmitRates(scopes, 1, now.Add(time.Minute)); err != nil || got != "" {
		t.Fatalf("分钟恢复: %s %v", got, err)
	}
	scopes[2].RPM = &one
	scopes[1].RPM = nil
	scopes[0].RPM = nil
	if got, err := c.AdmitRates(scopes, 1, now.Add(time.Minute)); err != nil || got != "team rpm_limit" {
		t.Fatalf("中途设置丢失已有用量: %s %v", got, err)
	}
	if _, err := c.AdmitRates(nil, 1, now); err != nil {
		t.Fatal(err)
	}
	_ = c.rdb.Close()
	if _, err := c.AdmitRates(scopes, 1, now); err == nil {
		t.Fatal("Redis 故障未返回错误")
	}
}

// TestRateAdmissionRedisConcurrent 验证真实 Redis 跨调用原子上限；100 个并发密钥仅 7 个允许，testRedis 自动清理。
func TestRateAdmissionRedisConcurrent(t *testing.T) {
	c := testRedis(t)
	limit := 7
	now := time.Unix(120, 0)
	var wg sync.WaitGroup
	var accepted atomic.Int32
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			scopes := []RateScope{{ID: fmt.Sprint("key/", id), Kind: "key"}, {ID: "org/shared", Kind: "organization", TPM: &limit}}
			got, err := c.AdmitRates(scopes, 1, now)
			if err != nil {
				t.Error(err)
			}
			if got == "" && err == nil {
				accepted.Add(1)
			}
		}(i)
	}
	wg.Wait()
	if got := accepted.Load(); got != 7 {
		t.Fatalf("并发 TPM 放行 %d，预期 7", got)
	}
}

// TestRatePlanReservations 用真实 Redis 与纯函数对照固定分配保护；覆盖共享透传、部分消费、零和分钟恢复，进程自动清理。
func TestRatePlanReservations(t *testing.T) {
	c := testRedis(t)
	limit := 4
	now := time.Unix(240, 0)
	for _, tokens := range []bool{false, true} {
		t.Run(fmt.Sprint(tokens), func(t *testing.T) {
			root := RateNode{RateScope: RateScope{ID: fmt.Sprint(tokens, "/org"), Kind: "org"}, Parent: -1}
			fixed := RateNode{RateScope: RateScope{ID: fmt.Sprint(tokens, "/fixed"), Kind: "user"}, Parent: 1}
			if tokens {
				root.TPM = &limit
				fixed.TPM = &limit
			} else {
				root.RPM = &limit
				fixed.RPM = &limit
			}
			plan := RatePlan{Nodes: []RateNode{root, {RateScope: RateScope{ID: fmt.Sprint(tokens, "/team"), Kind: "team"}, Parent: 0}, fixed, {RateScope: RateScope{ID: fmt.Sprint(tokens, "/shared"), Kind: "user"}, Parent: 1}}, Path: []int{3, 1, 0}}
			if got := CheckRatePlan(plan, make([]int64, 4), make([]int64, 4), 1); got != "org "+map[bool]string{false: "rpm_limit", true: "tpm_limit"}[tokens] {
				t.Fatalf("共享挤占固定分配: %s", got)
			}
			if got, err := c.AdmitRatePlan(plan, 1, now); err != nil || got == "" {
				t.Fatalf("Redis 共享挤占: %s %v", got, err)
			}
			plan.Path = []int{2, 1, 0}
			for i := 0; i < 4; i++ {
				if got, err := c.AdmitRatePlan(plan, 1, now); err != nil || got != "" {
					t.Fatalf("固定分配未穿过共享父级: %s %v", got, err)
				}
			}
			if got, err := c.AdmitRatePlan(plan, 1, now); err != nil || got == "" {
				t.Fatalf("固定分配超额: %s %v", got, err)
			}
			if got, err := c.AdmitRatePlan(plan, 1, now.Add(time.Minute)); err != nil || got != "" {
				t.Fatalf("恢复失败: %s %v", got, err)
			}
		})
	}
}

// TestRatePlanPartialReservations 对照纯函数和真实 Redis 验证 10 总量、两个固定 4、共享 2；覆盖透传、所有调用顺序和分钟恢复，临时 Redis 自动清理。
func TestRatePlanPartialReservations(t *testing.T) {
	c := testRedis(t)
	for _, tokens := range []bool{false, true} {
		for _, order := range [][]int{{4, 4, 2, 2, 2, 2, 3, 3, 3, 3}, {2, 4, 3, 4, 2, 3, 2, 3, 2, 3}, {2, 2, 2, 2, 3, 3, 3, 3, 4, 4}} {
			t.Run(fmt.Sprint(tokens, order), func(t *testing.T) {
				total, fixed := 10, 4
				plan := RatePlan{Nodes: []RateNode{
					{RateScope: RateScope{ID: t.Name() + "/org", Kind: "org"}, Parent: -1},
					{RateScope: RateScope{ID: t.Name() + "/team", Kind: "team"}, Parent: 0},
					{RateScope: RateScope{ID: t.Name() + "/a", Kind: "user"}, Parent: 1},
					{RateScope: RateScope{ID: t.Name() + "/b", Kind: "user"}, Parent: 1},
					{RateScope: RateScope{ID: t.Name() + "/shared", Kind: "user"}, Parent: 1},
				}}
				if tokens {
					plan.Nodes[0].TPM = &total
					plan.Nodes[2].TPM = &fixed
					plan.Nodes[3].TPM = &fixed
				} else {
					plan.Nodes[0].RPM = &total
					plan.Nodes[2].RPM = &fixed
					plan.Nodes[3].RPM = &fixed
				}
				rpm, tpm := make([]int64, 5), make([]int64, 5)
				now := time.Unix(360, 0)
				for _, id := range order {
					plan.Path = []int{id, 1, 0}
					if pure := CheckRatePlan(plan, rpm, tpm, 1); pure != "" {
						t.Fatalf("预留容量内调用被纯函数拒绝: %s", pure)
					}
					if got, err := c.AdmitRatePlan(plan, 1, now); err != nil || got != "" {
						t.Fatalf("预留容量内 Redis 调用失败: %s %v", got, err)
					}
					for _, i := range plan.Path {
						rpm[i]++
						tpm[i]++
					}
					if id == 4 && rpm[4] == 2 {
						pure := CheckRatePlan(plan, rpm, tpm, 1)
						got, err := c.AdmitRatePlan(plan, 1, now)
						if pure == "" || got != pure || err != nil {
							t.Fatalf("共享第三次挤占固定分配: pure=%s redis=%s err=%v", pure, got, err)
						}
					}
				}
				if got, err := c.AdmitRatePlan(plan, 1, now.Add(time.Minute)); err != nil || got != "" {
					t.Fatalf("新分钟未恢复: %s %v", got, err)
				}
			})
		}
	}
}
