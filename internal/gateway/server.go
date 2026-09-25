// 网关进程。组装配置、数据库和 Redis，并提供健康检查、登录和聊天入口。
package gateway

import (
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sunqirui1987/xhub/internal/cache"
	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/gateway/family"
	"github.com/sunqirui1987/xhub/internal/gateway/models"
	"github.com/sunqirui1987/xhub/internal/gateway/module"
	"github.com/sunqirui1987/xhub/internal/gateway/prefs"
	"github.com/sunqirui1987/xhub/internal/gateway/ui"
	"github.com/sunqirui1987/xhub/internal/hooks"
	"github.com/sunqirui1987/xhub/internal/live"
	"github.com/sunqirui1987/xhub/internal/plugin"
	"github.com/sunqirui1987/xhub/internal/store"
)

const Version = family.ProxyVersion

// Server 是网关进程对象。登录在 session.go，HTTP 引擎在 engine.go，路由表在 routes.go，
// 子包适配在 wire.go，花费落库在 spend.go，预算和速率在 limits.go。
// 密钥在 keys，设置在 prefs，护栏在 guard，控制台代理在 ui，
// 用户与预算在 identity，模型在 models，用量在 usage，目录资源在 family，推理循环在 dataplane。
type Server struct {
	Cfg                *config.Config
	Store              *store.Store
	Client             *http.Client
	engine             *gin.Engine
	registered         map[string]struct{}
	Cache              *cache.DualCache
	Hooks              *hooks.Engine
	extensions         *plugin.Registry
	Busy               map[string]int
	Live               *live.Client
	rpmHits            map[string][]time.Time
	tpmHits            map[string][]tokHit
	catalog            []catRoute
	sessions           map[string]sessionRec
	ssoCodes           map[string]bool
	emailEvents        []emailEventSetting
	idem               map[string]idemRec
	uiProxy            http.Handler
	mu                 sync.Mutex
	yamlStoreModelInDB bool
	modules            []module.Module
	modulesReady       bool
}

type idemRec struct {
	Code int
	CT   string
	Body []byte
	Hdr  map[string]string
}

type tokHit struct {
	t time.Time
	n int
}

type sessionRec struct {
	Role   string
	UserID string
}

// 组装网关。会加载目录、合并路由设置，并注册专用路由和剩余的目录路由。
func New(cfg *config.Config, st *store.Store) *Server {
	s := &Server{
		Cfg:                cfg,
		Store:              st,
		Client:             &http.Client{Timeout: time.Duration(cfg.RouterSettings.Timeout) * time.Second},
		engine:             newEngine(),
		registered:         map[string]struct{}{},
		Cache:              cache.New(),
		Hooks:              hooks.New(),
		extensions:         plugin.New(),
		Busy:               map[string]int{},
		rpmHits:            map[string][]time.Time{},
		tpmHits:            map[string][]tokHit{},
		catalog:            loadCatalog(),
		sessions:           map[string]sessionRec{},
		ssoCodes:           map[string]bool{},
		idem:               map[string]idemRec{},
		uiProxy:            ui.NewProxy(),
		yamlStoreModelInDB: cfg.GeneralSettings.StoreModelInDB,
	}
	if cfg.GeneralSettings.RedisURL != "" {
		if client, err := live.Open(cfg.GeneralSettings.RedisURL); err == nil {
			s.Live = client
		}
	}
	prefs.ApplyTyped(s, prefs.MergedRouter(s))
	models.LoadStored(s)
	s.installModules()
	s.mountModules()
	// 每条 catalog 路由单独挂到 Gin。不再用 "/" 把未注册路径收成非 404。
	s.mountCatalog()
	return s
}

// Run 在 addr 上接受连接。进程入口用它，而不是把 ServeMux 交给 ListenAndServe。
func (s *Server) Run(addr string) error {
	if s.Live != nil {
		go s.flushLoop()
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	return (&http.Server{Handler: s.Handler()}).Serve(ln)
}
