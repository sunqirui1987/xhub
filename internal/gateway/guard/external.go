package guard

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"net/url"
	"strings"
	"time"
	"unicode"
)

// externalProviders 只列实际具有 Go 适配器的服务；花园中其他伙伴不能据目录存在宣称已接入。
var externalProviders = map[string]string{
	"openai_moderation": "OpenAI Moderation", "lakera_v2": "Lakera Guard",
	"azure/text_moderations": "Azure Content Safety", "azure/prompt_shield": "Azure Prompt Shield",
	"bedrock": "AWS Bedrock", "presidio": "Presidio PII", "litellm_proxy": "Remote LiteLLM",
	"hide-secrets": "Secret detection",
}

// validateExternal 按提供商协议验证必填字段、基础 URL、认证组合及支持的操作。
// 参数：p：litellm_params 配置，guardrail 决定服务类型。
// 返回：error：无效配置原因；nil 表示可以进入执行阶段，不代表远端凭据已验证。
// 调用：Validate、runExternal。
// 测试：external_test.go。
func validateExternal(p map[string]any) error {
	kind := str(p["guardrail"])
	required := []string{}
	switch kind {
	case "openai_moderation", "lakera_v2":
		required = []string{"api_key"}
	case "azure/text_moderations", "azure/prompt_shield":
		required = []string{"api_base", "api_key"}
	case "presidio":
		required = []string{"api_base"}
		if str(p["action"]) == "redact" {
			required = append(required, "anonymizer_api_base")
		}
	case "litellm_proxy":
		required = []string{"api_base", "api_key", "remote_guardrail_name"}
	case "bedrock":
		required = []string{"guardrailIdentifier", "guardrailVersion", "aws_region_name"}
	case "hide-secrets":
	default:
		return fmt.Errorf("provider is not implemented")
	}
	for _, key := range required {
		if strings.TrimSpace(str(p[key])) == "" {
			return fmt.Errorf("%s is required", key)
		}
	}
	for _, key := range []string{"api_base", "anonymizer_api_base"} {
		if raw := str(p[key]); raw != "" {
			u, e := url.Parse(raw)
			if e != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
				return fmt.Errorf("%s must be an HTTP(S) base URL without credentials or query", key)
			}
		}
	}
	if action := str(p["action"]); action == "redact" && kind != "presidio" && kind != "hide-secrets" {
		return fmt.Errorf("this provider supports blocking only")
	}
	if value := p["severity_threshold"]; value != nil {
		n := priority(value)
		if n < 0 || n > 7 || n != float64(int(n)) {
			return fmt.Errorf("severity_threshold must be an integer from 0 to 7")
		}
		switch value.(type) {
		case int, float64:
		default:
			return fmt.Errorf("severity_threshold must be numeric")
		}
	}
	if kind == "bedrock" && ((str(p["aws_access_key_id"]) == "") != (str(p["aws_secret_access_key"]) == "")) {
		return fmt.Errorf("AWS access key and secret must be supplied together")
	}
	return nil
}

// externalRequest 发送外部护栏 POST 请求并要求成功响应为 JSON 对象。
// 参数：ctx：整个规则的取消上下文；endpoint：完整地址；headers：认证头；body：JSON 请求体。
// 返回：响应对象及错误；HTTP 失败、超限或响应形状错误都返回错误。
// 调用：OpenAI、Lakera、Azure、远端 LiteLLM、Presidio Anonymizer。
// 测试：external_test.go。
func externalRequest(ctx context.Context, endpoint string, headers map[string]string, body any) (map[string]any, error) {
	result := httpPrimitive(ctx, endpoint, "POST", headers, body, 10)
	if !boolOf(result["success"]) {
		return nil, fmt.Errorf("external guardrail: %s", str(result["error"]))
	}
	value, ok := result["body"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("external guardrail returned an invalid JSON object")
	}
	return value, nil
}

// flagDecision 严格读取外部服务的布尔判定，避免缺失字段被当作未命中。
// 参数：response：远端 JSON 对象；key：flagged 或 attackDetected 等字段名。
// 返回：判定值及错误；字符串、null 或缺失字段均报错。
// 调用：runExternal、runAzure。
// 测试：external_test.go 的畸形响应测试。
func flagDecision(response map[string]any, key string) (bool, error) {
	flagged, ok := response[key].(bool)
	if !ok {
		return false, fmt.Errorf("external response is missing boolean %s", key)
	}
	return flagged, nil
}

// runExternal 将统一文本输入转为各提供商协议并执行请求前检查；任何协议或网络错误默认阻止请求。
// 参数：p：已配置参数，运行时再次验证；texts：有序文本，最多 1 MiB/10000 段。
// 返回：action：allow/block/modify/redact；reason：不含原文的命中说明；out：完整位置对应文本；err：执行失败。
// 调用：runRuleDetailed；hide-secrets 由本地 runSecrets 执行。
// 测试：external_test.go：协议、鉴权、Bedrock 签名、畸形响应及实际规则链。
func runExternal(p map[string]any, texts []string) (action, reason string, out []string, err error) {
	if err = validateExternal(p); err != nil {
		return "block", "", nil, err
	}
	out = append([]string{}, texts...)
	action = "allow"
	if len(texts) == 0 || strings.TrimSpace(strings.Join(texts, "")) == "" {
		return
	}
	size := 0
	for _, text := range texts {
		size += len(text)
	}
	if size > maxCustomText || len(texts) > 10000 {
		return "block", "", nil, fmt.Errorf("external guardrail input too large")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	kind := str(p["guardrail"])
	base := strings.TrimRight(str(p["api_base"]), "/")
	key := str(p["api_key"])
	headers := map[string]string{"Authorization": "Bearer " + key}
	block := func() (string, string, []string, error) {
		return "block", externalProviders[kind] + " detected unsafe content", out, nil
	}
	switch kind {
	case "bedrock":
		opts := []func(*awsconfig.LoadOptions) error{awsconfig.WithRegion(str(p["aws_region_name"])), awsconfig.WithRetryMaxAttempts(1)}
		if str(p["aws_access_key_id"]) != "" {
			opts = append(opts, awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(credential(str(p["aws_access_key_id"])), credential(str(p["aws_secret_access_key"])), credential(str(p["aws_session_token"])))))
		}
		cfg, e := awsconfig.LoadDefaultConfig(ctx, opts...)
		if e != nil {
			return "block", "", nil, fmt.Errorf("cannot load AWS guardrail credentials")
		}
		client := bedrockruntime.NewFromConfig(cfg, func(o *bedrockruntime.Options) {
			if base != "" {
				o.BaseEndpoint = aws.String(base)
			}
		})
		content := make([]types.GuardrailContentBlock, len(texts))
		for i, text := range texts {
			content[i] = &types.GuardrailContentBlockMemberText{Value: types.GuardrailTextBlock{Text: aws.String(text)}}
		}
		result, e := client.ApplyGuardrail(ctx, &bedrockruntime.ApplyGuardrailInput{GuardrailIdentifier: aws.String(str(p["guardrailIdentifier"])), GuardrailVersion: aws.String(str(p["guardrailVersion"])), Source: types.GuardrailContentSourceInput, Content: content})
		if e != nil {
			return "block", "", nil, fmt.Errorf("Bedrock ApplyGuardrail failed")
		}
		switch result.Action {
		case types.GuardrailActionNone:
			return
		case types.GuardrailActionGuardrailIntervened:
			return block()
		default:
			return "block", "", nil, fmt.Errorf("invalid Bedrock action")
		}
	case "openai_moderation":
		if base == "" {
			base = "https://api.openai.com/v1"
		}
		model := str(p["model"])
		if model == "" {
			model = "omni-moderation-latest"
		}
		result, e := externalRequest(ctx, base+"/moderations", headers, map[string]any{"model": model, "input": texts})
		if e != nil {
			return "block", "", nil, e
		}
		rows, ok := result["results"].([]any)
		if !ok || len(rows) != len(texts) {
			return "block", "", nil, fmt.Errorf("invalid moderation results")
		}
		for _, row := range rows {
			value, _ := row.(map[string]any)
			flagged, e := flagDecision(value, "flagged")
			if e != nil {
				return "block", "", nil, e
			}
			if flagged {
				return block()
			}
		}
		return
	case "lakera_v2":
		if base == "" {
			base = "https://api.lakera.ai"
		}
		messages := []map[string]any{}
		for _, text := range texts {
			messages = append(messages, map[string]any{"role": "user", "content": text})
		}
		body := map[string]any{"messages": messages}
		if v := str(p["project_id"]); v != "" {
			body["project_id"] = v
		}
		result, e := externalRequest(ctx, base+"/v2/guard", headers, body)
		if e != nil {
			return "block", "", nil, e
		}
		flagged, e := flagDecision(result, "flagged")
		if e != nil {
			return "block", "", nil, e
		}
		if flagged {
			return block()
		}
		return
	case "azure/text_moderations", "azure/prompt_shield":
		return runAzure(ctx, p, texts)
	case "litellm_proxy":
		for i, text := range texts {
			result, e := externalRequest(ctx, base+"/guardrails/apply_guardrail", headers, map[string]any{"guardrail_name": str(p["remote_guardrail_name"]), "text": text, "input_type": "request"})
			if e != nil {
				return "block", "", nil, e
			}
			if boolOf(result["blocked"]) || str(result["action"]) == "block" {
				return block()
			}
			value, ok := result["response_text"].(string)
			if !ok {
				return "block", "", nil, fmt.Errorf("remote LiteLLM response is missing response_text")
			}
			if value != text {
				action = "modify"
				out[i] = value
			}
		}
		return
	case "presidio":
		return runPresidio(ctx, p, texts)
	}
	return "block", "", nil, fmt.Errorf("unsupported external provider")
}

// runPresidio 调用 Analyzer，校验 Unicode 字符区间；命中后按配置拦截或调用 Anonymizer 回写文本。
// 参数：ctx：共享总时限；p：Analyzer/Anonymizer 地址、language、entities、action；texts：输入文本。
// 返回：动作、原因、位置对应的文本及错误；非法实体区间或匿名化响应按失败处理。
// 调用：runExternal 的 presidio 分支。
// 测试：TestPresidioRedactionAndUnicode。
func runPresidio(ctx context.Context, p map[string]any, texts []string) (string, string, []string, error) {
	action := "allow"
	out := append([]string{}, texts...)
	language := str(p["language"])
	if language == "" {
		language = "en"
	}
	for i, text := range texts {
		payload := map[string]any{"text": text, "language": language}
		if entities := extraWords(p["entities"]); len(entities) > 0 {
			payload["entities"] = entities
		}
		response := httpPrimitive(ctx, strings.TrimRight(str(p["api_base"]), "/")+"/analyze", "POST", nil, payload, 10)
		if !boolOf(response["success"]) {
			return "block", "", nil, fmt.Errorf("Presidio analyze failed")
		}
		findings, ok := response["body"].([]any)
		if !ok {
			return "block", "", nil, fmt.Errorf("invalid Presidio findings")
		}
		if len(findings) == 0 {
			continue
		}
		for _, finding := range findings {
			value, _ := finding.(map[string]any)
			start, sok := value["start"].(float64)
			end, eok := value["end"].(float64)
			if !sok || !eok || start < 0 || end <= start || end > float64(len([]rune(text))) || start != float64(int(start)) || end != float64(int(end)) || str(value["entity_type"]) == "" {
				return "block", "", nil, fmt.Errorf("invalid Presidio entity span")
			}
		}
		if str(p["action"]) != "redact" {
			return "block", "Presidio detected personal information", out, nil
		}
		result, e := externalRequest(ctx, strings.TrimRight(str(p["anonymizer_api_base"]), "/")+"/anonymize", nil, map[string]any{"text": text, "analyzer_results": findings})
		if e != nil {
			return "block", "", nil, e
		}
		value, ok := result["text"].(string)
		if !ok {
			return "block", "", nil, fmt.Errorf("invalid Presidio anonymized text")
		}
		out[i] = value
		action = "redact"
	}
	return action, "", out, nil
}

// runAzure 按 Azure 10000 字符上限分段执行内容安全或提示词攻击检查，任一片段命中即停止。
// 参数：ctx：共享 10 秒时限；p：Azure 地址、密钥、版本和阈值；texts：文本段。
// 返回：动作、命中说明、原始文本及错误；全部片段通过才返回 allow。
// 调用：runExternal 的 Azure 分支。
// 测试：external_test.go：Azure 认证、分段、类别完整性及阈值。
func runAzure(ctx context.Context, p map[string]any, texts []string) (string, string, []string, error) {
	kind := str(p["guardrail"])
	base := strings.TrimRight(str(p["api_base"]), "/")
	key := str(p["api_key"])
	for _, chunk := range azureTextChunks(strings.Join(texts, "\n")) {
		var headers map[string]string
		headers = map[string]string{"Ocp-Apim-Subscription-Key": key}
		version := str(p["api_version"])
		if version == "" {
			version = "2024-09-01"
		}
		endpoint := base + "/contentsafety/text:analyze?api-version=" + url.QueryEscape(version)
		body := map[string]any{"text": chunk, "outputType": "EightSeverityLevels"}
		if kind == "azure/prompt_shield" {
			endpoint = base + "/contentsafety/text:shieldPrompt?api-version=" + url.QueryEscape(version)
			body = map[string]any{"userPrompt": chunk, "documents": []string{}}
		}
		result, e := externalRequest(ctx, endpoint, headers, body)
		if e != nil {
			return "block", "", nil, e
		}
		if kind == "azure/prompt_shield" {
			value, _ := result["userPromptAnalysis"].(map[string]any)
			flagged, e := flagDecision(value, "attackDetected")
			if e != nil {
				return "block", "", nil, e
			}
			if flagged {
				return "block", externalProviders[kind] + " detected unsafe content", texts, nil
			}
			continue
		}
		rows, ok := result["categoriesAnalysis"].([]any)
		if !ok || len(rows) == 0 {
			return "block", "", nil, fmt.Errorf("invalid Azure categoriesAnalysis")
		}
		threshold := 4.0
		if p["severity_threshold"] != nil {
			threshold = priority(p["severity_threshold"])
		}
		seen := map[string]bool{}
		for _, row := range rows {
			value, _ := row.(map[string]any)
			category := str(value["category"])
			switch category {
			case "Hate", "SelfHarm", "Sexual", "Violence":
			default:
				return "block", "", nil, fmt.Errorf("invalid Azure category")
			}
			severity, ok := value["severity"].(float64)
			if !ok || severity < 0 || severity > 7 || severity != float64(int(severity)) || seen[category] {
				return "block", "", nil, fmt.Errorf("invalid Azure severity")
			}
			seen[category] = true
			if severity >= threshold {
				return "block", externalProviders[kind] + " detected unsafe content", texts, nil
			}
		}
		if len(seen) != 4 {
			return "block", "", nil, fmt.Errorf("incomplete Azure categories")
		}

	}
	return "allow", "", texts, nil
}

// azureTextChunks 按 Unicode 字符分段，优先保留词边界并精确保留原始空白和文本。
// 参数：text：待送往 Azure 的文本。
// 返回：至少一个片段；每段最多 10000 个 rune；无空白的超长词按字符强制切分。
// 调用：runAzure。
// 测试：TestAzureUnicodeChunksAndLaterViolation。
func azureTextChunks(text string) []string {
	const limit = 10000
	runes := []rune(text)
	chunks := []string{}
	for len(runes) > limit {
		end := limit
		for i := limit - 1; i >= 0; i-- {
			if unicode.IsSpace(runes[i]) {
				end = i + 1
				break
			}
		}
		chunks = append(chunks, string(runes[:end]))
		runes = runes[end:]
	}
	if len(runes) > 0 || len(chunks) == 0 {
		chunks = append(chunks, string(runes))
	}
	return chunks
}
