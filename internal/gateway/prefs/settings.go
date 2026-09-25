// 路由设置和通用设置的读写。数据库里出现的键覆盖 YAML，没出现的键保留。
package prefs

import (
	"net/http"

	"github.com/sunqirui1987/xhub/internal/httpx"
	"github.com/sunqirui1987/xhub/internal/settings"
)

// YAML 和代码默认值合成的路由设置基线。allowed_fails 在这里固定写成 3。
// 合并后的值大于等于 1 时，失败会计入 Redis 并开始冷却；cooldown_time 为 0 时按 1 分钟。只有显式写成小于 1 才不记这次失败。
func Base(s Host) map[string]any {
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

// 数据库路由设置覆盖基线。数据库没有的键保留 YAML。
func MergedRouter(s Host) map[string]any {
	db, err := s.DB().ListConfig("router_settings")
	if err != nil || db == nil {
		db = map[string]any{}
	}
	return settings.Overlay(Base(s), db)
}

// 数据库通用设置覆盖 YAML。master_key、database_url、redis_url 不会从库里露出来。
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
	return settings.Overlay(base, db)
}

// 把局部更新写入命名空间。只保存补丁里出现的键，并在路由设置变更后刷新进程内策略。
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
	merged := settings.MergePatch(current, patch)
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

// 把合并结果里的策略、重试和超时写进类型化配置。没出现的字段不改。
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

// 接收路由、通用或 LiteLLM 设置的局部更新。需要管理身份。
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

// 控制台通用设置页要展示的字段定义，含类型和默认值。
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

// 带 stored_in_db 的通用设置列表。只在库里有的键为 true，只在 YAML 里的为 false，两边都没有为 null。
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

// 更新一个通用设置字段。缺少 field_name 时返回 400。
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

// 删除一个通用设置字段。删除后 YAML 基线重新生效。
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
