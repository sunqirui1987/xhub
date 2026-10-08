// Package config loads process configuration. It accepts PostgreSQL only and keeps YAML keys the typed structs do not declare.
package config

import (
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/sunqirui1987/xhub/internal/logx"
	"gopkg.in/yaml.v3"
	"sync"
)

// Config is the loaded process configuration. RouterRaw and GeneralRaw keep YAML keys the typed structs do not name, so a database overlay can compare by key.
type Config struct {
	ModelList       []ModelEntry    `yaml:"model_list"`
	RouterSettings  RouterSettings  `yaml:"router_settings"`
	LiteLLMSettings map[string]any  `yaml:"litellm_settings"`
	GeneralSettings GeneralSettings `yaml:"general_settings"`
	// Raw maps keep YAML keys the typed structs do not name. Database overlay wins per key.
	RouterRaw  map[string]any `yaml:"-"`
	GeneralRaw map[string]any `yaml:"-"`
}

// ModelEntry is one deployment. ModelName is the public model name and LiteLLMParams are the upstream parameters.
type ModelEntry struct {
	ModelName     string         `yaml:"model_name"`
	LiteLLMParams map[string]any `yaml:"litellm_params"`
	ModelInfo     map[string]any `yaml:"model_info"`
}

// Disabled reports whether this deployment is excluded from model discovery and runtime routing.
// 参数：接收者包含部署的 model_info。
// 返回 bool：model_info.disabled 明确为 true 时返回真，缺失或类型不符时返回假。
// 调用：模型发现、路由和数据面在挑选候选部署时调用。
// 测试：router/disabled_test.go、dataplane/disabled_test.go、gateway/models/list_test.go。
func (m ModelEntry) Disabled() bool {
	disabled, _ := m.ModelInfo["disabled"].(bool)
	return disabled
}

// RouterSettings holds the routing strategy, retry count, and timeout. Other router keys from YAML stay in Config.RouterRaw.
type RouterSettings struct {
	RoutingStrategy string  `yaml:"routing_strategy"`
	NumRetries      int     `yaml:"num_retries"`
	Timeout         float64 `yaml:"timeout"`
}

// GeneralSettings holds the database, Redis, the platform administrator, and a
// few switches. Other general-settings keys stay in Config.GeneralRaw.
//
// MasterKey is optional and is not a login. The console administrator is the
// admin_email account. An empty master key means there is no emergency
// credential.
type GeneralSettings struct {
	MasterKey               string `yaml:"master_key"`
	DatabaseURL             string `yaml:"database_url"`
	RedisURL                string `yaml:"redis_url"`
	StoreModelInDB          bool   `yaml:"store_model_in_db"`
	StorePromptsInSpendLogs bool   `yaml:"store_prompts_in_spend_logs"`
	AllowMasterKeyLLM       bool   `yaml:"allow_master_key_llm"`
	// AdminEmail and AdminPassword seed the first platform administrator at
	// startup, so a deployment does not have to call POST /bootstrap by hand.
	//
	// The password is an initial password, not a managed one: the account is
	// created only when no account with AdminEmail exists yet, and a later
	// change to this value never rewrites a stored password. Changing the
	// password of a live account goes through the normal account routes.
	//
	// Both values accept the os.environ/NAME form every other config string does,
	// so a password can come from the environment instead of the file.
	AdminEmail    string `yaml:"admin_email"`
	AdminPassword string `yaml:"admin_password"`
	AdminName     string `yaml:"admin_name"`
	// DisableEnvCredentialLogin turns the seeding off entirely. It is the same
	// switch LiteLLM uses to refuse the environment-credential account, so an
	// operator who has already provisioned accounts is not given a second
	// administrator by a config file.
	DisableEnvCredentialLogin bool `yaml:"disable_env_credential_login"`
}

var logTraceOnceConfig sync.Once

// Load reads YAML and requires a postgres:// or postgresql:// database URL.
// 参数 path（string）：YAML 配置文件的磁盘路径，不是请求 URL。
// 调用：catalog/classify.go、gateway/catalog.go
// 测试：activity_http_test.go、builtin_providers_test.go、chains_test.go
// 返回：解析后的进程配置。读文件、YAML 或数据库 URL 协议校验失败时返回错误。
func Load(path string) (*Config, error) {
	logTraceOnceConfig.Do(func() { logx.Trace("enter config.Load") })

	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Config
	if err := yaml.Unmarshal(raw, &c); err != nil {
		return nil, err
	}
	c.GeneralSettings.MasterKey = resolve(c.GeneralSettings.MasterKey)
	c.GeneralSettings.DatabaseURL = resolve(c.GeneralSettings.DatabaseURL)
	c.GeneralSettings.RedisURL = resolve(c.GeneralSettings.RedisURL)
	c.GeneralSettings.AdminEmail = resolve(c.GeneralSettings.AdminEmail)
	c.GeneralSettings.AdminPassword = resolve(c.GeneralSettings.AdminPassword)
	c.GeneralSettings.AdminName = resolve(c.GeneralSettings.AdminName)
	var doc struct {
		RouterSettings  map[string]any `yaml:"router_settings"`
		GeneralSettings map[string]any `yaml:"general_settings"`
		LiteLLMSettings map[string]any `yaml:"litellm_settings"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	c.RouterRaw = doc.RouterSettings
	c.GeneralRaw = doc.GeneralSettings
	if c.LiteLLMSettings == nil {
		c.LiteLLMSettings = doc.LiteLLMSettings
	}
	if c.GeneralSettings.DatabaseURL == "" {
		return nil, fmt.Errorf("general_settings.database_url is required and must be a postgres:// URL")
	}
	dbURL, err := url.Parse(c.GeneralSettings.DatabaseURL)
	if err != nil || (dbURL.Scheme != "postgres" && dbURL.Scheme != "postgresql") {
		return nil, fmt.Errorf("general_settings.database_url must be a postgres:// or postgresql:// URL")
	}
	if c.RouterSettings.RoutingStrategy == "" {
		c.RouterSettings.RoutingStrategy = "simple-shuffle"
	}
	if c.RouterSettings.NumRetries == 0 {
		c.RouterSettings.NumRetries = 2
	}
	if c.RouterSettings.Timeout == 0 {
		c.RouterSettings.Timeout = 60
	}
	for i := range c.ModelList {
		for k, v := range c.ModelList[i].LiteLLMParams {
			if s, ok := v.(string); ok {
				c.ModelList[i].LiteLLMParams[k] = resolve(s)
			}
		}
	}
	return &c, nil
}

// resolve expands environment placeholders in a config string. A variable that is not set becomes the empty string, which is what makes "os.environ/NAME" safe to write: the feature that key configures stays off rather than acting on placeholder text.
// 参数 s（string）：解析要处理的文本。空串表示这段没有内容。
// 返回 string（string）：展开环境变量后的配置文本。变量不存在时保留原样或变成空，由占位符写法决定。
// 调用：Load 在展开每条部署的 litellm_params 时。
// 测试：无直接单测
func resolve(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "os.environ/") {
		return os.Getenv(strings.TrimPrefix(s, "os.environ/"))
	}
	return s
}

// ParamString 从一条部署的 litellm_params 里按名字读字符串。缺键或类型不对时返回 fallback。
// 参数 e：一条模型部署。key：参数名，例如 model 或 api_base。fallback：缺值或类型不对时的结果。
// 返回：读到的字符串，或 fallback。
// 调用：router.DeploymentID、数据面选上游地址。测试：bypass_logic_test.go 间接使用。
func (e ModelEntry) ParamString(key, fallback string) string {
	if e.LiteLLMParams == nil {
		return fallback
	}
	v, ok := e.LiteLLMParams[key]
	if !ok {
		return fallback
	}
	s, _ := v.(string)
	if s == "" {
		return fallback
	}
	return s
}

// SplitProviderModel splits provider/model. With no slash the provider is empty and the model name is the whole string.
// 参数 raw（string）：拆分供应商模型使用的原始内容。空串表示调用方没有提供这项。
// 返回 provider（string）：供应商标识，例如 openai 或 volcengine；model（string）：发给上游或对外展示的模型名。
// 调用：dataplane/serve.go
// 测试：builtin_providers_test.go
func SplitProviderModel(raw string) (provider, model string) {
	raw = strings.TrimSpace(raw)
	if i := strings.Index(raw, "/"); i > 0 {
		return raw[:i], raw[i+1:]
	}
	return "openai", raw
}
