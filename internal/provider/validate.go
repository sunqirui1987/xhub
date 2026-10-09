package provider

import (
	"fmt"
	"github.com/sunqirui1987/xhub/internal/config"
	"slices"
	"strings"
)

// ValidateDeployment 校验新系统的显式端点绑定，不从历史字段推断默认值。
// 参数 m（config.ModelEntry）：待创建或更新的部署。
// 返回 error：缺少传输、端点、供应商不匹配或固定模型不在白名单时返回错误；合法时为 nil。
// 调用：模型写入校验、DeploymentEndpoints。测试：validate_test.go。
func ValidateDeployment(m config.ModelEntry) error {
	if err := ValidateCatalogBinding(str(m.ModelInfo["catalog_id"]), m); err != nil {
		return err
	}
	id := SelectedTransport(m)
	if id == "" {
		return fmt.Errorf("model_info.transport must select a registered transport")
	}
	ids := stringList(m.ModelInfo["endpoint_types"])
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
		// 对话别名可包含模型路径和版本冒号；只拒绝会被 URL 清理改变的空段、点段，保证 Google 入口可调用。
		if DialogueProtocol(t.Protocol) {
			for _, segment := range strings.Split(m.ModelName, "/") {
				if strings.TrimSpace(segment) == "" || segment == "." || segment == ".." {
					return fmt.Errorf("对外模型名称不能为空，路径段不能留空或为 .、..；支持斜杠（/）和冒号（:）")
				}
			}
		}
		for _, endpoint := range ids {
			valid := false
			for _, entry := range EndpointTypes() {
				if entry.ID == endpoint && CompatibleEndpoint(entry, t) {
					valid = true
				}
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
