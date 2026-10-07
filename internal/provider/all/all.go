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
func init() { logx.Debug("provider directories imported") }
