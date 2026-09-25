// 模块装载。每个功能实现 module.Module，自己声明路径；这里只按名字把内置模块装上。
package gateway

import (
	"fmt"
	"net/http"

	"github.com/sunqirui1987/xhub/internal/gateway/family"
	"github.com/sunqirui1987/xhub/internal/gateway/guard"
	"github.com/sunqirui1987/xhub/internal/gateway/identity"
	"github.com/sunqirui1987/xhub/internal/gateway/keys"
	"github.com/sunqirui1987/xhub/internal/gateway/models"
	"github.com/sunqirui1987/xhub/internal/gateway/module"
	"github.com/sunqirui1987/xhub/internal/gateway/prefs"
	"github.com/sunqirui1987/xhub/internal/gateway/usage"
)

// Handle 满足 module.Registrar。pattern 是「方法 路径」。已挂过的模式不会被后来的模块覆盖。
func (s *Server) Handle(pattern string, h http.HandlerFunc) { s.handle(pattern, h) }

// Use 按名字装上一个模块。同名再次装入会失败，已挂上的路径保持不变。
// 进程已经对外服务时，新模块会立刻挂上；还在 New 里时先记下，等 mountModules 一起挂。
func (s *Server) Use(m module.Module) error {
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

// ModuleNames 按装入顺序返回模块名的副本。
func (s *Server) ModuleNames() []string {
	out := make([]string, len(s.modules))
	for i, m := range s.modules {
		out[i] = m.Name()
	}
	return out
}

// installModules 装上进程自带的模块。新增功能优先用 Use，不必改这些名字。
func (s *Server) installModules() {
	builtins := []module.Module{
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

// mountModules 把已经装入、但还没挂路由的模块挂到 Gin。
func (s *Server) mountModules() {
	for _, m := range s.modules {
		m.Mount(s)
	}
	s.modulesReady = true
}

// healthModule 是存活、就绪和控制台启动配置。
func healthModule(s *Server) module.Module {
	return module.Bind("health", func(reg module.Registrar) {
		reg.Handle("GET /health/liveliness", s.healthLive)
		reg.Handle("GET /health/liveness", s.healthLive)
		reg.Handle("GET /health/readiness", s.healthReady)
		reg.Handle("GET /health/readiness/details", s.healthDetails)
		reg.Handle("GET /health", s.healthReady)
		reg.Handle("GET /.well-known/litellm-ui-config", s.uiConfig)
		reg.Handle("GET /litellm/.well-known/litellm-ui-config", s.uiConfig)
	})
}

// sessionModule 是用户名密码登录和 SSO 换会话。
func sessionModule(s *Server) module.Module {
	return module.Bind("session", func(reg module.Registrar) {
		reg.Handle("POST /login", s.login)
		reg.Handle("POST /v2/login", s.login)
		reg.Handle("POST /v3/login", s.login)
		reg.Handle("POST /v3/login/exchange", s.loginExchange)
	})
}

// tokensModule 是本地 token 计数和支持的参数。
func tokensModule(s *Server) module.Module {
	return module.Bind("tokens", func(reg module.Registrar) {
		reg.Handle("POST /utils/token_counter", s.tokenCounter)
		reg.Handle("GET /utils/supported_openai_params", s.supportedOpenAIParams)
	})
}

// ingressModule 是聊天、嵌入、补全、消息和语音翻译的入口。
func ingressModule(s *Server) module.Module {
	return module.Bind("ingress", func(reg module.Registrar) {
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

// accessModule 是 SSO、邮件事件、缓存探测、客户和 SCIM。
func accessModule(s *Server) module.Module {
	return module.Bind("access", func(reg module.Registrar) {
		reg.Handle("GET /sso/key/generate", s.ssoGenerate)
		reg.Handle("GET /email/event_settings", s.emailEventSettings)
		reg.Handle("PATCH /email/event_settings", s.emailEventSettings)
		reg.Handle("POST /email/event_settings/reset", s.emailEventSettingsReset)
		reg.Handle("POST /flushall", s.flushCache)
		reg.Handle("GET /cache/settings", s.cacheSettings)
		reg.Handle("POST /cache/settings", s.cacheSettings)
		reg.Handle("GET /cache/ping", s.cachePing)
		reg.Handle("GET /ping", s.cachePing)
		reg.Handle("POST /config/callback/delete", s.callbackDelete)
		reg.Handle("GET /customer/list", s.customerList)
		reg.Handle("GET /end_user/list", s.customerList)
		reg.Handle("GET /Users", s.scimUsers)
		reg.Handle("GET /Groups", s.scimGroups)
	})
}
