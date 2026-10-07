// Package openai 是协议适配这一档的占位目录。
//
// 适配的端点由能力表描述，不由这里登记：一条能力对应一组入口路径，上游走
// llm.Endpoint / llm.Build，由 (op, 供应商) 决定。这个包没有需要登记的数据，
// 所以也没有 init。
//
// 它保留在 all.go 的导入清单里，是因为那份清单就是供应商目录的入口；
// 删掉这一行会让目录少一个来源，没有别的好处。
package openai

import (
	"github.com/sunqirui1987/xhub/internal/logx"
)

// init 记一次载入。这个目录没有需要登记的数据，所以只留这一行。
// 参数：无。
// 返回：无。只写一行进程日志。
// 调用：Go 在载入这个包时自动执行。
// 测试：无直接单测
// init 记一次载入。这个目录没有需要登记的数据，所以只留这一行。
// 参数：无。
// 返回：无。只写一行进程日志。
// 调用：Go 在载入这个包时自动执行。
// 测试：无直接单测
func init() { logx.Trace("enter provider openai") }
