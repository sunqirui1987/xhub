package provider

import (
	"github.com/sunqirui1987/xhub/internal/config"
	"net/url"
)

// EndpointBinding 表示客户端可调用的公开端点契约，不包含上游密钥和内部地址。
// 同一模型别名可以绑定多种协议；路径、方法和后续任务操作均来自已验证的部署。
type EndpointBinding struct {
	// EndpointID 是公开协议目录中的稳定 ID，例如 fal:queue；不是模型名称。
	EndpointID string `json:"endpoint_id"`
	// Transport 是执行该部署的注册传输 ID，供配置和排查使用；客户端仍以 Path 调用。
	Transport string `json:"transport"`
	// Kind 区分统一协议适配与原生透传，前端据此选择参数编辑及可用控制项。
	Kind Kind `json:"kind"`
	// Protocol 指定请求和响应事件的协议，避免把 Responses、Messages 的终态混用。
	Protocol string `json:"protocol"`
	// Family 表示 chat、image、video 等能力分类，不能代替明确的端点声明。
	Family string `json:"family"`
	// Method 与 Path 共同确定客户端可调用的创建操作，路径始终指向本网关。
	Method string `json:"method"`
	Path   string `json:"path"`
	// Actions 列出该模型对应的创建和任务查询契约，不包含上游地址或供应商凭据。
	Actions []Action `json:"actions,omitempty"`
}

// DeploymentEndpoints 将有效部署投影成可公开的端点列表。
// 参数 m（config.ModelEntry）：模型部署，必须显式声明传输，标准接口自动生成，Bypass 按声明开放。
// 返回 []EndpointBinding：只含该模型可调用的创建路径和配套任务操作；无效或禁用部署返回空列表。
// 调用：模型可用列表、管理页面模型分组。测试：bindings_test.go。
func DeploymentEndpoints(m config.ModelEntry) []EndpointBinding {
	out := []EndpointBinding{}
	if m.Disabled() || ValidateDeployment(m) != nil {
		return out
	}
	// 对话入口路径来自公开目录；供应商和自定义执行配置只决定如何访问上游。
	selected := SelectedTransport(m)
	for _, e := range EndpointTypes() {
		if !DialogueProtocol(e.Protocol) || AllowsEndpoint(m, e.ID) != nil {
			continue
		}
		for _, path := range e.Paths {
			out = append(out, EndpointBinding{EndpointID: e.ID, Transport: selected, Kind: e.Kind, Protocol: e.Protocol, Family: e.Family, Method: "POST", Path: Expand(path, map[string]string{"model": url.PathEscape(m.ModelName)})})
		}
	}
	// 非对话异步操作保留显式执行器的任务路径，不承诺跨媒体转换。
	for _, t := range Transports() {
		if DialogueProtocol(t.Protocol) || t.ID != selected || AllowsEndpoint(m, t.EndpointID) != nil {
			continue
		}
		model := OfficialID(t.StripPrefix, m.ParamString("model", ""))
		for _, a := range t.Actions {
			if a.Name != "create" || (a.Model != "" && a.Model != model) {
				continue
			}
			actions := []Action{}
			for _, action := range t.Actions {
				if action.Model != "" && action.Model != model {
					continue
				}
				actions = append(actions, Action{Name: action.Name, Method: action.Method, PublicPath: Expand(action.PublicPath, map[string]string{"model": url.PathEscape(m.ModelName)}), TaskQuery: action.TaskQuery})
			}
			out = append(out, EndpointBinding{EndpointID: t.EndpointID, Transport: selected, Kind: t.Kind, Protocol: t.Protocol, Family: t.Family, Method: a.Method, Path: Expand(a.PublicPath, map[string]string{"model": url.PathEscape(m.ModelName)}), Actions: actions})
		}
	}
	return out
}

// MergeEndpoints 合并同一对外模型别名下多条部署的端点。
// 参数 current、next（[]EndpointBinding）：已有绑定和新增绑定。
// 返回 []EndpointBinding：按 HTTP 方法和公开路径去重的并集，保留首次声明的顺序。
// 调用：模型可用列表。测试：bindings_test.go。
func MergeEndpoints(current, next []EndpointBinding) []EndpointBinding {
	for _, item := range next {
		found := false
		for _, old := range current {
			if old.Method == item.Method && old.Path == item.Path {
				found = true
				break
			}
		}
		if !found {
			current = append(current, item)
		}
	}
	return current
}
