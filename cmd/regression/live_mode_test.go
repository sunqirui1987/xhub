package regression

import (
	"os"

	"github.com/sunqirui1987/xhub/internal/logx"
)

// liveModeEnabled 报告这次是否要调用真实供应商，并把这个选择写进进程日志。
// 一个会在本地假上游和付费供应商之间悄悄切换的套件，光看输出是读不出来的，
// 所以在这里把选择说出来。
// 参数：无。
// 返回 bool（bool）：环境变量 XHUB_REGRESSION_LIVE 为 1 时返回真，表示这次调用真实供应商。
// 调用：harness_test.go 的 liveEnabled，供 live_test.go 决定跳过还是执行。
// 测试：live_test.go 在未设置环境变量时整体跳过。
func liveModeEnabled() bool {
	live := os.Getenv("XHUB_REGRESSION_LIVE") == "1"
	if live {
		logx.Info("regression suite running with live vendor calls")
		return true
	}
	logx.Debug("regression suite running against the fake provider")
	return false
}
