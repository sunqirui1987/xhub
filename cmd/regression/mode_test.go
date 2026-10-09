package regression

import "testing"

// runBoth 把同一条回归跑成两种模式。
//
// simulated 始终执行：上游是本地假服务器，故障可以按模型注入，断言可以核对拨号顺序。
// live 在 XHUB_REGRESSION_LIVE=1 时执行：同一条业务链路改打真实供应商。
// 参数 t 为上下文，simulated 为本地上游场景，live 为可选真实供应商场景；返回：无。
// 调用：同时存在本地和真实供应商验证的业务链；资源由各场景的隔离夹具清理。
func runBoth(t *testing.T, simulated func(t *testing.T), live func(t *testing.T)) {
	t.Helper()
	t.Run("simulated", simulated)
	if live == nil {
		return
	}
	t.Run("live", func(t *testing.T) {
		if !liveEnabled() {
			t.Skip("set XHUB_REGRESSION_LIVE=1 to run this case against the real vendor")
		}
		live(t)
	})
}

// runSimulated 执行必须脚本化上游的场景，不创建没有实现的 live 子测试。
// 参数 t 为测试上下文，simulated 为本地场景；返回：无；调用：故障注入回归。
// 网关、持久化与记账仍使用真实实现，资源由场景夹具清理。
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
