// Package config loads process configuration. It accepts PostgreSQL only and keeps YAML keys the typed structs do not declare.
package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
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

// RouterSettings holds the routing strategy, retry count, and timeout. Other router keys from YAML stay in Config.RouterRaw.
type RouterSettings struct {
	RoutingStrategy string  `yaml:"routing_strategy"`
	NumRetries      int     `yaml:"num_retries"`
	Timeout         float64 `yaml:"timeout"`
}

// GeneralSettings holds the master key, database, Redis, and a few switches. Other general-settings keys stay in Config.GeneralRaw.
type GeneralSettings struct {
	MasterKey                 string `yaml:"master_key"`
	DatabaseURL               string `yaml:"database_url"`
	RedisURL                  string `yaml:"redis_url"`
	StoreModelInDB            bool   `yaml:"store_model_in_db"`
	AllowMasterKeyLLM         bool   `yaml:"allow_master_key_llm"`
	DisableEnvCredentialLogin bool   `yaml:"disable_env_credential_login"`
}

// Load reads YAML. An empty database_url, or one that starts with sqlite or file:, returns an error instead of silently using a local file database.
func Load(path string) (*Config, error) {
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
	low := strings.ToLower(c.GeneralSettings.DatabaseURL)
	if strings.HasPrefix(low, "sqlite:") || strings.HasPrefix(low, "file:") {
		return nil, fmt.Errorf("sqlite is not supported; set general_settings.database_url to a postgres:// URL")
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
	if c.GeneralSettings.MasterKey == "" {
		return nil, fmt.Errorf("general_settings.master_key is required")
	}
	return &c, nil
}

// resolve expands environment placeholders in a config string. A missing variable keeps the original text instead of becoming empty.
func resolve(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "os.environ/") {
		return os.Getenv(strings.TrimPrefix(s, "os.environ/"))
	}
	return s
}

// ParamString reads a string from LiteLLMParams. A missing or wrong-typed value returns fallback.
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
func SplitProviderModel(raw string) (provider, model string) {
	raw = strings.TrimSpace(raw)
	if i := strings.Index(raw, "/"); i > 0 {
		return raw[:i], raw[i+1:]
	}
	return "openai", raw
}
