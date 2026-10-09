package router

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/sunqirui1987/xhub/internal/config"
	"math"
)

// Allocation 表示一个稳定部署 ID 的目标权重，仅用于 traffic-split。
type Allocation struct {
	DeploymentID string  `json:"deployment_id"`
	Weight       float64 `json:"weight"`
}

// Policy 是客户覆盖及内部执行使用的分配文档。
type Policy struct {
	Strategy    string       `json:"strategy"`
	Allocations []Allocation `json:"allocations,omitempty"`
}

// DefaultWeights 是模型管理唯一的默认分配配置，仅含相对权重。
type DefaultWeights struct {
	Allocations []Allocation `json:"allocations"`
}

// ParseDefaultWeights 严格读取全局权重，转换为加权随机策略供请求及目录使用；不接受策略字段，无副作用。
func ParseDefaultWeights(raw any) (Policy, error) {
	data, err := json.Marshal(raw)
	if err != nil {
		return Policy{}, err
	}
	var weights DefaultWeights
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&weights); err != nil {
		return Policy{}, err
	}
	p := Policy{Strategy: "traffic-split", Allocations: weights.Allocations}
	return p, p.Validate()
}

// ParsePolicy 严格解析 JSON 分配对象，供配置写入和请求解析使用；返回策略或字段错误，无副作用。
func ParsePolicy(raw any) (Policy, error) {
	data, err := json.Marshal(raw)
	if err != nil {
		return Policy{}, err
	}
	var p Policy
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err = dec.Decode(&p); err != nil {
		return p, fmt.Errorf("invalid allocation policy: %w", err)
	}
	return p, p.Validate()
}

// Validate 检查策略与权重，供写入和运行时共用；返回重复部署、非有限数或全零权重错误，不修改输入。
func (p Policy) Validate() error {
	if err := ValidateStrategy(p.Strategy); err != nil {
		return err
	}
	if p.Strategy != "traffic-split" {
		if len(p.Allocations) != 0 {
			return fmt.Errorf("allocations require traffic-split")
		}
		return nil
	}
	// 空列表表示所有兼容部署使用权重 1。模型管理只有在用户调整过时才需要保存明细。
	if len(p.Allocations) == 0 {
		return nil
	}
	seen := map[string]bool{}
	total := 0.0
	for _, a := range p.Allocations {
		if a.DeploymentID == "" || seen[a.DeploymentID] {
			return fmt.Errorf("deployment_id must be nonempty and unique")
		}
		seen[a.DeploymentID] = true
		if math.IsNaN(a.Weight) || math.IsInf(a.Weight, 0) || a.Weight < 0 {
			return fmt.Errorf("weight must be finite and nonnegative")
		}
		total += a.Weight
	}
	if math.IsInf(total, 0) {
		return fmt.Errorf("total weight must be finite")
	}
	if total <= 0 {
		return fmt.Errorf("at least one deployment must have positive weight")
	}
	return nil
}

// Shares 返回部署 ID 到权重的只读映射，供调度和预览使用；未列出的部署由调度器使用默认权重 1，不读旧权重。
func (p Policy) Shares() map[string]float64 {
	out := map[string]float64{}
	for _, a := range p.Allocations {
		out[a.DeploymentID] = a.Weight
	}
	return out
}

// ValidateDeployments 检查份额引用属于指定公开模型，供默认策略及客户模板写入使用；不修改部署。
func (p Policy) ValidateDeployments(list []config.ModelEntry, model string) error {
	ids := map[string]bool{}
	for _, dep := range list {
		if dep.ModelName == model {
			ids[DeploymentID(dep)] = true
		}
	}
	for _, a := range p.Allocations {
		if !ids[a.DeploymentID] {
			return fmt.Errorf("deployment %q does not belong to public model %q", a.DeploymentID, model)
		}
	}
	return nil
}
