package provider

import (
	"fmt"
	"slices"
	"strings"

	"github.com/sunqirui1987/xhub/internal/config"
)

// Execution 解析部署显式选择的上游配置，不使用供应商、地址或型号推断协议。
// 参数为部署；返回注册配置及存在标记。调用：校验、候选筛选、统一与原生执行。
func Execution(m config.ModelEntry) (Transport, bool) {
	for _, t := range Transports() {
		if t.ID == SelectedTransport(m) {
			return t, true
		}
	}
	return Transport{}, false
}

// DialogueProtocol 判断注册协议是否实现首版对话转换；参数为协议 ID，返回支持状态。
// 调用：部署校验和统一入口；媒体协议保持原执行器，不跨媒体转换。
func DialogueProtocol(protocol string) bool {
	return slices.Contains([]string{"openai-chat", "openai-responses", "anthropic-messages", "gemini", "vertex"}, protocol)
}

// AllowsEndpoint 校验入口目录、部署声明与上游协议兼容性，返回明确的排除原因。
// 参数为部署及用户入口 ID；调用：实际选路与只读预览，无副作用。
func AllowsEndpoint(m config.ModelEntry, endpoint string) error {
	if m.Disabled() {
		return fmt.Errorf("deployment disabled")
	}
	if err := ValidateDeployment(m); err != nil {
		return err
	}
	declared := stringList(m.ModelInfo["endpoint_types"])
	standard := false
	for _, e := range EndpointTypes() {
		if e.ID == endpoint && e.Category != "bypass" {
			standard = true
		}
	}
	if !standard && !slices.Contains(declared, endpoint) {
		return fmt.Errorf("endpoint %s is not enabled", endpoint)
	}
	for _, e := range EndpointTypes() {
		if e.ID != endpoint {
			continue
		}
		if t, ok := Execution(m); ok {
			if CompatibleEndpoint(e, t) {
				return nil
			}
		}
		return fmt.Errorf("upstream execution is incompatible with %s", endpoint)
	}
	return fmt.Errorf("endpoint is not registered")
}

// ResolveHit 将用户原生操作绑定到选中部署的执行路径与鉴权配置。
// 参数为入口命中和部署；返回实际执行命中或错误。调用：原生创建/查询，不接受任意 URL。
func ResolveHit(hit Hit, m config.ModelEntry) (Hit, error) {
	if err := AllowsEndpoint(m, hit.Transport.EndpointID); err != nil {
		return Hit{}, err
	}
	t, ok := Execution(m)
	if !ok {
		return Hit{}, fmt.Errorf("execution profile missing")
	}
	// 固定动作保存上游模型路径；部署可保留执行传输声明的目录前缀。
	model := OfficialID(t.StripPrefix, m.ParamString("model", ""))
	for _, a := range t.Actions {
		if a.Name == hit.Action.Name && (a.Model == "" || a.Model == model) {
			// 一个原生协议有多个 create 操作，必须保持入站操作后缀，禁止流式被发成非流式。
			if t.Protocol == "gemini" || t.Protocol == "vertex" {
				if a.PublicPath[strings.LastIndex(a.PublicPath, ":")+1:] != hit.Action.PublicPath[strings.LastIndex(hit.Action.PublicPath, ":")+1:] {
					continue
				}
			}
			// 路径模型来自部署，入口范围保持入站协议，避免模板与任务跨入口。
			names := make(map[string]string, len(hit.Names)+1)
			for k, v := range hit.Names {
				names[k] = v
			}
			if t.Protocol == "gemini" || t.Protocol == "vertex" {
				names["model"] = model
			}
			t.EndpointID = hit.Transport.EndpointID
			return Hit{Transport: t, Action: a, Names: names}, nil
		}
	}
	return Hit{}, fmt.Errorf("operation is not registered")
}
