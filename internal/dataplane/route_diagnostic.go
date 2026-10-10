package dataplane

import (
	"errors"
	"fmt"
	"math"

	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/llm"
	"github.com/sunqirui1987/xhub/internal/logx"
	"github.com/sunqirui1987/xhub/internal/provider"
	"github.com/sunqirui1987/xhub/internal/router"
)

// routeDiagnosis 保存当前公开模型的只读筛选事实；部署仅在内部使用，日志只输出白名单字段。
type routeDiagnosis struct {
	loaded, matched, disabled, compatible, available, cooling, zeroWeight int
	endpoint                                                              string
	decisions                                                             []provider.CandidateDecision
	stale                                                                 []router.Allocation
	state                                                                 router.State
}

// diagnoseRoute 为 Serve 解释目录、声明、请求能力、冷却及权重筛选。
// 参数 directory 为未过滤目录，eligible 为实际兼容池，alias/endpoint 为公开请求标识，settings/state 为解析快照，
// dialogue/adapted 控制请求能力检查；返回诊断，不调用上游、不修改目录、权重或随机/轮询状态。
func diagnoseRoute(directory, eligible []config.ModelEntry, alias, endpoint string, settings router.RouteSettings, state router.State, dialogue *llm.Dialogue, adapted bool) routeDiagnosis {
	d := routeDiagnosis{loaded: len(directory), endpoint: endpoint, state: state}
	active, _ := dropDisabled(directory)
	matched := settings.Candidates(active, alias)
	d.matched = len(matched)
	if !adapted {
		dialogue = nil
	}
	_, d.decisions = provider.Candidates(matched, endpoint, dialogue)
	// 单独匹配暂停目录，避免全局暂停计数把其他公开模型误判为已禁用；复制元数据不修改共享配置。
	var paused []config.ModelEntry
	for _, dep := range directory {
		if !dep.Disabled() {
			continue
		}
		info := make(map[string]any, len(dep.ModelInfo))
		for k, v := range dep.ModelInfo {
			info[k] = v
		}
		info["disabled"] = false
		dep.ModelInfo = info
		paused = append(paused, dep)
	}
	for _, dep := range settings.Candidates(paused, alias) {
		d.disabled++
		d.decisions = append(d.decisions, provider.CandidateDecision{Deployment: dep, Reason: "deployment disabled"})
	}
	compatible := settings.Candidates(eligible, alias)
	d.compatible = len(compatible)
	d.available = len(router.Available(compatible, settings.Strategy(), state))
	for _, dep := range compatible {
		if state.Cooldown[router.CooldownID(dep)] {
			d.cooling++
		}
		if router.IsSplitStrategy(settings.Strategy()) {
			w, exists := state.Allocations[router.DeploymentID(dep)]
			if exists && (w <= 0 || math.IsNaN(w) || math.IsInf(w, 0)) {
				d.zeroWeight++
			}
		}
	}
	ids := map[string]bool{}
	for _, decision := range d.decisions {
		ids[router.DeploymentID(decision.Deployment)] = true
	}
	for _, allocation := range settings.Policy.Allocations {
		if !ids[allocation.DeploymentID] {
			d.stale = append(d.stale, allocation)
		}
	}
	return d
}

// log 输出本次请求的阶段计数和部署排除原因；参数为路径、关联 ID、公开名和规则快照。
// Serve 在调度前调用，无返回值，仅写日志；凭据、正文及完整上游地址不输出，字符串引用以隔离换行。
func (d routeDiagnosis) log(path, callID, alias string, settings router.RouteSettings) {
	logx.Debug("process path=%s step=route_diagnosis call_id=%q model=%q endpoint=%q loaded=%d matched=%d disabled=%d compatible=%d available=%d cooling=%d zero_weight=%d strategy=%q template_id=%q template_name=%q source=%q rule_source=%q", path, callID, alias, d.endpoint, d.loaded, d.matched, d.disabled, d.compatible, d.available, d.cooling, d.zeroWeight, settings.Strategy(), settings.TemplateID, settings.TemplateName, settings.Source, settings.RuleSource)
	for _, decision := range d.decisions {
		dep, reason := decision.Deployment, decision.Reason
		weight, exists := d.state.Allocations[router.DeploymentID(dep)]
		if !exists {
			weight = 1
		}
		if reason == "" && d.state.Cooldown[router.CooldownID(dep)] {
			reason = "deployment is cooling down"
		}
		if reason == "" && router.IsSplitStrategy(settings.Strategy()) && (weight <= 0 || math.IsNaN(weight) || math.IsInf(weight, 0)) {
			reason = "deployment has no allocated traffic"
		}
		if reason == "" {
			reason = "eligible"
		}
		protocol := ""
		transport, _ := dep.ModelInfo["transport"].(string)
		if execution, ok := provider.Execution(dep); ok {
			protocol = execution.Protocol
		}
		logx.Debug("process path=%s step=route_candidate call_id=%q model=%q deployment_id=%q public_model=%q upstream_model=%q connection=%q supplier=%q upstream_host=%q transport=%q protocol=%q weight=%g weight_applied=%t reason=%q", path, callID, alias, router.DeploymentID(dep), dep.ModelName, dep.ParamString("model", ""), dep.ParamString("litellm_credential_name", ""), dep.ParamString("custom_llm_provider", ""), baseHost(dep.ParamString("api_base", "")), transport, protocol, weight, router.IsSplitStrategy(settings.Strategy()), safeErr(errors.New(reason)))
	}
	for _, allocation := range d.stale {
		logx.Debug("process path=%s step=route_allocation call_id=%q model=%q deployment_id=%q weight=%g reason=%q", path, callID, alias, allocation.DeploymentID, allocation.Weight, "allocation references no matching deployment")
	}
}

// failure 将空路由映射为可操作的接口错误；参数 alias 为请求公开名，返回状态、类型和安全提示。
// Serve 仅在主模型及回退均耗尽后调用，不向客户泄漏内部部署、地址或供应商校验细节。
func (d routeDiagnosis) failure(alias string) (int, string, string) {
	if d.matched == 0 {
		if d.disabled > 0 {
			return 400, "model_disabled", "model is disabled: " + alias
		}
		return 400, "invalid_request", "model not found: " + alias
	}
	if d.compatible == 0 {
		return 400, "model_unavailable", fmt.Sprintf("model %s exists but has no deployment compatible with endpoint %s and this request; check model transport, endpoint configuration and request capabilities", alias, d.endpoint)
	}
	if d.zeroWeight == d.compatible {
		return 503, "model_unavailable", "model " + alias + " has no allocated traffic; check deployment weights in model defaults or the selected route template"
	}
	if d.cooling == d.compatible {
		return 503, "model_unavailable", "model " + alias + " has all compatible deployments cooling down; retry later"
	}
	return 503, "model_unavailable", "model " + alias + " has no available deployment; check cooldowns and route allocations"
}
