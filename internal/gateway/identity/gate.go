// 用户、团队、组织、项目和预算的 HTTP 处理。路由由本包的 Module 挂上，本包不引用 gateway。
package identity

import (
	"net/http"

	"github.com/sunqirui1987/xhub/internal/auth"
	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/store"
)

// Gate 是这些管理接口需要的进程能力。实现是 *gateway.Server。
type Gate interface {
	RequireManage(w http.ResponseWriter, r *http.Request) *auth.Principal
	DB() *store.Store
	MakeKey(plain string, body map[string]any) (store.Key, error)
	KeyJSON(k store.Key, plain string, includePlain bool) map[string]any
	ModelList() []config.ModelEntry
	ModelPublic(m config.ModelEntry) map[string]any
}
