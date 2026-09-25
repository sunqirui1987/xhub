// 虚拟密钥管理需要的进程能力。实现是 *gateway.Server。本包不引用 gateway。
package keys

import (
	"net/http"

	"github.com/sunqirui1987/xhub/internal/auth"
	"github.com/sunqirui1987/xhub/internal/store"
)

// Host 是创建、列出和更新虚拟密钥时要问进程要的东西。
type Host interface {
	RequireManage(w http.ResponseWriter, r *http.Request) *auth.Principal
	// RequireMixed 管理身份或推理身份都可以。密钥健康检查用它，纯管理接口用 RequireManage。
	RequireMixed(w http.ResponseWriter, r *http.Request) *auth.Principal
	DB() *store.Store
}
