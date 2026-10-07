// Package dataplane sends one model call and does not import the gateway
// process. The process implements Host and calls Serve or ServeBypass.
//
// Two loops live here:
//
//   - serve.go handles adapted operations. It picks a deployment, asks llm
//     to encode the body, and tries the next deployment after a failure.
//     stream.go copies the SSE response. usage.go fills token counts the
//     upstream left out. log.go redacts secrets before a log line.
//
//   - official.go handles bypass operations. The body is forwarded with the
//     model field replaced. Task polls return to the deployment that created
//     the task.
//
// Both loops call Host.RememberExchange, Host.AnnotateCall, and
// Host.RecordSpend so the usage row is written by the process, not here.
// live.go is the Redis side of routing state and the spend flush.
package dataplane

import (
	"github.com/sunqirui1987/xhub/internal/logx"
)

// init 记一次包载入。这个文件只放包文档，没有别的可记的事。
// 参数：无。
// 返回：无。只写一行进程日志。
// 调用：Go 在载入这个包时自动执行。
// 测试：无直接单测
// init 记一次包载入。这个文件只放包文档，没有别的可记的事。
// 参数：无。
// 返回：无。只写一行进程日志。
// 调用：Go 在载入这个包时自动执行。
// 测试：无直接单测
func init() { logx.Trace("enter dataplane") }
