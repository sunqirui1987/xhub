// 请求发出前的护栏。命中拦截时数据面不再访问上游。
package guard

import (
	"net/http"

	"github.com/sunqirui1987/xhub/internal/auth"
	"github.com/sunqirui1987/xhub/internal/store"
)

// Host 是试跑护栏和读取护栏配置时要问进程要的东西。实现是 *gateway.Server。本包不引用 gateway。
type Host interface {
	RequireManage(w http.ResponseWriter, r *http.Request) *auth.Principal
	DB() *store.Store
}
