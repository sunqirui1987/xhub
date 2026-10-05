// Package gateway is the process. It wires configuration, the database, and Redis, and it serves health checks, login, and the chat entry.
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
)

const Version = family.ProxyVersion

// Server is the gateway process object. Login is in session.go, the HTTP engine is in engine.go, and the route table is in routes.go.
// Subpackage adapters are in wire.go, spend persistence is in spend.go, and budget and rate checks are in limits.go.
// Keys live in keys, settings in prefs, and guardrails in guard. The console is a separate process; this server does not proxy it.
// Users and budgets live in identity, models in models, usage in usage, catalog resources in family, and the inference loop in dataplane.
type Server struct {
	Cfg                *config.Config
	Store              *store.Store
	// IAM is the identity store: users, teams, organizations, projects, keys
	// and the audit log. Authz is the single authorization layer over it.
	IAM                *iam.DB
	Authz              *authz.Authorizer
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
	mu                 sync.Mutex
	yamlStoreModelInDB bool
	modules            []httpx.Module
	modulesReady       bool
	exchanges          map[string]promptExchange
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

// New assembles the gateway. It loads the catalog, merges router settings, and registers dedicated routes plus the remaining catalog routes.
// The identity store is required: a gateway without one could not tell who is calling.
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
	s.seedAdmin()
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
