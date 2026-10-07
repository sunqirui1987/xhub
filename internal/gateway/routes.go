// routes.go mounts the built-in modules in this order: health, session, keys,
// models, tokens, ingress, access, identity, usage, prefs, guard, family.
// Use rejects a second module with the same name. During New the modules are
// stored and mounted together. A Use call after New mounts that module immediately.

package gateway

import (
	"fmt"
	"net/http"

	"github.com/sunqirui1987/xhub/internal/gateway/family"
	"github.com/sunqirui1987/xhub/internal/gateway/guard"
	"github.com/sunqirui1987/xhub/internal/gateway/identity"
	"github.com/sunqirui1987/xhub/internal/gateway/keys"
	"github.com/sunqirui1987/xhub/internal/gateway/models"
	"github.com/sunqirui1987/xhub/internal/gateway/prefs"
	"github.com/sunqirui1987/xhub/internal/gateway/usage"
	"github.com/sunqirui1987/xhub/internal/httpx"
	"github.com/sunqirui1987/xhub/internal/logx"
	"sync"
)

var logTraceOnceRoutes sync.Once

// Handle implements httpx.Registrar. pattern is "METHOD /path". A pattern already mounted is not replaced by a later module.
// 参数 pattern（string）：要匹配的路径模板或正则；h（http.HandlerFunc）：要挂上的 HTTP 处理函数。
// 返回：无。这条 METHOD /path 已登记。已经挂过的模式不会被后到的模块替换。
// 调用：gateway/family/mount.go、gateway/guard/mount.go、gateway/identity/mount.go、gateway/ingress.go
// 测试：access_log_test.go、console_split_test.go、dial_log_test.go
func (s *Server) Handle(pattern string, h http.HandlerFunc) {
	logTraceOnceRoutes.Do(func() { logx.Trace("enter gateway.Handle") })
	s.handle(pattern, h)
}

// Use mounts a module by name. Mounting the same name again fails, and paths already mounted stay as they are. If the process is already serving, the new module is mounted immediately. During New it isrecorded and mounted later with the others.
// 参数 m（httpx.Module）：正在累加或展示的可挂到网关的模块。
// 返回 error（error）：失败原因，nil 表示这一步成功。
// 调用：仅在 routes.go 内使用
// 测试：无直接单测
func (s *Server) Use(m httpx.Module) error {
	if m == nil || m.Name() == "" {
		return fmt.Errorf("module name is required")
	}
	for _, existing := range s.modules {
		if existing.Name() == m.Name() {
			return fmt.Errorf("module %q is already registered", m.Name())
		}
	}
	s.modules = append(s.modules, m)
	if s.modulesReady {
		m.Mount(s)
	}
	return nil
}

// ModuleNames returns a copy of the module names in mount order.
// 参数：无。
// 返回 []string（[]string）：Module名称。没有匹配时为空切片。
// 调用：仅在 routes.go 内使用
// 测试：无直接单测
func (s *Server) ModuleNames() []string {
	out := make([]string, len(s.modules))
	for i, m := range s.modules {
		out[i] = m.Name()
	}
	return out
}

// installModules mounts the modules the process ships with. A new feature should call Use instead of editing this list.
// 参数：无。
// 调用：gateway/server.go
// 测试：无直接单测
// 返回：无。进程自带的模块已挂上。新功能应走 Use，不要改这份名单。
func (s *Server) installModules() {
	builtins := []httpx.Module{
		healthModule(s),
		sessionModule(s),
		keys.Module(s),
		models.Module(s),
		tokensModule(s),
		ingressModule(s),
		accessModule(s),
		identity.Module(s),
		usage.Module(s),
		prefs.Module(s),
		guard.Module(s),
		family.Module(s),
	}
	for _, m := range builtins {
		if err := s.Use(m); err != nil {
			panic(err)
		}
	}
}

// mountModules attaches modules that were recorded but not yet mounted onto Gin.
// 参数：无。
// 调用：gateway/server.go
// 测试：无直接单测
// 返回：无。已登记但还没挂到 Gin 的模块已挂上，并标记模块就绪。
func (s *Server) mountModules() {
	for _, m := range s.modules {
		m.Mount(s)
	}
	s.modulesReady = true
}

// healthModule serves liveness, readiness, and the dashboard startup config.
// 参数 s（*Server）：healthModule使用的网关进程。
// 返回 httpx.Module（httpx.Module）：可挂到网关上的模块。
// 调用：仅在 routes.go 内使用
// 测试：无直接单测
func healthModule(s *Server) httpx.Module {
	return httpx.Bind("health", func(reg httpx.Registrar) {
		reg.Handle("GET /health/liveliness", s.healthLive)
		reg.Handle("GET /health/liveness", s.healthLive)
		reg.Handle("GET /health/readiness", s.healthReady)
		reg.Handle("GET /health/readiness/details", s.healthDetails)
		reg.Handle("GET /health", s.healthReady)
		reg.Handle("GET /.well-known/litellm-ui-config", s.uiConfig)
		reg.Handle("GET /litellm/.well-known/litellm-ui-config", s.uiConfig)
	})
}

// sessionModule serves username-password login, logout, and the identity the console builds its pages from. The SSO code exchange is gone: it minted an administrator session from an unauthenticated code
// .
// 参数 s（*Server）：会话Module使用的网关进程。
// 返回 httpx.Module（httpx.Module）：可挂到网关上的模块。
// 调用：仅在 routes.go 内使用
// 测试：无直接单测
func sessionModule(s *Server) httpx.Module {
	return httpx.Bind("session", func(reg httpx.Registrar) {
		reg.Handle("POST /login", s.login)
		reg.Handle("POST /v2/login", s.login)
		reg.Handle("POST /v3/login", s.login)
		reg.Handle("POST /logout", s.logout)
		reg.Handle("POST /v2/logout", s.logout)
		reg.Handle("POST /v3/logout", s.logout)
		reg.Handle("GET /auth/me", s.me)
		reg.Handle("POST /auth/logout", s.logout)
		// Bootstrap creates the first platform administrator and runs once. It
		// is the only route that accepts the master key.
		reg.Handle("POST /bootstrap", s.bootstrap)
		reg.Handle("GET /bootstrap/status", s.bootstrapStatus)
		reg.Handle("GET /authorize/flow", s.authorizeFlow)
		reg.Handle("POST /authorize/complete", s.authorizeComplete)
		reg.Handle("POST /v1/mcp/server/oauth/{server_id}/token", s.mcpOAuthToken)
		reg.Handle("GET /public/v1/model_hub", s.publicModelHub)
		reg.Handle("GET /public/v1/model_hub/{facet}", s.publicModelHubFacet)
		reg.Handle("GET /public/model_hub", s.publicModelHub)
		reg.Handle("GET /public/model_hub/info", s.publicModelHubInfo)
		reg.Handle("GET /public/endpoints", s.publicEndpoints)
		reg.Handle("GET /model_hub", s.publicModelHub)
		reg.Handle("GET /model_hub/{facet}", s.publicModelHubFacet)
		reg.Handle("POST /model_hub/update_useful_links", s.updateUsefulLinks)
		reg.Handle("GET /auto_router/shadow_eval", s.shadowEvalList)
	})
}

// tokensModule serves the local token counter and the supported-parameter list.
// 参数 s（*Server）：令牌Module使用的网关进程。
// 返回 httpx.Module（httpx.Module）：可挂到网关上的模块。
// 调用：仅在 routes.go 内使用
// 测试：无直接单测
func tokensModule(s *Server) httpx.Module {
	return httpx.Bind("tokens", func(reg httpx.Registrar) {
		reg.Handle("POST /utils/token_counter", s.tokenCounter)
		reg.Handle("GET /utils/supported_openai_params", s.supportedOpenAIParams)
		reg.Handle("POST /utils/transform_request", s.transformRequest)
	})
}

// ingressModule is the entry for chat, embeddings, completions, messages, and audio translation.
// 参数 s（*Server）：ingressModule使用的网关进程。
// 返回 httpx.Module（httpx.Module）：可挂到网关上的模块。
// 调用：仅在 routes.go 内使用
// 测试：无直接单测
func ingressModule(s *Server) httpx.Module {
	return httpx.Bind("ingress", func(reg httpx.Registrar) {
		reg.Handle("POST /v1/chat/completions", s.chat)
		reg.Handle("POST /chat/completions", s.chat)
		reg.Handle("POST /v1/embeddings", s.embeddings)
		reg.Handle("POST /embeddings", s.embeddings)
		reg.Handle("POST /v1/completions", s.completions)
		reg.Handle("POST /completions", s.completions)
		reg.Handle("POST /v1/messages", s.messages)
		reg.Handle("POST /v1/audio/translations", s.audioTranslations)
		reg.Handle("POST /audio/translations", s.audioTranslations)
	})
}

// accessModule serves cache probes and customers. Email events, IP allow-lists, and SCIM are gone.
// 参数 s（*Server）：accessModule使用的网关进程。
// 返回 httpx.Module（httpx.Module）：可挂到网关上的模块。
// 调用：仅在 routes.go 内使用
// 测试：无直接单测
func accessModule(s *Server) httpx.Module {
	return httpx.Bind("access", func(reg httpx.Registrar) {
		reg.Handle("POST /flushall", s.flushCache)
		reg.Handle("GET /cache/settings", s.cacheSettings)
		reg.Handle("POST /cache/settings", s.cacheSettings)
		reg.Handle("GET /get/ui_theme_settings", s.uiTheme)
		reg.Handle("PATCH /update/ui_theme_settings", s.uiTheme)
		reg.Handle("POST /upload/logo", s.uiTheme)
		reg.Handle("POST /prompts/test", s.promptTest)
		reg.Handle("POST /search_tools/test_connection", s.searchToolTest)
		reg.Handle("GET /cache/ping", s.cachePing)
		reg.Handle("GET /ping", s.cachePing)
		reg.Handle("GET /customer/list", s.customerList)
		reg.Handle("GET /end_user/list", s.customerList)
	})
}
