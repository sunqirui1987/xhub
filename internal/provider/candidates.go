package provider

import (
	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/llm"
)

// CandidateDecision 保存同一入口的兼容性判断；Deployment 仅内部使用，禁止直接序列化凭据。
type CandidateDecision struct {
	Deployment config.ModelEntry
	Reason     string
}

// Candidates 对已匹配公开模型的部署执行目录、声明和请求能力筛选。
// 参数 list、endpoint、dialogue：部署、目录入口和可选已解析对话；返回兼容部署与全部判断。
// 调用：实际统一选路、原生选路及只读预览；不附加凭据、不调用上游、不推进策略状态。
func Candidates(list []config.ModelEntry, endpoint string, dialogue *llm.Dialogue) ([]config.ModelEntry, []CandidateDecision) {
	pool := make([]config.ModelEntry, 0, len(list))
	decisions := make([]CandidateDecision, 0, len(list))
	for _, dep := range list {
		err := AllowsEndpoint(dep, endpoint)
		if err == nil && dialogue != nil {
			execution, _ := Execution(dep)
			_, err = llm.EncodeDialogue(*dialogue, execution.Protocol, dep.ParamString("model", ""))
		}
		decision := CandidateDecision{Deployment: dep}
		if err != nil {
			decision.Reason = err.Error()
		} else {
			pool = append(pool, dep)
		}
		decisions = append(decisions, decision)
	}
	return pool, decisions
}
