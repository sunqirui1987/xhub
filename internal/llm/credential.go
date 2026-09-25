// 把密钥库里的 credential_values 填进部署参数。缺的字段保持原值。
package llm

import (
	"os"
	"strings"
)

// credentialFields 是命名凭证可以填进部署的字段。
//
// 来源是 LiteLLM 的 CredentialLiteLLMParams，再加上 custom_llm_provider。
// 只复制这张表里的键。凭证里如果多带了别的字段，不能漏进一次模型调用，
// 否则上游会收到它不认识的参数。
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

// Hydrate 用命名凭证补齐部署里还空着的参数。
//
// LiteLLM 的 load_credentials_from_list 只在「键完全不存在」时写入。
// 网关里部署经常把 api_key 存成空字符串（响应脱敏之后再读回来就是这样），
// 所以空字符串也当成没填。部署上已经写了的非空值永远优先，凭证不能覆盖它。
//
// 字符串里的 os.environ/NAME 在这次调用时展开，不在保存凭证时展开。
// 这样同一条凭证可以跟着进程环境走。custom_llm_provider 再转成小写，
// 配置里的 OpenAI 和协议里的 openai 是同一个供应商。
//
// 返回新的 map，不改调用方传入的部署参数。
func Hydrate(params, credentialValues map[string]any) map[string]any {
	out := map[string]any{}
	for key, value := range params {
		out[key] = value
	}
	if len(credentialValues) > 0 {
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

// expandEnv 识别配置里的 os.environ/NAME。
// 没有这个前缀的字符串原样返回。变量不存在时得到空字符串，和 os.Getenv 一致。
func expandEnv(s string) string {
	s = strings.TrimSpace(s)
	const prefix = "os.environ/"
	if strings.HasPrefix(s, prefix) {
		return os.Getenv(strings.TrimPrefix(s, prefix))
	}
	return s
}

// 判断凭证值是否空白。空白字段不覆盖部署上已经写好的值。
func blank(v any) bool {
	if v == nil {
		return true
	}
	text, ok := v.(string)
	return ok && strings.TrimSpace(text) == ""
}
