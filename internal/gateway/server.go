// server.go holds the Server value and New. Login is session.go, the HTTP
// front door is engine.go, route mounting is routes.go and ingress.go, spend
// is spend.go, and the Host adapters are wire.go.

package gateway

import (
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sunqirui1987/xhub/internal/authz"
	"github.com/sunqirui1987/xhub/internal/cache"
	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/dataplane"
	"github.com/sunqirui1987/xhub/internal/gateway/family"
	"github.com/sunqirui1987/xhub/internal/gateway/models"
	"github.com/sunqirui1987/xhub/internal/gateway/prefs"
	"github.com/sunqirui1987/xhub/internal/httpx"
	"github.com/sunqirui1987/xhub/internal/iam"

	"github.com/sunqirui1987/xhub/internal/hooks"
	"github.com/sunqirui1987/xhub/internal/live"
	"github.com/sunqirui1987/xhub/internal/logx"
	"github.com/sunqirui1987/xhub/internal/plugin"
	"github.com/sunqirui1987/xhub/internal/store"

	_ "github.com/sunqirui1987/xhub/internal/provider/all"
)

const Version = family.ProxyVersion

// Server is one gateway process. Fields are grouped by who writes them.
// Request-scoped maps below are keyed by call id and deleted when the spend
// row is written, so a handler must AnnotateCall and RememberExchange before
// RecordSpend.
type Server struct {
	Cfg   *config.Config
	Store *store.Store
	// IAM is users, teams, organizations, projects, keys, and the audit log.
	// Authz is the only authorization check over that store.
	IAM   *iam.DB
	Authz *authz.Authorizer
	// Client is the upstream HTTP client. Its timeout is the router timeout.
	Client *http.Client
	// engine is the Gin mux. registered remembers "METHOD pattern" so a catalog
	// route cannot replace a module route that was mounted first.
	engine     *gin.Engine
	registered map[string]struct{}
	Cache      *cache.DualCache
	Hooks      *hooks.Engine
	extensions *plugin.Registry
	// Busy counts in-flight calls per deployment id inside this process.
	Busy map[string]int
	// Live is Redis. Nil means spend is written to PostgreSQL inside the request
	// and affinity pins stay in the map below.
	Live *live.Client
	// rpmHits and tpmHits are the in-process rate windows used when Redis is off.
	rpmHits map[string][]time.Time
	tpmHits map[string][]tokHit
	// catalog is the routes.json table mounted after the modules.
	catalog []catRoute
	// sessions and ssoCodes are the console login state. emailEvents is the
	// notification settings page. idem is the Idempotency-Key replay buffer.
	sessions    map[string]sessionRec
	ssoCodes    map[string]bool
	emailEvents []emailEventSetting
	idem        map[string]idemRec
	mu          sync.Mutex
	// yamlStoreModelInDB is the startup flag. A database override can change
	// the same behavior later through prefs.
	yamlStoreModelInDB bool
	modules            []httpx.Module
	modulesReady       bool
	// exchanges holds headers and bodies until recordSpend copies them onto the
	// usage row. callNotes holds provider, TTFT, session, and deployment.
	// guardrailNotes holds the rule name when a guardrail refused the call.
	exchanges      map[string]promptExchange
	callNotes      map[string]dataplane.CallNote
	guardrailNotes map[string]string
	// affinity is the session pin and the official-task pin. Official task pins
	// use a 7-day TTL; chat session pins use affinityTTL. affinityMu guards
	// this map only. mu guards the other process-local maps.
	affinity   map[string]affinityPin
	affinityMu sync.Mutex
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

// ValidateKeyRelations is gone with the old store: a key's ownership is
// resolved from the database by the authorization layer on every request, so
// there is no separate validation step that could disagree with it.

var logTraceOnceServer sync.Once

// New assembles the gateway. It loads the catalog, merges router settings, and registers dedicated routes plus the remaining catalog routes. The identity store is required: a gateway without one could not tell who is calling.
// 调用：authz/authz.go、authz/decide.go、cache/cache.go、dataplane/serve.go
// 测试：activity_http_test.go、authz_test.go、builtin_providers_test.go
// 参数 cfg（*config.Config）：进程配置，含模型表和路由策略；st（*store.Store）：此刻的冷却、延迟、用量和并发，用来排序；db（*iam.DB）：身份和用量库。
// 返回 *Server（*Server）：装好模块和目录路由的网关进程。
func New(cfg *config.Config, st *store.Store, db *iam.DB) *Server {
	logTraceOnceServer.Do(func() { logx.Trace("enter gateway.New") })

	s := &Server{
		Cfg:                cfg,
		Store:              st,
		IAM:                db,
		Authz:              authz.New(db),
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
		yamlStoreModelInDB: cfg.GeneralSettings.StoreModelInDB,
	}
	if cfg.GeneralSettings.RedisURL != "" {
		if client, err := live.Open(cfg.GeneralSettings.RedisURL); err == nil {
			s.Live = client
		}
	}
	prefs.ApplyTyped(s, prefs.MergedRouter(s))
	models.LoadStored(s)
	// Hand-entered price rows and suppliers are laid over the embedded Modelink
	// catalog before any request is served.
	models.LoadPriceOverrides(s)
	// An armed reload plan keeps the market prices current without a restart.
	models.StartScheduledReload(s)
	s.seedAdmin()
	s.installModules()
	s.mountModules()
	// mountCatalog registers each catalog route on Gin by itself. Unregistered paths are not swallowed by a "/" handler that would hide 404s.
	s.mountCatalog()
	return s
}

// Run accepts connections on addr. The process entry uses it instead of handing a ServeMux to ListenAndServe.
// 参数 addr（string）：Run使用的addr。空串表示调用方没有提供这项。
// 返回 error（error）：失败原因，nil 表示这一步成功。
// 调用：dataplane/serve.go、live/redis.go、plugin/registry.go
// 测试：authz_test.go、catalog_reads_test.go、chains_test.go
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
