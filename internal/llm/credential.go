// Package llm fills deployment parameters from credential_values in the secret store. A missing field keeps its previous value.
package llm

import (
	"github.com/sunqirui1987/xhub/internal/logx"
	"os"
	"strings"
	"sync"
)

// credentialFields lists the fields a named credential may copy into a deployment.
//
// The source is LiteLLM CredentialLiteLLMParams plus custom_llm_provider.
// Only keys in this list are copied. Extra fields on a credential must not leak into a model call,
// or the upstream would receive a parameter it does not understand.
var credentialFields = []string{
	"api_key",
	"api_base",
	"api_version",
	"azure_ad_token",
	"tenant_id",
	"client_id",
	"client_secret",
	"azure_scope",
	"azure_username",
	"azure_password",
	"vertex_project",
	"vertex_location",
	"vertex_credentials",
	"region_name",
	"gcs_bucket_name",
	"aws_access_key_id",
	"aws_secret_access_key",
	"aws_session_token",
	"aws_region_name",
	"aws_session_name",
	"aws_profile_name",
	"aws_role_name",
	"aws_web_identity_token",
	"aws_sts_endpoint",
	"aws_external_id",
	"aws_session_tags",
	"aws_bedrock_runtime_endpoint",
	"aws_bedrock_project_id",
	"s3_bucket_name",
	"s3_region_name",
	"s3_encryption_key_id",
	"aws_batch_role_arn",
	"s3_output_bucket_name",
	"bedrock_tags",
	"watsonx_region_name",
	"custom_llm_provider",
}

var logTraceOnceCredential sync.Once

// Hydrate fills deployment parameters that are still empty from a named credential.
//
// LiteLLM load_credentials_from_list writes a value only when the key is entirely absent.
// Deployments in this gateway often store api_key as an empty string after a redacted response is read back,
// so an empty string also counts as unset.
//
// api_key is the exception. The dashboard masks a stored key as ***** and the update handler
// keeps the previous value, so a model can keep a stale key after the user points it at a credential.
// A non-blank api_key on the credential replaces that stale key. Every other credential field
// still fills only a blank deployment field, and a non-empty value there still wins.
//
// os.environ/NAME inside a string is expanded on this call, not when the credential is saved.
// The same credential can follow the process environment. custom_llm_provider is then lowercased,
// so OpenAI in config and openai in the protocol name the same provider.
//
// Hydrate returns a new map and does not modify the deployment parameters the caller passed in.
func Hydrate(params, credentialValues map[string]any) map[string]any {
	logTraceOnceCredential.Do(func() { logx.Trace("enter llm.Hydrate") })

	out := map[string]any{}
	for key, value := range params {
		out[key] = value
	}
	if len(credentialValues) > 0 {
		if text, ok := credentialValues["api_key"].(string); ok && strings.TrimSpace(text) != "" {
			delete(out, "api_key")
		}
		for _, key := range credentialFields {
			if !blank(out[key]) {
				continue
			}
			value, ok := credentialValues[key]
			if !ok || blank(value) {
				continue
			}
			out[key] = value
		}
	}
	for key, value := range out {
		text, ok := value.(string)
		if !ok {
			continue
		}
		expanded := expandEnv(text)
		if key == "custom_llm_provider" {
			expanded = strings.ToLower(expanded)
		}
		out[key] = expanded
	}
	return out
}

// expandEnv recognizes os.environ/NAME in a config string.
// A string without that prefix is returned unchanged. A missing variable becomes an empty string, matching os.Getenv.
func expandEnv(s string) string {
	s = strings.TrimSpace(s)
	const prefix = "os.environ/"
	if strings.HasPrefix(s, prefix) {
		return os.Getenv(strings.TrimPrefix(s, prefix))
	}
	return s
}

// blank reports whether a credential value is empty. A blank field does not overwrite a value already set on the deployment.
func blank(v any) bool {
	if v == nil {
		return true
	}
	text, ok := v.(string)
	return ok && strings.TrimSpace(text) == ""
}
