package provider

import (
	"fmt"
	"github.com/sunqirui1987/xhub/internal/config"
	"slices"
)

// ValidateDeployment 校验新系统的显式端点绑定，不从历史字段推断默认值。
// 参数 m（config.ModelEntry）：待创建或更新的部署。
// 返回 error：缺少传输、端点、供应商不匹配或固定模型不在白名单时返回错误；合法时为 nil。
// 调用：模型写入校验、DeploymentEndpoints。测试：validate_test.go。
func ValidateDeployment(m config.ModelEntry) error {
	id := SelectedTransport(m)
	if id == "" {
		return fmt.Errorf("model_info.transport must select a registered transport")
	}
	ids := stringList(m.ModelInfo["endpoint_types"])
	if len(ids) == 0 {
		return fmt.Errorf("model_info.endpoint_types is required")
	}
	seen := map[string]bool{}
	for _, endpoint := range ids {
		if seen[endpoint] {
			return fmt.Errorf("duplicate endpoint type %s", endpoint)
		}
		seen[endpoint] = true
	}
	for _, t := range Transports() {
		if t.ID != id {
			continue
		}
		for _, endpoint := range ids {
			valid := false
			for _, entry := range EndpointTypes() {
				if entry.ID == endpoint && entry.Kind == KindBypass && entry.Protocol == t.Protocol && (DialogueProtocol(t.Protocol) || endpoint == t.EndpointID) {
					valid = true
				}
			}
			for _, entry := range EndpointTypes() {
				if entry.ID == endpoint && entry.Kind == KindAdapted && entry.Protocol == t.Protocol {
					valid = true
				}
			}
			if DialogueProtocol(t.Protocol) {
				valid = valid || slices.Contains([]string{"chat", "responses", "messages"}, endpoint)
			}
			if !valid {
				return fmt.Errorf("transport %s is incompatible with endpoint type %s", id, endpoint)
			}
		}
		slug := m.ParamString("custom_llm_provider", "")
		if len(t.Providers) > 0 && !slices.Contains(t.Providers, slug) {
			return fmt.Errorf("transport %s does not support provider %s", id, slug)
		}
		// 固定动作登记的是上游模型路径；部署可以保留目录使用的 qiniu/ 路由前缀。
		model := OfficialID(t.StripPrefix, m.ParamString("model", ""))
		fixed, found := false, false
		for _, action := range t.Actions {
			if action.Name == "create" && action.Model != "" {
				fixed = true
				found = found || model == action.Model
			}
		}
		if fixed && !found {
			return fmt.Errorf("model %s is not registered in transport %s", model, id)
		}
		return nil
	}
	return fmt.Errorf("unregistered transport %s", id)
}
