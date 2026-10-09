package models

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/httpx"
	"github.com/sunqirui1987/xhub/internal/router"
)

// checkFallbackRemoval 在删除或改名最后一个公开部署之前检查回退引用。
// 参数为宿主、公开名与部署 ID；返回冲突或存储错误，调用方持有模型锁。
// 保留同名其他部署时允许修改；存在源策略或目标引用时要求先清除配置，避免跨表部分更新。
func checkFallbackRemoval(s Host, name, id string) error {
	if err := checkRoutingAllocationRemoval(s, id); err != nil {
		return err
	}
	for _, dep := range *s.ModelTable() {
		if dep.ModelName == name && str(dep.ModelInfo["id"]) != id && !nonModelEntry(dep) {
			return nil
		}
	}
	if err := checkRoutingGroupRemoval(s, name); err != nil {
		return err
	}
	policies, err := s.RecordStore().ListConfig("model_fallbacks")
	if err != nil {
		return err
	}
	for source, raw := range policies {
		policy, err := router.ParseFallbackPolicy(raw)
		if err != nil {
			return err
		}
		if source == name && len(policy.Fallbacks)+len(policy.ContextWindow)+len(policy.ContentPolicy) > 0 {
			return fmt.Errorf("clear fallback policy for %q before deleting or renaming its last deployment", name)
		}
		for _, targets := range [][]string{policy.Fallbacks, policy.ContextWindow, policy.ContentPolicy} {
			for _, target := range targets {
				if target == name {
					return fmt.Errorf("remove fallback reference from %q before deleting or renaming %q", source, name)
				}
			}
		}
	}
	return nil
}

// SetFallback 保存或清除公开模型回退配置；参数为管理员请求的 model_name 和 policy。
// 返回保存结果或 400/403/404/503；模型锁串行化目标校验与写入，不调用上游。
func SetFallback(s Host, w http.ResponseWriter, r *http.Request) {
	p := s.RequireManage(w, r)
	if p == nil {
		return
	}
	if !p.PlatformAdmin() {
		httpx.WriteError(w, 403, "forbidden", "platform administrator required")
		return
	}
	var input struct {
		Model  string          `json:"model_name"`
		Policy json.RawMessage `json:"policy"`
	}
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&input); err != nil || input.Model == "" {
		httpx.WriteError(w, 400, "invalid_request", "model_name and policy required")
		return
	}
	var trailing any
	if dec.Decode(&trailing) != io.EOF {
		httpx.WriteError(w, 400, "invalid_request", "exactly one JSON object required")
		return
	}
	policy, err := router.ParseFallbackPolicy(input.Policy)
	if err != nil {
		httpx.WriteError(w, 400, "invalid_request", err.Error())
		return
	}
	s.LockModels()
	defer s.UnlockModels()
	exists := false
	for _, dep := range *s.ModelTable() {
		if dep.ModelName == input.Model && !nonModelEntry(dep) {
			exists = true
		}
	}
	if !exists {
		httpx.WriteError(w, 404, "not_found", "public model not found")
		return
	}
	raw, err := s.RecordStore().ListConfig("model_fallbacks")
	if err != nil {
		httpx.WriteError(w, 503, "unavailable", "model fallbacks unavailable")
		return
	}
	policies := map[string]router.FallbackPolicy{}
	for name, value := range raw {
		if name == input.Model {
			continue
		}
		parsed, e := router.ParseFallbackPolicy(value)
		if e != nil {
			httpx.WriteError(w, 503, "unavailable", "invalid stored fallback policy")
			return
		}
		policies[name] = parsed
	}
	policies[input.Model] = policy
	models := []config.ModelEntry{}
	for _, dep := range *s.ModelTable() {
		if !nonModelEntry(dep) {
			models = append(models, dep)
		}
	}
	if err = router.ValidateFallbackGraph(policies, models); err != nil {
		httpx.WriteError(w, 400, "invalid_request", err.Error())
		return
	}
	if s.RecordStore() == nil {
		httpx.WriteError(w, 503, "unavailable", "model fallback store unavailable")
		return
	}
	if len(policy.Fallbacks)+len(policy.ContextWindow)+len(policy.ContentPolicy) == 0 {
		err = s.RecordStore().DeleteConfig("model_fallbacks", input.Model)
	} else {
		err = s.RecordStore().PutConfig("model_fallbacks", input.Model, policy)
	}
	if err != nil {
		httpx.WriteError(w, 503, "unavailable", err.Error())
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"model_name": input.Model, "fallback_policy": policy})
}
