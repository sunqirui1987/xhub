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

// openLiveChat 起一个指着 fennoai 便宜模型的网关，并建好五层租户。
func openLiveChat(t *testing.T, name string) (*harness, string, chained, string) {
	t.Helper()
	keys := liveCredentials(t)
	const model = "fennoai/gpt-5.6-sol"
	h := newHarness(t, liveChatDeployment(model, "fennoai", "openai", liveChatBase("fennoai"), keys.fenno))
	h.live = true
	admin := h.adminSession()
	return h, admin, h.openScope(t, admin, name), model
}
