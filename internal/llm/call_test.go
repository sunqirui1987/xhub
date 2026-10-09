package llm

import "testing"

func TestHydrateCredentialAPIKeyReplacesStaleDeploymentKey(t *testing.T) {
	got := Hydrate(map[string]any{
		"api_key":                 "qiniu-test-key",
		"api_base":                "https://api.qnaigc.com/v1",
		"litellm_credential_name": "qiniu",
	}, map[string]any{
		"api_key":  "sk-real",
		"api_base": "https://credential.example/v1",
	})
	if got["api_key"] != "sk-real" {
		t.Fatalf("api_key %v", got["api_key"])
	}
	if got["api_base"] != "https://api.qnaigc.com/v1" {
		t.Fatalf("api_base %v", got["api_base"])
	}
}

func TestHydrateKeepsDeploymentKeyWhenCredentialHasNone(t *testing.T) {
	got := Hydrate(map[string]any{
		"api_key":  "sk-model",
		"api_base": "",
	}, map[string]any{
		"api_key":  "   ",
		"api_base": "https://credential.example/v1",
	})
	if got["api_key"] != "sk-model" {
		t.Fatalf("api_key %v", got["api_key"])
	}
	if got["api_base"] != "https://credential.example/v1" {
		t.Fatalf("api_base %v", got["api_base"])
	}
}

