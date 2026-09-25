// 花费报表和自动路由基准需要的进程能力。数字来自 PostgreSQL，本包不写花费日志。
package usage

import (
	"net/http"

	"github.com/sunqirui1987/xhub/internal/auth"
	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/store"
)

// Host 是用量接口要问进程要的东西。实现是 *gateway.Server。本包不引用 gateway。
type Host interface {
	RequireManage(w http.ResponseWriter, r *http.Request) *auth.Principal
	DB() *store.Store
	// ModelList 返回当前模型表的副本。基准接口只用它找出策略路由器的名字，没有样本时不编造分数。
	ModelList() []config.ModelEntry
}
