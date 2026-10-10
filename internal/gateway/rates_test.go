package gateway

import (
	"fmt"
	"github.com/sunqirui1987/xhub/internal/live"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestHierarchicalLocalRates 验证四层合并计数、零/共享、拒绝不计数及分钟恢复；内存前置，无数据清理需求。
func TestHierarchicalLocalRates(t *testing.T) {
	for _, kind := range []string{"key", "user", "team", "organization"} {
		for _, dimension := range []string{"rpm", "tpm"} {
			t.Run(kind+"/"+dimension, func(t *testing.T) {
				s := &Server{}
				now := time.Unix(120, 0)
				limit := 2
				scopes := []live.RateScope{{ID: "key/a", Kind: "key"}, {ID: "user/a", Kind: "user"}, {ID: "team/a", Kind: "team"}, {ID: "org/a", Kind: "organization"}}
				for i := range scopes {
					if scopes[i].Kind == kind {
						if dimension == "rpm" {
							scopes[i].RPM = &limit
						} else {
							scopes[i].TPM = &limit
						}
					}
				}
				for i := 0; i < 2; i++ {
					if got := s.admitLocalRates(scopes, 1, now); got != "" {
						t.Fatal(got)
					}
				}
				want := kind + " " + dimension + "_limit"
				if got := s.admitLocalRates(scopes, 1, now); got != want {
					t.Fatalf("拒绝层: %s != %s", got, want)
				}
				if got := s.admitLocalRates(scopes, 1, now.Add(time.Minute)); got != "" {
					t.Fatalf("分钟未恢复: %s", got)
				}
				limit = 0
				if got := s.admitLocalRates(scopes, 0, now.Add(2*time.Minute)); got != want {
					t.Fatalf("零未拦截: %s", got)
				}
			})
		}
	}
	s := &Server{}
	now := time.Unix(120, 0)
	one := 1
	zero := 0
	scopes := []live.RateScope{{ID: "key/a", Kind: "key", RPM: &one, TPM: &zero}, {ID: "user/a", Kind: "user", RPM: &one}}
	if got := s.admitLocalRates(scopes, 1, now); got != "key tpm_limit" {
		t.Fatal(got)
	}
	scopes[0].TPM = nil
	if got := s.admitLocalRates(scopes, 1, now); got != "" {
		t.Fatalf("拒绝消耗了 RPM: %s", got)
	}
	scopes[0].ID = "key/b"
	if got := s.admitLocalRates(scopes, 1, now); got != "user rpm_limit" {
		t.Fatalf("更换密钥绕过个人限制: %s", got)
	}
}

// TestHierarchicalLocalRatesConcurrent 验证 100 个并发密钥共享团队限额严格放行 7 个；前置纯内存，退出后无残留。
func TestHierarchicalLocalRatesConcurrent(t *testing.T) {
	s := &Server{}
	limit := 7
	now := time.Unix(120, 0)
	var wg sync.WaitGroup
	var accepted atomic.Int32
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			scopes := []live.RateScope{{ID: fmt.Sprint("key/", id), Kind: "key"}, {ID: "team/shared", Kind: "team", RPM: &limit}}
			if s.admitLocalRates(scopes, 1, now) == "" {
				accepted.Add(1)
			}
		}(i)
	}
	wg.Wait()
	if got := accepted.Load(); got != 7 {
		t.Fatalf("并发放行 %d，预期 7", got)
	}
}
