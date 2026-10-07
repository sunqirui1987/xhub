// Package all imports every provider directory so their endpoint types register.
// Adding a supplier is a new directory plus one import line in this file.
package all

import (
	_ "github.com/sunqirui1987/xhub/internal/provider/openai"
	_ "github.com/sunqirui1987/xhub/internal/provider/qiniu"
	_ "github.com/sunqirui1987/xhub/internal/provider/volcengine"

	"github.com/sunqirui1987/xhub/internal/logx"
)

// init 记一次供应商目录的载入。这个包本身只是导入清单，载入这件事值得留一行。
// init 记一次供应商目录的载入。这个包本身只是导入清单，载入这件事值得留一行。
// 参数：无。
// 返回：无。只写一行进程日志。
// 调用：Go 在载入这个包时自动执行。
// 测试：无直接单测
func init() { logx.Debug("provider directories imported") }
