package providerconfig

import (
	"strings"
	"testing"
)

const validConfig = `version: 1
providers:
  - id: ALPHA
    enabled: true
    credential_name: alpha
    key_env: XHUB_ALPHA_KEY
    base: https://alpha.example/v1
    protocol: openai
    models: [shared-model, alpha-only]
  - id: BETA
    enabled: true
    credential_name: beta
    key_env: XHUB_BETA_KEY
    base: http://beta.example/api
    protocol: openai
    models: [shared-model]
weighted_scenarios:
  - name: shared-model-split
    model_name: public-shared
    requests: 10
    deployments:
      - id: alpha-a
        provider: ALPHA
        model: shared-model
        weight: 3
      - id: beta-a
        provider: BETA
        model: shared-model
        weight: 7
`

func TestLoadAcceptsSharedModelsAndDistinctDeploymentConnections(t *testing.T) {
	config := strings.Replace(validConfig, `  - id: BETA
    enabled: true
    credential_name: beta
    key_env: XHUB_BETA_KEY
    base: http://beta.example/api
    protocol: openai
    models: [shared-model]
`, "", 1)
	config = strings.Replace(config, "id: beta-a\n        provider: BETA", "id: alpha-b\n        provider: ALPHA", 1)

	cfg, err := Load(strings.NewReader(config))
	if err != nil {
		t.Fatalf("Load() rejected duplicate model connections with distinct IDs: %v", err)
	}
	if got := len(cfg.WeightedScenarios[0].Deployments); got != 2 {
		t.Fatalf("deployment count = %d, want 2", got)
	}

	if _, err := Load(strings.NewReader(validConfig)); err != nil {
		t.Fatalf("Load() rejected the same model name across providers: %v", err)
	}
}

func TestLoadRejectsMalformedConfigurations(t *testing.T) {
	tests := []struct {
		name    string
		old     string
		new     string
		wantErr string
	}{
		{"wrong version", "version: 1", "version: 2", "version must be 1"},
		{"provider ID format", "id: ALPHA", "id: alpha", "uppercase environment-variable stem"},
		{"duplicate provider", "id: BETA", "id: ALPHA", "providers[1].id must be unique"},
		{"bad key env", "key_env: XHUB_ALPHA_KEY", "key_env: 1INVALID", "valid environment-variable name"},
		{"non-http URL", "https://alpha.example/v1", "ftp://alpha.example/v1", "HTTP or HTTPS URL"},
		{"URL credentials", "https://alpha.example/v1", "https://user:pass@alpha.example/v1", "without credentials"},
		{"unknown protocol", "protocol: openai", "protocol: gemini", "protocol must be openai or anthropic"},
		{"empty models", "models: [shared-model, alpha-only]", "models: []", "models must be nonempty"},
		{"duplicate scenario", "  - name: shared-model-split", "  - name: shared-model-split\n    model_name: another\n    requests: 2\n    deployments:\n      - {id: x, provider: ALPHA, model: shared-model, weight: 1}\n      - {id: y, provider: BETA, model: shared-model, weight: 1}\n  - name: shared-model-split", "name must be unique"},
		{"requests below range", "requests: 10", "requests: 0", "between 1 and 100"},
		{"requests above range", "requests: 10", "requests: 101", "between 1 and 100"},
		{"too few deployments", "      - id: beta-a\n        provider: BETA\n        model: shared-model\n        weight: 7\n", "", "at least two entries"},
		{"duplicate deployment ID", "id: beta-a", "id: alpha-a", "id must be unique"},
		{"unknown provider", "provider: BETA", "provider: MISSING", "configured provider"},
		{"model outside provider", "model: shared-model\n        weight: 3", "model: beta-only\n        weight: 3", "model must belong"},
		{"anthropic scenario", "protocol: openai\n    models: [shared-model]", "protocol: anthropic\n    models: [shared-model]", "openai protocol"},
		{"negative weight", "weight: 3", "weight: -1", "weight must be nonnegative"},
		{"zero total", "weight: 3\n      - id: beta-a\n        provider: BETA\n        model: shared-model\n        weight: 7", "weight: 0\n      - id: beta-a\n        provider: BETA\n        model: shared-model\n        weight: 0", "positive total"},
		{"partial reduced cycle", "requests: 10", "requests: 9", "whole reduced-weight cycles"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := strings.Replace(validConfig, test.old, test.new, 1)
			if input == validConfig {
				t.Fatal("test replacement did not change input")
			}
			_, err := Load(strings.NewReader(input))
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("Load() error = %v, want substring %q", err, test.wantErr)
			}
		})
	}
}

func TestLoadRejectsUnknownFieldsAndAdditionalDocumentsWithoutEchoingInput(t *testing.T) {
	const secret = "accidentally-pasted-secret"
	tests := []struct {
		name  string
		input string
	}{
		{"unknown field", validConfig + "unknown_field: " + secret + "\n"},
		{"invalid YAML", validConfig + "[" + secret + "\n"},
		{"second document", validConfig + "---\nsecret: " + secret + "\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Load(strings.NewReader(test.input))
			if err == nil {
				t.Fatal("Load() succeeded")
			}
			if strings.Contains(err.Error(), secret) {
				t.Fatalf("Load() echoed input in error: %v", err)
			}
		})
	}
}

func TestLoadRejectsIntegerOverflowWithoutEchoingInput(t *testing.T) {
	const oversized = "999999999999999999999999999999999999999999999999"
	input := strings.Replace(validConfig, "weight: 3", "weight: "+oversized, 1)
	_, err := Load(strings.NewReader(input))
	if err == nil {
		t.Fatal("Load() accepted an overflowing integer")
	}
	if strings.Contains(err.Error(), oversized) {
		t.Fatalf("Load() echoed overflowing input in error: %v", err)
	}
}

func TestValidateDecodedConfig(t *testing.T) {
	cfg, err := Load(strings.NewReader(validConfig))
	if err != nil {
		t.Fatalf("Load(valid config): %v", err)
	}
	if err := Validate(cfg); err != nil {
		t.Fatalf("Validate(valid config): %v", err)
	}

	cfg.WeightedScenarios[0].Deployments[0].Weight = 0
	cfg.WeightedScenarios[0].Deployments[1].Weight = 0
	if err := Validate(cfg); err == nil || !strings.Contains(err.Error(), "positive total") {
		t.Fatalf("Validate(all-zero weights) = %v, want positive-total error", err)
	}
}
