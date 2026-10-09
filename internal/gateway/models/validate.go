package models

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/sunqirui1987/xhub/internal/catalog"
	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/logx"
	"github.com/sunqirui1987/xhub/internal/provider"
)

var logTraceOnceValidate sync.Once

// validateDeployment uses base-unit USD prices, just like the price catalog.
// Existing inline connections remain editable; new UI deployments use credentials.
// 参数 s（Host）：读取凭据和目录的宿主；name（string）：公开部署名；params（map[string]any）：上游连接和人工费率；info（map[string]any）：计价来源和目录基准模型。
// 返回 error（error）：字段、价格或凭据不一致时返回可展示的校验错误。
// 调用：New、Update。
// 测试：admin_test.go。
func validateDeployment(s Host, name string, params, info map[string]any) error {
	logTraceOnceValidate.Do(func() { logx.Trace("enter models.validateDeployment") })
	if strings.TrimSpace(name) == "" || strings.TrimSpace(str(params["model"])) == "" {
		return fmt.Errorf("model_name and litellm_params.model are required")
	}
	if strings.HasPrefix(str(params["model"]), "auto_router/") || strings.HasPrefix(str(params["model"]), "adaptive_router/") {
		return fmt.Errorf("auto routers are no longer supported")
	}
	if err := provider.ValidateDeployment(config.ModelEntry{LiteLLMParams: params, ModelInfo: info}); err != nil {
		return err
	}
	for _, field := range rateFields {
		if value := params[field]; value != nil {
			if n, ok := numberField(value); !ok || n < 0 {
				return fmt.Errorf("%s must be a finite non-negative number", field)
			}
		}
	}
	if raw := params["rates"]; raw != nil {
		rates, ok := catalog.DecodeRates(raw)
		encoded, err := json.Marshal(raw)
		var entries []json.RawMessage
		if err != nil || json.Unmarshal(encoded, &entries) != nil || len(entries) != len(rates) {
			return fmt.Errorf("every rate must be valid")
		}
		if !ok {
			return fmt.Errorf("rates must contain valid prices")
		}
		for _, rate := range rates {
			if n, ok := numberField(rate.USD); !ok || n < 0 {
				return fmt.Errorf("rates must contain finite non-negative prices")
			}
			if rate.Measure != "token" && rate.Measure != "picture" && rate.Measure != "second" && rate.Measure != "query" {
				return fmt.Errorf("unsupported rate measure")
			}
			if rate.Window != "all" && rate.Window != "offpeak" && rate.Window != "peak" {
				return fmt.Errorf("unsupported rate window")
			}
			if rate.Side != "input" && rate.Side != "output" && rate.Side != "cache_read" && rate.Side != "cache_write" && rate.Side != "batch_input" && rate.Side != "batch_output" {
				return fmt.Errorf("unsupported rate side")
			}
		}
	}
	if source := str(info["pricing_source"]); source != "" && source != "catalog" && source != "manual" {
		return fmt.Errorf("unsupported pricing source")
	}
	if info["pricing_source"] == "catalog" && str(info["base_model"]) == "" {
		return fmt.Errorf("catalog pricing requires base_model")
	}
	if info["pricing_source"] == "manual" {
		hasPrice := false
		if _, ok := catalog.DecodeRates(params["rates"]); ok {
			hasPrice = true
		}
		for _, field := range rateFields {
			if _, ok := numberField(params[field]); ok {
				hasPrice = true
			}
		}
		if !hasPrice {
			return fmt.Errorf("manual pricing requires at least one price; use 0 for free")
		}
	}
	if id := str(info["base_model"]); id != "" {
		row, ok := catalog.ModelRow(id)
		if !ok {
			return fmt.Errorf("pricing model %s does not exist", id)
		}
		_, hasRates := catalog.DecodeRates(row["rates"])
		for _, field := range rateFields {
			if _, ok := numberField(row[field]); ok {
				hasRates = true
			}
		}
		if !hasRates {
			return fmt.Errorf("pricing model %s has no prices", id)
		}
	}
	if supplier := str(params["litellm_credential_name"]); supplier != "" {
		record, err := s.RecordStore().GetKV("credentials", supplier)
		if err != nil || record == nil {
			return fmt.Errorf("model provider %s is not configured", supplier)
		}
		if !credentialProtocolMatches(supplier, record, str(params["custom_llm_provider"])) {
			return fmt.Errorf("model protocol does not match its provider")
		}
	}
	return nil
}

// credentialProtocolMatches 校验部署协议与已存凭据的兼容性，供创建和更新模型调用。
// 参数 name：凭据名称；record：已存凭据（允许缺字段）；current：部署声明的协议。
// 返回 bool：兼容时为真；不修改凭据或部署，空协议沿用历史允许行为。
// 七牛同时提供 OpenAI 兼容接口和原生接口；只有明确七牛身份的凭据可跨这两个声明使用，
// 不能依据请求模型名或地址把任意 OpenAI 凭据升级为七牛。旧内置名称仅在缺少 builtin 时兜底。
// 测试：validate_credential_test.go、regression/model_credential_test.go。
func credentialProtocolMatches(name string, record map[string]any, current string) bool {
	info, _ := record["credential_info"].(map[string]any)
	values, _ := record["credential_values"].(map[string]any)
	protocol := credentialText(info, "custom_llm_provider")
	if protocol == "" {
		protocol = credentialText(values, "custom_llm_provider")
	}
	protocol = strings.ToLower(protocol)
	current = strings.ToLower(strings.TrimSpace(current))
	if protocol == "" || current == "" || protocol == current {
		return true
	}
	builtin := credentialText(info, "builtin")
	qiniu := builtin == BuiltinQiniu || (builtin == "" && name == BuiltinQiniu)
	return qiniu && (protocol == "openai" || protocol == "qiniu") && (current == "openai" || current == "qiniu")
}
