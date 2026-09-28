// Package gateway loads modules. Each feature implements httpx.Module and declares its own paths. This file only mounts the built-in modules by name.
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
func (s *Server) Handle(pattern string, h http.HandlerFunc) {
	logTraceOnceRoutes.Do(func() { logx.Trace("enter gateway.Handle") })
	s.handle(pattern, h)
}

// Use mounts a module by name. Mounting the same name again fails, and paths already mounted stay as they are.
// If the process is already serving, the new module is mounted immediately. During New it is recorded and mounted later with the others.
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
func (s *Server) ModuleNames() []string {
	out := make([]string, len(s.modules))
	for i, m := range s.modules {
		out[i] = m.Name()
	}
	return out
}

// installModules mounts the modules the process ships with. A new feature should call Use instead of editing this list.
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
func (s *Server) mountModules() {
	for _, m := range s.modules {
		m.Mount(s)
	}
	s.modulesReady = true
}

// healthModule serves liveness, readiness, and the dashboard startup config.
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

// sessionModule serves username-password login and the SSO session exchange.
func sessionModule(s *Server) httpx.Module {
	return httpx.Bind("session", func(reg httpx.Registrar) {
		reg.Handle("POST /login", s.login)
		reg.Handle("POST /v2/login", s.login)
		reg.Handle("POST /v3/login", s.login)
		reg.Handle("POST /v3/login/exchange", s.loginExchange)
		reg.Handle("GET /onboarding/get_token", s.onboardingGetToken)
		reg.Handle("POST /onboarding/claim_token", s.onboardingClaim)
		reg.Handle("GET /authorize/flow", s.authorizeFlow)
		reg.Handle("POST /authorize/complete", s.authorizeComplete)
		reg.Handle("POST /v1/mcp/server/oauth/{server_id}/token", s.mcpOAuthToken)
		reg.Handle("GET /public/v1/model_hub", s.publicModelHub)
		reg.Handle("GET /public/v1/model_hub/{facet}", s.publicModelHubFacet)
		reg.Handle("GET /public/model_hub", s.publicModelHub)
		reg.Handle("GET /public/model_hub/info", s.publicModelHubInfo)
		reg.Handle("GET /model_hub", s.publicModelHub)
		reg.Handle("GET /model_hub/{facet}", s.publicModelHubFacet)
		reg.Handle("POST /model_hub/update_useful_links", s.updateUsefulLinks)
		reg.Handle("GET /config_overrides/cyberark", s.configOverride)
		reg.Handle("POST /config_overrides/cyberark", s.configOverride)
		reg.Handle("DELETE /config_overrides/cyberark", s.configOverride)
		reg.Handle("POST /config_overrides/cyberark/test_connection", s.configOverride)
		reg.Handle("GET /config_overrides/hashicorp_vault", s.configOverride)
		reg.Handle("POST /config_overrides/hashicorp_vault", s.configOverride)
		reg.Handle("DELETE /config_overrides/hashicorp_vault", s.configOverride)
		reg.Handle("POST /config_overrides/hashicorp_vault/test_connection", s.configOverride)
		reg.Handle("GET /auto_router/shadow_eval", s.shadowEvalList)
	})
}

// tokensModule serves the local token counter and the supported-parameter list.
func tokensModule(s *Server) httpx.Module {
	return httpx.Bind("tokens", func(reg httpx.Registrar) {
		reg.Handle("POST /utils/token_counter", s.tokenCounter)
		reg.Handle("GET /utils/supported_openai_params", s.supportedOpenAIParams)
		reg.Handle("POST /utils/transform_request", s.transformRequest)
	})
}

// ingressModule is the entry for chat, embeddings, completions, messages, and audio translation.
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

// accessModule serves SSO, email events, cache probes, customers, and SCIM.
func accessModule(s *Server) httpx.Module {
	return httpx.Bind("access", func(reg httpx.Registrar) {
		reg.Handle("GET /sso/key/generate", s.ssoGenerate)
		reg.Handle("GET /email/event_settings", s.emailEventSettings)
		reg.Handle("PATCH /email/event_settings", s.emailEventSettings)
		reg.Handle("POST /email/event_settings/reset", s.emailEventSettingsReset)
		reg.Handle("POST /flushall", s.flushCache)
		reg.Handle("GET /cache/settings", s.cacheSettings)
		reg.Handle("POST /cache/settings", s.cacheSettings)
		reg.Handle("GET /get/allowed_ips", s.allowedIPRoute)
		reg.Handle("POST /add/allowed_ip", s.allowedIPRoute)
		reg.Handle("POST /delete/allowed_ip", s.allowedIPRoute)
		reg.Handle("GET /get/ui_theme_settings", s.uiTheme)
		reg.Handle("PATCH /update/ui_theme_settings", s.uiTheme)
		reg.Handle("POST /upload/logo", s.uiTheme)
		reg.Handle("POST /prompts/test", s.promptTest)
		reg.Handle("POST /search_tools/test_connection", s.searchToolTest)
		reg.Handle("GET /cache/ping", s.cachePing)
		reg.Handle("GET /ping", s.cachePing)
		reg.Handle("POST /config/callback/delete", s.callbackDelete)
		reg.Handle("GET /customer/list", s.customerList)
		reg.Handle("GET /end_user/list", s.customerList)
		reg.Handle("GET /Users", s.scimUsers)
		reg.Handle("GET /Groups", s.scimGroups)
	})
}
