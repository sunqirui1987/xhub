// 目录里尚未单独注册的资源形状。推理操作转给 dataplane，其余按键值读写。
package family

import (
	"net/http"

	"github.com/sunqirui1987/xhub/internal/auth"
	"github.com/sunqirui1987/xhub/internal/store"
)

// ProxyVersion 写进目录 config 资源的 version 字段，也是进程健康检查里的 litellm_version。
const ProxyVersion = "xhub-dev"

// Host 是目录资源接口要问进程要的东西。实现是 *gateway.Server。本包不引用 gateway。
type Host interface {
	RequireLLM(w http.ResponseWriter, r *http.Request) *auth.Principal
	RequireMixed(w http.ResponseWriter, r *http.Request) *auth.Principal
	RequireManage(w http.ResponseWriter, r *http.Request) *auth.Principal
	DB() *store.Store
	// DataPlane 把已经识别的推理操作交给上游循环。op 为空时不要调用。
	DataPlane(w http.ResponseWriter, r *http.Request, op string)
	// EnforceIdentityLimits 检查模型允许列表、预算和速率。拒绝时已经写好响应并返回 false。
	EnforceIdentityLimits(w http.ResponseWriter, path string, p *auth.Principal, alias string, est int) bool
}
