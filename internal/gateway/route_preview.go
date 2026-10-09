package gateway

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/sunqirui1987/xhub/internal/authz"
	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/gateway/models"
	"github.com/sunqirui1987/xhub/internal/gateway/prefs"
	"github.com/sunqirui1987/xhub/internal/gateway/templateauth"
	"github.com/sunqirui1987/xhub/internal/httpx"
	"github.com/sunqirui1987/xhub/internal/llm"
	"github.com/sunqirui1987/xhub/internal/provider"
	"github.com/sunqirui1987/xhub/internal/router"
)

// previewDeployment 是预览的安全投影，只包含声明与路由事实，不序列化连接凭据或内部地址。
type previewDeployment struct {
	DeploymentID string   `json:"deployment_id"`
	Connection   string   `json:"connection"`
	Supplier     string   `json:"supplier"`
	Model        string   `json:"upstream_model"`
	Transport    string   `json:"transport"`
	Protocol     string   `json:"protocol"`
	Reason       string   `json:"excluded_reason,omitempty"`
	Weight       *float64 `json:"weight,omitempty"`
	Share        *float64 `json:"target_share,omitempty"`
}

// routePreview 校验可见模板或授权草稿并预览候选，不请求上游、不扣额度、不修改轮询状态。
// 参数 w、r：HTTP 响应及包含 model_name、endpoint_id、template_id、可选 body/request 的请求。
// 返回：规则来源、策略及安全部署列表；无效输入返回 400，不可见模板按统一授权规则拒绝。
// 调用：路由模板及部署页面；草稿的权限与对应模板写入权限相同。
func (s *Server) routePreview(w http.ResponseWriter, r *http.Request) {
	p := s.RequireUser(w, r)
	if p == nil {
		return
	}
	var input struct {
		Model      string         `json:"model_name"`
		Endpoint   string         `json:"endpoint_id"`
		TemplateID string         `json:"template_id"`
		OrgID      string         `json:"organization_id"`
		TeamID     string         `json:"team_id"`
		Body       map[string]any `json:"body"`
		Request    map[string]any `json:"request"`
	}
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	if err := decoder.Decode(&input); err != nil || strings.TrimSpace(input.Model) == "" || !provider.KnownEndpoint(input.Endpoint) {
		httpx.WriteTypedError(w, r.URL.Path, 400, "invalid_request", "model_name and registered endpoint_id are required")
		return
	}
	var trailing any
	if decoder.Decode(&trailing) != io.EOF {
		httpx.WriteTypedError(w, r.URL.Path, 400, "invalid_request", "exactly one JSON object is required")
		return
	}
	if !models.AllowsModel(s, r.Context(), p, p.TeamID, input.Model) {
		httpx.WriteTypedError(w, r.URL.Path, 403, "forbidden", "model is not allowed")
		return
	}
	settings := prefs.BuiltinSettings()
	object := authz.Object{Type: authz.ObjectRouteTemplate, OrgID: input.OrgID, TeamID: input.TeamID}
	if input.TemplateID != "" {
		if s.IAM == nil {
			httpx.WriteTypedError(w, r.URL.Path, 503, "unavailable", "identity store unavailable")
			return
		}
		if s.WriteAuthz(w, r, templateauth.Selection(s, r, p, input.TemplateID)) {
			return
		}
		row, err := s.IAM.GetRouteTemplate(r.Context(), input.TemplateID)
		if err != nil || row == nil {
			httpx.WriteTypedError(w, r.URL.Path, 503, "unavailable", "template unavailable")
			return
		}
		object.ID = row.ID
		if row.OrganizationID != nil {
			object.OrgID = *row.OrganizationID
		} else {
			object.OrgID = ""
		}
		if row.TeamID != nil {
			object.TeamID = *row.TeamID
		} else {
			object.TeamID = ""
		}
		settings = prefs.RouteSettings{Settings: row.Settings(), TemplateID: row.ID, TemplateName: row.Name, Source: "selected"}
	} else {
		// 只读继承解析直接读取绑定，不进入会修改速率计数的预算检查。
		for _, scope := range prefs.RequestChain(p.KeyID, p.TeamID, p.OrgID) {
			if s.IAM == nil {
				break
			}
			id, err := s.IAM.ScopeRouteTemplate(r.Context(), scope.Kind, scope.ID)
			if err != nil {
				httpx.WriteTypedError(w, r.URL.Path, 503, "unavailable", "template binding unavailable")
				return
			}
			if id == "" {
				continue
			}
			if s.WriteAuthz(w, r, templateauth.Selection(s, r, p, id)) {
				return
			}
			row, err := s.IAM.GetRouteTemplate(r.Context(), id)
			if err != nil {
				httpx.WriteTypedError(w, r.URL.Path, 503, "unavailable", "template unavailable")
				return
			}
			if row != nil {
				settings = prefs.RouteSettings{Settings: row.Settings(), TemplateID: row.ID, TemplateName: row.Name, Source: scope.Kind}
			}
			break
		}
	}
	if input.Body != nil {
		if s.WriteAuthz(w, r, s.Authorize(r, p, authz.ActionRouteTemplateWrite, object)) {
			return
		}
		if err := prefs.ValidateRouteTemplateDocument(input.Body); err != nil {
			httpx.WriteTypedError(w, r.URL.Path, 400, "invalid_request", err.Error())
			return
		}
		settings.Settings = input.Body
		settings.Source = "draft"
	}
	// 预览与数据面从同一编译器读取模型默认、模板组和回退。
	s.LockModels()
	directory := append([]config.ModelEntry(nil), (*s.ModelTable())...)
	s.UnlockModels()
	settings = router.Compile(settings, s.RecordStore(), directory)
	settings = settings.ForModel(input.Model)
	if settings.Err != nil {
		httpx.WriteTypedError(w, r.URL.Path, 400, "invalid_request", settings.Err.Error())
		return
	}
	if err := router.ValidateStrategy(settings.Strategy()); err != nil {
		httpx.WriteTypedError(w, r.URL.Path, 400, "invalid_request", err.Error())
		return
	}
	var dialogue *llm.Dialogue
	if input.Request != nil {
		for _, entry := range provider.EndpointTypes() {
			if entry.ID == input.Endpoint && entry.Kind == provider.KindAdapted && provider.DialogueProtocol(entry.Protocol) {
				d, err := llm.ParseDialogue(entry.Protocol, input.Request)
				if err != nil {
					httpx.WriteTypedError(w, r.URL.Path, 400, "unsupported_capability", err.Error())
					return
				}
				dialogue = &d
			}
		}
	}
	s.LockModels()
	list := append([]config.ModelEntry(nil), s.Models()...)
	s.UnlockModels()
	// 保留暂停部署以解释原因；实际匹配函数会排除它们。
	matched := settings.Candidates(list, input.Model)
	for _, dep := range list {
		if dep.ModelName == input.Model && dep.Disabled() {
			matched = append(matched, dep)
		}
	}
	compatible, decisions := provider.Candidates(matched, input.Endpoint, dialogue)
	state := s.RouteState()
	rows := make([]previewDeployment, 0, len(decisions))
	total := 0.0
	weighted := router.IsSplitStrategy(settings.Strategy())
	state.Allocations = settings.Policy.Shares()
	available := map[string]bool{}
	for _, dep := range router.Available(compatible, settings.Strategy(), state) {
		available[router.CooldownID(dep)] = true
	}
	for _, decision := range decisions {
		dep := decision.Deployment
		row := previewDeployment{DeploymentID: router.DeploymentID(dep), Connection: dep.ParamString("litellm_credential_name", ""), Supplier: dep.ParamString("custom_llm_provider", ""), Model: dep.ParamString("model", ""), Transport: provider.SelectedTransport(dep), Reason: decision.Reason}
		if execution, ok := provider.Execution(dep); ok {
			row.Protocol = execution.Protocol
		}
		if row.Reason == "" && !available[router.CooldownID(dep)] && state.Cooldown[router.CooldownID(dep)] {
			row.Reason = "deployment is cooling down"
		}
		if weighted {
			weight, exists := state.Allocations[router.DeploymentID(dep)]
			if !exists {
				weight = 1
			}
			row.Weight = &weight
			if row.Reason == "" && weight <= 0 {
				row.Reason = "deployment has no allocated traffic"
			}
			if row.Reason == "" {
				total += weight
			}
		}
		if !weighted && row.Reason == "" {
			total++
		}
		rows = append(rows, row)
	}
	if weighted || settings.Strategy() == "random" {
		for i := range rows {
			share := 0.0
			if rows[i].Reason == "" && total > 0 {
				if weighted {
					share = *rows[i].Weight / total
				} else {
					share = 1 / total
				}
			}
			rows[i].Share = &share
		}
	}
	httpx.WriteJSON(w, 200, map[string]any{"model_name": input.Model, "endpoint_id": input.Endpoint, "template_id": settings.TemplateID, "source": settings.Source, "rule_source": settings.RuleSource, "routing_strategy": settings.Strategy(), "data": rows})
}
