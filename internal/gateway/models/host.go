// 模型管理需要的进程能力。实现是 *gateway.Server。本包不引用 gateway，避免循环导入。
package models

import (
	"net/http"

	"github.com/sunqirui1987/xhub/internal/auth"
	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/store"
)

// Host 是创建、更新和列出模型时要问进程要的东西。
// 更新、删除和屏蔽在持锁期间写数据库，锁的范围和拆包之前的 mu 一样。
type Host interface {
	RequireManage(w http.ResponseWriter, r *http.Request) *auth.Principal
	DB() *store.Store
	// LockModels 与 UnlockModels 成对使用。请求路径上改模型表之前必须先锁。
	LockModels()
	UnlockModels()
	// ModelTable 返回进程内模型切片的指针。持锁时可以改；LoadStored 在对外服务前调用，那时还没有并发请求。
	ModelTable() *[]config.ModelEntry
	// Resolve 解析会话或虚拟密钥。失败时不写响应，由调用方决定 401 的正文。
	Resolve(r *http.Request) (*auth.Principal, error)
	// AllowLLM 报告这个身份能否调用推理。主密钥默认不能，除非进程打开了 allow_master_key_llm。
	AllowLLM(p *auth.Principal) bool
}
