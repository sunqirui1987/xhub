package server

import (
	"encoding/json"
	"net/http"

	"github.com/sunqirui1987/xhub/internal/httpx"
)

func (s *Server) ssoGenerate(w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	code := "sso-" + httpx.CallID()[:10]
	s.mu.Lock()
	s.ssoCodes[code] = true
	s.mu.Unlock()
	ret := r.URL.Query().Get("return_to")
	if ret == "" {
		ret = r.URL.Query().Get("redirect_to")
	}
	url := "/login?code=" + code
	if ret != "" {
		url += "&redirect_to=" + ret
	}
	httpx.WriteJSON(w, 200, map[string]any{"url": url, "login_url": url})
}

func (s *Server) loginExchange(w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	var body struct {
		Code string `json:"code"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	s.mu.Lock()
	ok := s.ssoCodes[body.Code]
	delete(s.ssoCodes, body.Code)
	s.mu.Unlock()
	if !ok && body.Code == "" {
		httpx.WriteError(w, 401, "invalid_api_key", "invalid sso code")
		return
	}
	if !ok {
		httpx.WriteError(w, 401, "invalid_api_key", "invalid sso code")
		return
	}
	s.loginSuccess(w, "admin", "proxy_admin")
}

type emailEventSetting struct {
	Event   string `json:"event"`
	Enabled bool   `json:"enabled"`
}

func defaultEmailEventSettings() []emailEventSetting {
	return []emailEventSetting{
		{Event: "Virtual Key Created", Enabled: false},
		{Event: "New User Invitation", Enabled: true},
		{Event: "Virtual Key Rotated", Enabled: false},
		{Event: "Soft Budget Crossed", Enabled: false},
		{Event: "Max Budget Alert", Enabled: false},
	}
}

func (s *Server) currentEmailEventSettings() []emailEventSetting {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.emailEvents == nil {
		return defaultEmailEventSettings()
	}
	out := make([]emailEventSetting, len(s.emailEvents))
	copy(out, s.emailEvents)
	return out
}

func (s *Server) emailEventSettings(w http.ResponseWriter, r *http.Request) {
	if s.requireManage(w, r) == nil {
		return
	}
	if r.Method == http.MethodPatch {
		var body struct {
			Settings []emailEventSetting `json:"settings"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Settings == nil {
			httpx.WriteError(w, 400, "invalid_request", "settings required")
			return
		}
		s.mu.Lock()
		s.emailEvents = append([]emailEventSetting(nil), body.Settings...)
		s.mu.Unlock()
	}
	httpx.WriteJSON(w, 200, map[string]any{"settings": s.currentEmailEventSettings()})
}

func (s *Server) emailEventSettingsReset(w http.ResponseWriter, r *http.Request) {
	if s.requireManage(w, r) == nil {
		return
	}
	settings := defaultEmailEventSettings()
	s.mu.Lock()
	s.emailEvents = settings
	s.mu.Unlock()
	httpx.WriteJSON(w, 200, map[string]any{"settings": settings})
}

func (s *Server) flushCache(w http.ResponseWriter, r *http.Request) {
	httpx.WriteError(w, 404, "not_found", "Not Found")
}

func (s *Server) cacheSettings(w http.ResponseWriter, r *http.Request) {
	httpx.WriteError(w, 404, "not_found", "Not Found")
}

func (s *Server) routerSettings(w http.ResponseWriter, r *http.Request) {
	if s.requireManage(w, r) == nil {
		return
	}
	rs := s.mergedRouterSettings()
	httpx.WriteJSON(w, 200, map[string]any{
		"routing_strategy":              rs["routing_strategy"],
		"num_retries":                   rs["num_retries"],
		"timeout":                       rs["timeout"],
		"router_settings":               rs,
		"fields":                        s.routerSettingsFields(rs),
		"current_values":                rs,
		"routing_strategy_descriptions": routingStrategyDescriptions(),
	})
}

func (s *Server) routerSettingsMap() map[string]any {
	return s.mergedRouterSettings()
}

func routingStrategyDescriptions() map[string]string {
	return map[string]string{
		"simple-shuffle":         "Randomly picks a deployment from the list. Simple and fast.",
		"least-busy":             "Routes to the deployment with the lowest number of ongoing requests.",
		"latency-based-routing":  "Routes to the deployment with the lowest latency over a sliding window.",
		"cost-based-routing":     "Routes to the deployment with the lowest cost per token.",
		"usage-based-routing":    "Routes to the deployment with the lowest TPM (Tokens Per Minute) usage. (deprecated)",
		"usage-based-routing-v2": "Improved version of usage-based routing with better tracking.",
	}
}

func (s *Server) routerSettingsFields(rs map[string]any) []map[string]any {
	strats := []string{"simple-shuffle", "least-busy", "latency-based-routing", "cost-based-routing", "usage-based-routing", "usage-based-routing-v2"}
	type spec struct {
		name, typ, desc, ui string
		def                 any
		opts                []string
	}
	specs := []spec{
		{"routing_strategy", "String", "Routing strategy to use for load balancing across deployments", "Routing Strategy", "simple-shuffle", strats},
		{"routing_strategy_args", "Dictionary", "Arguments to pass to the routing strategy", "Routing Strategy Args", map[string]any{}, nil},
		{"routing_groups", "List", "Named subsets of model_names that share a routing strategy", "Routing Groups", []any{}, nil},
		{"num_retries", "Integer", "Number of retries for failed requests", "Number of Retries", 0, nil},
		{"timeout", "Float", "Timeout for requests in seconds", "Timeout", nil, nil},
		{"stream_timeout", "Float", "Timeout for streaming requests in seconds", "Stream Timeout", nil, nil},
		{"max_fallbacks", "Integer", "Maximum number of fallbacks to try before exiting the call", "Max Fallbacks", 5, nil},
		{"fallbacks", "List", "List of fallback model mappings", "Fallbacks", []any{}, nil},
		{"context_window_fallbacks", "List", "List of fallback models for context window errors", "Context Window Fallbacks", []any{}, nil},
		{"content_policy_fallbacks", "List", "List of fallback models for content policy errors", "Content Policy Fallbacks", []any{}, nil},
		{"allowed_fails", "Integer", "Number of times a deployment can fail before being added to cooldown", "Allowed Fails", 3, nil},
		{"cooldown_time", "Float", "Time in seconds to cooldown a deployment after failure", "Cooldown Time", nil, nil},
		{"retry_after", "Integer", "Minimum time to wait before retrying a failed request in seconds", "Retry After", 0, nil},
		{"retry_policy", "Dictionary", "Custom retry policy for different exception types", "Retry Policy", nil, nil},
		{"model_group_alias", "Dictionary", "Aliases for model groups", "Model Group Alias", map[string]any{}, nil},
		{"enable_pre_call_checks", "Boolean", "Enable pre-call checks before routing requests", "Enable Pre-call Checks", false, nil},
		{"enable_tag_filtering", "Boolean", "Enable tag-based routing", "Enable Tag Filtering", false, nil},
	}
	out := make([]map[string]any, 0, len(specs))
	for _, sp := range specs {
		item := map[string]any{
			"field_name":        sp.name,
			"field_type":        sp.typ,
			"field_value":       rs[sp.name],
			"field_description": sp.desc,
			"field_default":     sp.def,
			"ui_field_name":     sp.ui,
			"link":              nil,
		}
		if sp.opts != nil {
			item["options"] = sp.opts
		}
		out = append(out, item)
	}
	return out
}

func (s *Server) configCallbacks(w http.ResponseWriter, r *http.Request) {
	if s.requireManage(w, r) == nil {
		return
	}
	emptyVars := map[string]any{}
	cbs := []any{}
	for _, kind := range []string{"callback", "callbacks"} {
		list, _ := s.Store.ListKV(kind)
		for _, row := range list {
			name := str(row["callback_name"])
			if name == "" {
				name = str(row["name"])
			}
			if name == "" {
				name = str(row["id"])
			}
			if name != "" {
				cbs = append(cbs, name)
			} else {
				cbs = append(cbs, row)
			}
		}
	}
	httpx.WriteJSON(w, 200, map[string]any{
		"status":              "success",
		"callbacks":           cbs,
		"success_callbacks":   cbs,
		"failure_callbacks":   []any{},
		"available_callbacks": map[string]any{},
		"alerts": []any{
			map[string]any{"name": "slack", "variables": emptyVars, "active_alerts": []any{}, "alerts_to_webhook": map[string]any{}},
			map[string]any{"name": "email", "variables": emptyVars},
			map[string]any{"name": "ms_teams", "variables": emptyVars},
		},
		"active_alerting_destinations": []any{},
		"router_settings":              s.mergedRouterSettings(),
	})
}

func (s *Server) configList(w http.ResponseWriter, r *http.Request) {
	if s.requireManage(w, r) == nil {
		return
	}
	if r.URL.Query().Get("config_type") == "general_settings" || r.URL.Query().Get("config_type") == "" {
		httpx.WriteJSON(w, 200, s.generalSettingsList())
		return
	}
	httpx.WriteJSON(w, 200, []map[string]any{
		{
			"field_name": "mcp_internal_ip_ranges", "field_type": "List", "field_value": []any{},
			"field_description": "Internal IP ranges treated as private for MCP", "stored_in_db": false,
			"field_default_value": []any{},
		},
		{
			"field_name": "alert_to_webhook_url", "field_type": "Dictionary", "field_value": map[string]any{},
			"field_description": "Alert type to webhook URL", "stored_in_db": false,
			"field_default_value": map[string]any{},
		},
		{
			"field_name": "allow_requests_on_db_unavailable", "field_type": "Boolean", "field_value": false,
			"field_description": "Allow requests when the database is unavailable", "stored_in_db": false,
			"field_default_value": false,
		},
	})
}

func (s *Server) embeddings(w http.ResponseWriter, r *http.Request) {
	s.dataPlane(w, r, "embeddings")
}

func (s *Server) completions(w http.ResponseWriter, r *http.Request) {
	s.dataPlane(w, r, "completions")
}

func (s *Server) messages(w http.ResponseWriter, r *http.Request) {
	s.dataPlane(w, r, "messages")
}

func (s *Server) audioTranslations(w http.ResponseWriter, r *http.Request) {
	s.dataPlane(w, r, "audio_translation")
}
