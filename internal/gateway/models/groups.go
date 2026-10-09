package models

import (
	"encoding/json"
	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/httpx"
	"github.com/sunqirui1987/xhub/internal/router"
	"io"
	"net/http"
	"strings"
)

// ModelGroup 聚合一个公开模型的部署、默认权重和独立回退策略，不复制供应商数据。
type ModelGroup struct {
	Fallback    router.FallbackPolicy `json:"fallback_policy"`
	Name        string                `json:"model_name"`
	Deployments []map[string]any      `json:"deployments"`
	Default     router.DefaultWeights `json:"default_weights"`
}

// groupModels 按公开名称聚合完整部署；搜索任一部署命中即返回完整组，防止搜索和分页拆散分配配置。
// 参数为部署目录、默认配置和搜索词；返回安全组目录或无效默认错误，不修改共享配置。
func groupModels(list []config.ModelEntry, defaults map[string]any, search string) ([]ModelGroup, error) {
	groups := []ModelGroup{}
	indexes := map[string]int{}
	matches := map[string]bool{}
	search = strings.ToLower(strings.TrimSpace(search))
	for _, dep := range list {
		if nonModelEntry(dep) {
			continue
		}
		i, ok := indexes[dep.ModelName]
		if !ok {
			i = len(groups)
			indexes[dep.ModelName] = i
			policy := router.Policy{Strategy: "traffic-split"}
			if raw, present := defaults[dep.ModelName]; present {
				var err error
				policy, err = router.ParseDefaultWeights(raw)
				if err != nil {
					return nil, err
				}
			}
			groups = append(groups, ModelGroup{Name: dep.ModelName, Default: router.DefaultWeights{Allocations: policy.Allocations}, Deployments: []map[string]any{}})
		}
		row := Public(dep)
		row["id"] = router.DeploymentID(dep)
		row["db_model"] = modelIsDB(dep)
		groups[i].Deployments = append(groups[i].Deployments, row)
		text := dep.ModelName + " " + router.DeploymentID(dep) + " " + dep.ParamString("model", "") + " " + dep.ParamString("litellm_credential_name", "") + " " + dep.ParamString("custom_llm_provider", "")
		if search == "" || strings.Contains(strings.ToLower(text), search) {
			matches[dep.ModelName] = true
		}
	}
	out := []ModelGroup{}
	for _, g := range groups {
		if matches[g.Name] {
			out = append(out, g)
		}
	}
	return out, nil
}

// Groups 提供管理页按公开模型分页的目录；管理员鉴权后返回组内部署、实时默认权重和回退策略。
// 参数为宿主和 HTTP 请求；写入安全 JSON 或权限、存储错误；不调用上游。
func Groups(s Host, w http.ResponseWriter, r *http.Request) {
	p := s.RequireManage(w, r)
	if p == nil {
		return
	}
	if !p.PlatformAdmin() {
		httpx.WriteError(w, 403, "forbidden", "platform administrator required")
		return
	}
	defaults, err := s.RecordStore().ListConfig("model_defaults")
	if err != nil {
		httpx.WriteError(w, 503, "unavailable", "model defaults unavailable")
		return
	}
	fallbacks, err := s.RecordStore().ListConfig("model_fallbacks")
	if err != nil {
		httpx.WriteError(w, 503, "unavailable", "model fallbacks unavailable")
		return
	}
	s.LockModels()
	list := append([]config.ModelEntry(nil), (*s.ModelTable())...)
	s.UnlockModels()
	groups, err := groupModels(list, defaults, r.URL.Query().Get("search"))
	if err != nil {
		httpx.WriteError(w, 500, "invalid_model_default", err.Error())
		return
	}
	for i := range groups {
		if raw, exists := fallbacks[groups[i].Name]; exists {
			groups[i].Fallback, err = router.ParseFallbackPolicy(raw)
			if err != nil {
				httpx.WriteError(w, 500, "invalid_model_fallback", err.Error())
				return
			}
		}
	}
	page, size := queryIntDefault(r, "page", 1), queryIntDefault(r, "size", 20)
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 20
	}
	if size > 200 {
		size = 200
	}
	total := len(groups)
	start := (page - 1) * size
	if start > total {
		start = total
	}
	end := start + size
	if end > total {
		end = total
	}
	pages := 1
	if total > 0 {
		pages = (total + size - 1) / size
	}
	httpx.WriteJSON(w, 200, map[string]any{"data": groups[start:end], "total_count": total, "current_page": page, "total_pages": pages})
}

// SetDefault 保存公开模型的唯一全局分配；严格验证相对权重和部署归属，不存储分配策略。
// 参数为 model_name、weights 的请求；返回保存结果或 400/404/503，无旧字段迁移或上游调用。
func SetDefault(s Host, w http.ResponseWriter, r *http.Request) {
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
		Policy json.RawMessage `json:"weights"`
	}
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil || input.Model == "" {
		httpx.WriteError(w, 400, "invalid_request", "model_name and weights required")
		return
	}
	var trailing any
	if decoder.Decode(&trailing) != io.EOF {
		httpx.WriteError(w, 400, "invalid_request", "exactly one JSON object required")
		return
	}
	policy, err := router.ParseDefaultWeights(input.Policy)
	if err != nil {
		httpx.WriteError(w, 400, "invalid_request", err.Error())
		return
	}
	// 校验与写入共用模型锁，避免部署同时删除后存入悬空分配引用。
	s.LockModels()
	defer s.UnlockModels()
	list := *s.ModelTable()
	exists := false
	for _, dep := range list {
		if dep.ModelName == input.Model {
			exists = true
		}
	}
	if !exists {
		httpx.WriteError(w, 404, "not_found", "public model not found")
		return
	}
	if err := policy.ValidateDeployments(list, input.Model); err != nil {
		httpx.WriteError(w, 400, "invalid_request", err.Error())
		return
	}
	if err := s.RecordStore().PutConfig("model_defaults", input.Model, router.DefaultWeights{Allocations: policy.Allocations}); err != nil {
		httpx.WriteError(w, 503, "unavailable", err.Error())
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"model_name": input.Model, "weights": router.DefaultWeights{Allocations: policy.Allocations}})
}
