// Package gateway is the process. It wires configuration, the database, and Redis, and it serves health checks, login, and the chat entry.
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
	"github.com/sunqirui1987/xhub/internal/gateway/prefs"
	"github.com/sunqirui1987/xhub/internal/httpx"

	"github.com/sunqirui1987/xhub/internal/hooks"
	"github.com/sunqirui1987/xhub/internal/live"
	"github.com/sunqirui1987/xhub/internal/plugin"
	"github.com/sunqirui1987/xhub/internal/store"
)

const Version = family.ProxyVersion

// Server is the gateway process object. Login is in session.go, the HTTP engine is in engine.go, and the route table is in routes.go.
// Subpackage adapters are in wire.go, spend persistence is in spend.go, and budget and rate checks are in limits.go.
// Keys live in keys, settings in prefs, guardrails in guard, and the dashboard proxy in ui_proxy.go.
// Users and budgets live in identity, models in models, usage in usage, catalog resources in family, and the inference loop in dataplane.
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
	modules            []httpx.Module
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

// New assembles the gateway. It loads the catalog, merges router settings, and registers dedicated routes plus the remaining catalog routes.
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
		uiProxy:            NewProxy(),
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
	// mountCatalog registers each catalog route on Gin by itself. Unregistered paths are not swallowed by a "/" handler that would hide 404s.
	s.mountCatalog()
	return s
}

// Run accepts connections on addr. The process entry uses it instead of handing a ServeMux to ListenAndServe.
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
