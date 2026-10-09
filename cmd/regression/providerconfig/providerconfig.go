// Package providerconfig parses and validates the non-secret provider metadata
// used by live E2E runs and deterministic regression tests.
package providerconfig

import (
	"errors"
	"fmt"
	"io"
	"math"
	"net/url"
	"regexp"
	"strings"

	"github.com/sunqirui1987/xhub/internal/logx"
	"gopkg.in/yaml.v3"
)

var (
	envStemPattern    = regexp.MustCompile("^[A-Z][A-Z0-9_]*$")
	envNamePattern    = regexp.MustCompile("^[A-Za-z_][A-Za-z0-9_]*$")
	simpleIDPattern   = regexp.MustCompile("^[A-Za-z0-9][A-Za-z0-9_.-]*$")
	decodeError       = errors.New("invalid provider configuration YAML or schema")
	multipleDocsError = errors.New("provider configuration must contain exactly one YAML document")
)

// Config is the complete provider metadata document.
type Config struct {
	Version           int        `yaml:"version" json:"version"`
	Providers         []Provider `yaml:"providers" json:"providers"`
	WeightedScenarios []Scenario `yaml:"weighted_scenarios" json:"weighted_scenarios"`
}

// Provider describes one upstream without containing its credential value.
type Provider struct {
	ID             string   `yaml:"id" json:"id"`
	Enabled        bool     `yaml:"enabled" json:"enabled"`
	CredentialName string   `yaml:"credential_name" json:"credential_name"`
	KeyEnv         string   `yaml:"key_env" json:"key_env"`
	Base           string   `yaml:"base" json:"base"`
	Protocol       string   `yaml:"protocol" json:"protocol"`
	Models         []string `yaml:"models" json:"models"`
}

// Scenario describes a deterministic weighted-routing exercise.
type Scenario struct {
	Name        string       `yaml:"name" json:"name"`
	ModelName   string       `yaml:"model_name" json:"model_name"`
	Requests    int          `yaml:"requests" json:"requests"`
	Deployments []Deployment `yaml:"deployments" json:"deployments"`
}

// Deployment is one weighted upstream connection in a scenario.
type Deployment struct {
	ID       string `yaml:"id" json:"id"`
	Provider string `yaml:"provider" json:"provider"`
	Model    string `yaml:"model" json:"model"`
	Weight   int    `yaml:"weight" json:"weight"`
}

// Load decodes one strict YAML document and validates all cross-references.
// Parse failures are deliberately sanitized so malformed input values, which
// could include accidentally pasted credentials, are never repeated in errors.
// 参数 r（io.Reader）：供应商元数据 YAML；不得包含密钥值。
// 返回 Config 和 error：配置合法时返回完整元数据，否则返回不回显输入的错误。
// 调用：真实供应商测试配置加载入口。
// 测试：providerconfig_test.go。
func Load(r io.Reader) (Config, error) {
	var cfg Config
	if r == nil {
		return Config{}, decodeError
	}

	decoder := yaml.NewDecoder(r)
	decoder.KnownFields(true)
	if err := decoder.Decode(&cfg); err != nil {
		return Config{}, decodeError
	}

	var extra yaml.Node
	if err := decoder.Decode(&extra); err != io.EOF {
		if err != nil {
			return Config{}, decodeError
		}
		return Config{}, multipleDocsError
	}

	if err := Validate(cfg); err != nil {
		return Config{}, err
	}
	logx.Debug("provider configuration validated providers=%d weighted_scenarios=%d", len(cfg.Providers), len(cfg.WeightedScenarios))
	return cfg, nil
}

// Validate checks identifiers, URLs, provider references, and complete weight cycles.
// 参数 cfg（Config）：已解析的供应商与权重场景。
// 返回 error：首个不满足约束的字段错误；合法时为 nil。
// 调用：Load，以及从 JSON 环境元数据解码的真实供应商测试。
// 测试：providerconfig_test.go。
func Validate(cfg Config) error {
	if cfg.Version != 1 {
		return errors.New("version must be 1")
	}

	providers := make(map[string]Provider, len(cfg.Providers))
	models := make(map[string]map[string]struct{}, len(cfg.Providers))
	for i, provider := range cfg.Providers {
		path := fmt.Sprintf("providers[%d]", i)
		if !envStemPattern.MatchString(provider.ID) {
			return fmt.Errorf("%s.id must be an uppercase environment-variable stem", path)
		}
		if _, exists := providers[provider.ID]; exists {
			return fmt.Errorf("%s.id must be unique", path)
		}
		if strings.TrimSpace(provider.CredentialName) == "" {
			return fmt.Errorf("%s.credential_name must be nonempty", path)
		}
		if !envNamePattern.MatchString(provider.KeyEnv) {
			return fmt.Errorf("%s.key_env must be a valid environment-variable name", path)
		}
		if !validHTTPURL(provider.Base) {
			return fmt.Errorf("%s.base must be an HTTP or HTTPS URL without credentials, query, or fragment", path)
		}
		if provider.Protocol != "openai" && provider.Protocol != "anthropic" {
			return fmt.Errorf("%s.protocol must be openai or anthropic", path)
		}
		if len(provider.Models) == 0 {
			return fmt.Errorf("%s.models must be nonempty", path)
		}

		providerModels := make(map[string]struct{}, len(provider.Models))
		for j, model := range provider.Models {
			if strings.TrimSpace(model) == "" || model != strings.TrimSpace(model) {
				return fmt.Errorf("%s.models[%d] must be a nonempty trimmed model name", path, j)
			}
			providerModels[model] = struct{}{}
		}
		providers[provider.ID] = provider
		models[provider.ID] = providerModels
	}

	scenarioNames := make(map[string]struct{}, len(cfg.WeightedScenarios))
	deploymentIDs := make(map[string]struct{})
	for i, scenario := range cfg.WeightedScenarios {
		path := fmt.Sprintf("weighted_scenarios[%d]", i)
		if !simpleIDPattern.MatchString(scenario.Name) {
			return fmt.Errorf("%s.name must be a simple identifier", path)
		}
		if _, exists := scenarioNames[scenario.Name]; exists {
			return fmt.Errorf("%s.name must be unique", path)
		}
		scenarioNames[scenario.Name] = struct{}{}
		if strings.TrimSpace(scenario.ModelName) == "" || scenario.ModelName != strings.TrimSpace(scenario.ModelName) {
			return fmt.Errorf("%s.model_name must be nonempty and trimmed", path)
		}
		if scenario.Requests < 1 || scenario.Requests > 100 {
			return fmt.Errorf("%s.requests must be between 1 and 100", path)
		}
		if len(scenario.Deployments) < 2 {
			return fmt.Errorf("%s.deployments must contain at least two entries", path)
		}

		var divisor uint64
		for j, deployment := range scenario.Deployments {
			deploymentPath := fmt.Sprintf("%s.deployments[%d]", path, j)
			if !simpleIDPattern.MatchString(deployment.ID) {
				return fmt.Errorf("%s.id must be a simple identifier", deploymentPath)
			}
			if _, exists := deploymentIDs[deployment.ID]; exists {
				return fmt.Errorf("%s.id must be unique", deploymentPath)
			}
			deploymentIDs[deployment.ID] = struct{}{}

			provider, exists := providers[deployment.Provider]
			if !exists {
				return fmt.Errorf("%s.provider must reference a configured provider", deploymentPath)
			}
			if _, exists := models[deployment.Provider][deployment.Model]; !exists {
				return fmt.Errorf("%s.model must belong to its provider", deploymentPath)
			}
			if provider.Protocol != "openai" {
				return fmt.Errorf("%s.provider must use the openai protocol", deploymentPath)
			}
			if deployment.Weight < 0 {
				return fmt.Errorf("%s.weight must be nonnegative", deploymentPath)
			}
			divisor = gcd(divisor, uint64(deployment.Weight))
		}
		if divisor == 0 {
			return fmt.Errorf("%s deployment weights must have a positive total", path)
		}

		var reducedCycle uint64
		for _, deployment := range scenario.Deployments {
			part := uint64(deployment.Weight) / divisor
			if math.MaxUint64-reducedCycle < part {
				return fmt.Errorf("%s reduced weight cycle is too large", path)
			}
			reducedCycle += part
		}
		if reducedCycle == 0 || reducedCycle > uint64(scenario.Requests) || uint64(scenario.Requests)%reducedCycle != 0 {
			return fmt.Errorf("%s.requests must span whole reduced-weight cycles", path)
		}
	}
	return nil
}

// validHTTPURL accepts a credential-free HTTP origin or base path.
// 参数 value（string）：供应商 API 根地址。
// 返回 bool：地址合法且没有凭据、查询或片段时为 true。
// 调用：validate。
// 测试：providerconfig_test.go。
func validHTTPURL(value string) bool {
	if value == "" || value != strings.TrimSpace(value) {
		return false
	}
	parsed, err := url.Parse(value)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.Hostname() == "" {
		return false
	}
	return parsed.User == nil && parsed.RawQuery == "" && parsed.Fragment == ""
}

// gcd finds the common divisor used to normalize a routing weight cycle.
// 参数 a、b（uint64）：两项非负权重。
// 返回 uint64：最大公约数。
// 调用：validate。
// 测试：providerconfig_test.go。
func gcd(a, b uint64) uint64 {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}
