// Package prefs reads and writes router settings and general settings. A key present in the database overrides YAML. A key that is absent stays.
package prefs

import (
	"net/http"

	"github.com/sunqirui1987/xhub/internal/httpx"
	"github.com/sunqirui1987/xhub/internal/logx"
	"sync"
)

var logTraceOnceSettings sync.Once

// Base is the router-settings baseline from YAML and code defaults. allowed_fails is fixed at 3 here.
// A merged allowed_fails of at least 1 records the failure in Redis and starts cooldown. A cooldown_time of 0 means one minute. Only an explicit value below 1 skips recording the failure.
func Base(s Host) map[string]any {
	logTraceOnceSettings.Do(func() { logx.Trace("enter prefs.Base") })

	rs := map[string]any{
		"routing_strategy":         s.Config().RouterSettings.RoutingStrategy,
		"routing_strategy_args":    map[string]any{},
		"routing_groups":           []any{},
		"num_retries":              s.Config().RouterSettings.NumRetries,
		"timeout":                  s.Config().RouterSettings.Timeout,
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
	for k, v := range s.Config().RouterRaw {
		rs[k] = v
	}
	return rs
}

// MergedRouter overlays database router settings on the baseline. Keys absent from the database keep the YAML value.
func MergedRouter(s Host) map[string]any {
	db, err := s.DB().ListConfig("router_settings")
	if err != nil || db == nil {
		db = map[string]any{}
	}
	return Overlay(Base(s), db)
}

// MergedGeneral overlays database general settings on YAML. master_key, database_url, and redis_url are not exposed from the database.
func MergedGeneral(s Host) map[string]any {
	base := map[string]any{}
	for k, v := range s.Config().GeneralRaw {
		if k == "master_key" || k == "database_url" || k == "redis_url" {
			continue
		}
		base[k] = v
	}
	db, err := s.DB().ListConfig("general_settings")
	if err != nil || db == nil {
		db = map[string]any{}
	}
	return Overlay(base, db)
}

// saveNamespacePatch writes a partial update into one namespace. Only keys present in the patch are stored, and a router-settings change refreshes the in-process strategy.
func saveNamespacePatch(s Host, namespace string, patch map[string]any) error {
	var current map[string]any
	switch namespace {
	case "router_settings":
		current = MergedRouter(s)
	case "general_settings":
		current = MergedGeneral(s)
	default:
		db, err := s.DB().ListConfig(namespace)
		if err != nil || db == nil {
			db = map[string]any{}
		}
		current = db
	}
	merged := MergePatch(current, patch)
	for k := range patch {
		if err := s.DB().PutConfig(namespace, k, merged[k]); err != nil {
			return err
		}
	}
	if namespace == "router_settings" {
		ApplyTyped(s, MergedRouter(s))
	}
	return nil
}

// ApplyTyped copies strategy, retries, and timeout from the merged document into the typed config. Fields that are absent are left unchanged.
func ApplyTyped(s Host, m map[string]any) {
	if v := str(m["routing_strategy"]); v != "" {
		s.Config().RouterSettings.RoutingStrategy = v
	}
	if _, ok := m["num_retries"]; ok {
		s.Config().RouterSettings.NumRetries = asInt(m["num_retries"])
	}
	if _, ok := m["timeout"]; ok {
		switch t := m["timeout"].(type) {
		case float64:
			s.Config().RouterSettings.Timeout = t
		case int:
			s.Config().RouterSettings.Timeout = float64(t)
		}
	}
}

// Update accepts a partial update of router, general, or LiteLLM settings. It requires a management identity.
func Update(s Host, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	body := readMap(r)
	for _, ns := range []string{"router_settings", "general_settings", "litellm_settings"} {
		patch, ok := body[ns].(map[string]any)
		if !ok || patch == nil {
			continue
		}
		if err := saveNamespacePatch(s, ns, patch); err != nil {
			httpx.WriteError(w, 500, "internal", err.Error())
			return
		}
	}
	httpx.WriteJSON(w, 200, map[string]any{
		"status":          "success",
		"router_settings": MergedRouter(s),
	})
}

type generalField struct {
	name, typ, desc string
	def             any
	opts            []string
	tab             string
}

// generalFieldCatalog returns the field definitions the general-settings page shows, including type and default.
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

// GeneralList returns the general-settings list with stored_in_db. A key only in the database is true, a key only in YAML is false, and a key in neither is null.
func GeneralList(s Host) []map[string]any {
	yamlBase := map[string]any{}
	for k, v := range s.Config().GeneralRaw {
		yamlBase[k] = v
	}
	db, err := s.DB().ListConfig("general_settings")
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

// FieldUpdate updates one general-settings field. A missing field_name returns 400.
func FieldUpdate(s Host, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
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
	if err := s.DB().PutConfig(ns, name, body["field_value"]); err != nil {
		httpx.WriteError(w, 500, "internal", err.Error())
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"status": "success", "field_name": name})
}

// FieldDelete deletes one general-settings field. After deletion the YAML baseline applies again.
func FieldDelete(s Host, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
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
	if err := s.DB().DeleteConfig(ns, name); err != nil {
		httpx.WriteError(w, 500, "internal", err.Error())
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"status": "success", "field_name": name})
}
