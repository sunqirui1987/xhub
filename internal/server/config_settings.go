package server

import (
	"net/http"

	"github.com/sunqirui1987/xhub/internal/httpx"
)

func (s *Server) baseRouterSettings() map[string]any {
	rs := map[string]any{
		"routing_strategy":         s.Cfg.RouterSettings.RoutingStrategy,
		"routing_strategy_args":    map[string]any{},
		"routing_groups":           []any{},
		"num_retries":              s.Cfg.RouterSettings.NumRetries,
		"timeout":                  s.Cfg.RouterSettings.Timeout,
		"stream_timeout":           nil,
		"max_fallbacks":            5,
		"fallbacks":                []any{},
		"context_window_fallbacks": []any{},
		"content_policy_fallbacks": []any{},
		"retry_policy":             map[string]any{},
		"model_group_retry_policy": map[string]any{},
		"model_group_alias":        map[string]any{},
		"allowed_fails":            3,
		"cooldown_time":            0,
		"retry_after":              0,
		"enable_pre_call_checks":   false,
		"enable_tag_filtering":     false,
	}
	for k, v := range s.Cfg.RouterRaw {
		rs[k] = v
	}
	return rs
}

func (s *Server) mergedRouterSettings() map[string]any {
	db, err := s.Store.ListConfig("router_settings")
	if err != nil || db == nil {
		db = map[string]any{}
	}
	return Overlay(s.baseRouterSettings(), db)
}

func (s *Server) mergedGeneralSettings() map[string]any {
	base := map[string]any{}
	for k, v := range s.Cfg.GeneralRaw {
		if k == "master_key" || k == "database_url" || k == "redis_url" {
			continue
		}
		base[k] = v
	}
	db, err := s.Store.ListConfig("general_settings")
	if err != nil || db == nil {
		db = map[string]any{}
	}
	return Overlay(base, db)
}

func (s *Server) saveNamespacePatch(namespace string, patch map[string]any) error {
	var current map[string]any
	switch namespace {
	case "router_settings":
		current = s.mergedRouterSettings()
	case "general_settings":
		current = s.mergedGeneralSettings()
	default:
		db, err := s.Store.ListConfig(namespace)
		if err != nil || db == nil {
			db = map[string]any{}
		}
		current = db
	}
	merged := MergePatch(current, patch)
	for k := range patch {
		if err := s.Store.PutConfig(namespace, k, merged[k]); err != nil {
			return err
		}
	}
	if namespace == "router_settings" {
		s.applyTypedRouter(s.mergedRouterSettings())
	}
	return nil
}

func (s *Server) applyTypedRouter(m map[string]any) {
	if v := str(m["routing_strategy"]); v != "" {
		s.Cfg.RouterSettings.RoutingStrategy = v
	}
	if _, ok := m["num_retries"]; ok {
		s.Cfg.RouterSettings.NumRetries = asInt(m["num_retries"])
	}
	if _, ok := m["timeout"]; ok {
		switch t := m["timeout"].(type) {
		case float64:
			s.Cfg.RouterSettings.Timeout = t
		case int:
			s.Cfg.RouterSettings.Timeout = float64(t)
		}
	}
}

func (s *Server) configUpdate(w http.ResponseWriter, r *http.Request) {
	if s.requireManage(w, r) == nil {
		return
	}
	body := readMap(r)
	for _, ns := range []string{"router_settings", "general_settings", "litellm_settings"} {
		patch, ok := body[ns].(map[string]any)
		if !ok || patch == nil {
			continue
		}
		if err := s.saveNamespacePatch(ns, patch); err != nil {
			httpx.WriteError(w, 500, "internal", err.Error())
			return
		}
	}
	httpx.WriteJSON(w, 200, map[string]any{
		"status":          "success",
		"router_settings": s.mergedRouterSettings(),
	})
}

type generalField struct {
	name, typ, desc string
	def             any
	opts            []string
	tab             string
}

func generalFieldCatalog() []generalField {
	return []generalField{
		{"enable_anthropic_prompt_caching", "Boolean", "Automatically add Anthropic prompt-cache breakpoints", false, nil, "prompt_caching"},
		{"anthropic_prompt_caching_ttl", "Select", "How long Anthropic keeps the prompt cache", nil, []string{"5m", "1h"}, "prompt_caching"},
		{"mcp_internal_ip_ranges", "List", "Internal IP ranges treated as private for MCP", []any{}, nil, ""},
		{"alert_to_webhook_url", "Dictionary", "Alert type to webhook URL", map[string]any{}, nil, ""},
		{"allow_requests_on_db_unavailable", "Boolean", "Allow requests when the database is unavailable", false, nil, ""},
		{"budget_exceeded_throttle_percentage", "Float", "Fraction of traffic to throttle after a budget is exceeded", nil, nil, ""},
		{"max_ui_session_budget", "Dollar", "Maximum spend for a dashboard session", nil, nil, ""},
	}
}

func (s *Server) generalSettingsList() []map[string]any {
	yamlBase := map[string]any{}
	for k, v := range s.Cfg.GeneralRaw {
		yamlBase[k] = v
	}
	db, err := s.Store.ListConfig("general_settings")
	if err != nil || db == nil {
		db = map[string]any{}
	}
	out := make([]map[string]any, 0, len(generalFieldCatalog()))
	for _, f := range generalFieldCatalog() {
		item := map[string]any{
			"field_name":          f.name,
			"field_type":          f.typ,
			"field_description":   f.desc,
			"field_default_value": f.def,
			"field_value":         f.def,
			"stored_in_db":        nil,
		}
		if f.tab != "" {
			item["field_tab"] = f.tab
		}
		if f.opts != nil {
			item["field_options"] = f.opts
		}
		if v, ok := yamlBase[f.name]; ok {
			item["field_value"] = v
			item["stored_in_db"] = false
		}
		if v, ok := db[f.name]; ok {
			item["field_value"] = v
			item["stored_in_db"] = true
		}
		out = append(out, item)
	}
	return out
}

func (s *Server) configFieldUpdate(w http.ResponseWriter, r *http.Request) {
	if s.requireManage(w, r) == nil {
		return
	}
	body := readMap(r)
	name := str(body["field_name"])
	if name == "" {
		httpx.WriteError(w, 400, "invalid_request", "field_name required")
		return
	}
	ns := str(body["config_type"])
	if ns == "" {
		ns = "general_settings"
	}
	if err := s.Store.PutConfig(ns, name, body["field_value"]); err != nil {
		httpx.WriteError(w, 500, "internal", err.Error())
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"status": "success", "field_name": name})
}

func (s *Server) configFieldDelete(w http.ResponseWriter, r *http.Request) {
	if s.requireManage(w, r) == nil {
		return
	}
	body := readMap(r)
	name := str(body["field_name"])
	if name == "" {
		httpx.WriteError(w, 400, "invalid_request", "field_name required")
		return
	}
	ns := str(body["config_type"])
	if ns == "" {
		ns = "general_settings"
	}
	if err := s.Store.DeleteConfig(ns, name); err != nil {
		httpx.WriteError(w, 500, "internal", err.Error())
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"status": "success", "field_name": name})
}
