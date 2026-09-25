// 路由设置和通用设置的 HTTP。数据库里出现的键覆盖 YAML，没出现的键保留。
package prefs

import (
	"net/http"

	"github.com/sunqirui1987/xhub/internal/auth"
	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/store"
)

// Host 是读写设置时要问进程要的东西。实现是 *gateway.Server。本包不引用 gateway。
// Config 返回进程内配置指针。ApplyTyped 会改其中的路由策略、重试和超时，范围与拆包前相同，不额外加锁。
type Host interface {
	RequireManage(w http.ResponseWriter, r *http.Request) *auth.Principal
	DB() *store.Store
	Config() *config.Config
}
