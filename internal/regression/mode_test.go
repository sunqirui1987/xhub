package regression

import "testing"

// runBoth 把同一条回归跑成两种模式。
//
// simulated 始终执行：上游是本地假服务器，故障可以按模型注入，断言可以核对拨号顺序。
// live 在 XHUB_REGRESSION_LIVE=1 时执行：同一条业务链路改打真实供应商。
// live 为 nil 表示这条链路必须脚本化上游（例如强制 500），真实供应商做不到，
// 于是 live 子测试跳过并写明原因。网关、数据库和记账在 simulated 里仍然是真的。
func runBoth(t *testing.T, simulated func(t *testing.T), live func(t *testing.T)) {
	t.Helper()
	t.Run("simulated", simulated)
	t.Run("live", func(t *testing.T) {
		if !liveEnabled() {
			t.Skip("set XHUB_REGRESSION_LIVE=1 to run this case against the real vendor")
		}
		if live == nil {
			t.Skip("this case scripts the upstream; the simulated run is the one that can force the fault")
		}
		live(t)
	})
}

// runSimulated 用于必须脚本化上游的用例。live 子测试仍会出现，未开 live 或无法注入故障时跳过。
func runSimulated(t *testing.T, simulated func(t *testing.T)) {
	t.Helper()
	runBoth(t, simulated, nil)
}

// openLiveChat 起一个指着真实供应商便宜模型的网关，并建好五层租户。
//
// 供应商从环境里发现，取第一家的第一个模型：这些业务链路用例（额度、名单、密钥）
// 要证明的是网关的记账和鉴权，与具体是哪一家无关，所以不需要把每家都跑一遍。
// 要专门验计量走 pricing_live_test.go，那边会遍历配置里的全部供应商和模型。
//
// 参数 t（*testing.T）：当前测试；name（string）：租户名字前缀。
// 返回 *harness（*harness）：网关；string（string）：管理员会话；chained（chained）：五层租户；
// string（string）：对外模型名。未开 live 或没配供应商时整个用例跳过。
func openLiveChat(t *testing.T, name string) (*harness, string, chained, string) {
	t.Helper()
	vendors := liveCredentials(t)
	vendor := vendors[0]
	model := vendor.ID + "/" + vendor.Models[0]
	h := newHarness(t, liveModelDeployment(vendor, vendor.Models[0]))
	h.live = true
	admin := h.adminSession()
	return h, admin, h.openScope(t, admin, name), model
}
