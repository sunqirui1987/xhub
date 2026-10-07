// Package regression 是网关的端到端回归测试套件，每次改动后都能整体跑一遍。
//
// 它对着一个真实的 Server 走 HTTP，用真实的 PostgreSQL schema；配了 Redis 时，
// 热花费和限流也走真实的 Redis 路径。除了上游供应商，边界上没有任何替身。上游
// 默认是一个本地假服务器，也可以用环境变量切成真实供应商。
//
// 这套测试按发布必须回答的问题排列，顺序和用户提的顺序一致：
//
//  1. 搭租户：加 fennoai 和 qiniu 供应商，再建组织、团队、项目、用户和密钥。
//  2. 用各种端点类型调模型：chat、embedding，以及自定义的 Seedance Bypass。
//  3. 核对返回值、用量日志、用量计数和缓存四者是不是一致。
//  4. 核对额度用尽时，拒绝发生在真正用尽的那一层：个人、密钥、项目、团队、组织。
//     模型名单按团队、项目、密钥收窄。每一层都是一条从放行到拒绝再到恢复的完整链路。
//  5. 核对路由策略、router-settings 和部署回退，以及密钥花费重置、密码重置。
//     同名多部署之间按权重分配流量也在这里。
//  6. 核对护栏在访问上游之前就把请求拦下。
//  7. 核对那笔钱是怎么算出来的：时段、缓存、按秒按张，以及日志详情读的是调用
//     当时存下来的费率，不是今天的价目表。
//
// 跑法：
//
//	go test ./internal/regression/ -v
//	go test ./internal/regression/ -v -run TestBudget
//	./scripts/regression.sh
//
// 每条链路都有 simulated 和 live 两个子测试。simulated 用本地假上游，始终执行。
// live 打真实供应商，默认跳过；打开之后才会花钱：
//
//	XHUB_REGRESSION_LIVE=1 go test ./internal/regression/ -v
//	./scripts/regression.sh --live
//
// 必须脚本化上游才能制造的故障（强制 500、429、卡住一条部署）只在 simulated 里执行。
// 那种 live 子测试会跳过并写明原因。网关、数据库和记账在 simulated 里仍然是真的。
//
// 真实密钥只从环境变量读，绝不写进仓库：
//
//	XHUB_REGRESSION_FENNO_KEY, XHUB_REGRESSION_QINIU_KEY
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
