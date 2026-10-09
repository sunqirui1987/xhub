// Package prefs builds the router-settings page, the callback view, and the general-settings list. The numbers come from the merged document.
package prefs

import (
	"net/http"

	"github.com/sunqirui1987/xhub/internal/httpx"
	"github.com/sunqirui1987/xhub/internal/logx"
	"sync"
)

var logTraceOncePage sync.Once

// Page writes the merged router settings and field descriptions for the dashboard to render.
// 参数 s（Host）：分页使用的数据面宿主；w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；r（*http.Request）：入站 HTTP 请求。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：gateway/prefs/mount.go
// 测试：无直接单测
func Page(s Host, w http.ResponseWriter, r *http.Request) {
	logTraceOncePage.Do(func() { logx.Trace("enter prefs.Page") })

	if s.RequireManage(w, r) == nil {
		return
	}
	rs := MergedRouter(s)
	httpx.WriteJSON(w, 200, map[string]any{
		"num_retries":                   rs["num_retries"],
		"timeout":                       rs["timeout"],
		"router_settings":               rs,
		"fields":                        Fields(s, rs),
		"current_values":                rs,
		"routing_strategy_descriptions": routingStrategyDescriptions(),
	})
}

// PageMap returns the merged router-settings document, including routing_groups.
// 参数 s（Host）：分页表使用的数据面宿主。
// 返回 map[string]any（map[string]any）：分页表的字段表。缺键表示上游或库里没有这个字段。
// 调用：仅在 page.go 内使用
// 测试：无直接单测
func PageMap(s Host) map[string]any {
	return MergedRouter(s)
}

// routingStrategyDescriptions returns the help text for each known strategy name. An unknown strategy is omitted.
// 参数：无。
// 返回 map[string]string（map[string]string）：routingStrategyDescriptions的字符串表。没有该键表示这项没有填。
// 调用：仅在 page.go 内使用
// 测试：无直接单测
func routingStrategyDescriptions() map[string]string {
	return map[string]string{"random": "简单随机", "traffic-split": "按部署百分比分配", "least-busy": "最少并发", "cost-based-routing": "最低成本", "latency-based-routing": "最低延迟", "usage-based-routing": "最低用量"}
}

// Fields splits router settings into the dashboard field list, with the current value and the default.
// 参数 s（Host）：Fields使用的数据面宿主；rs（map[string]any）：Fields读到的 JSON 对象。缺键表示没有该字段。
// 返回 []map[string]any（[]map[string]any）：一组map[string]any。没有匹配时为空切片，不是 nil 分页。
// 调用：仅在 page.go 内使用
// 测试：无直接单测
func Fields(s Host, rs map[string]any) []map[string]any {
	type spec struct {
		name, typ, desc, ui string
		def                 any
		opts                []string
	}
	specs := []spec{
		{"num_retries", "Integer", "失败请求的尝试次数", "尝试次数", 1, nil},
		{"timeout", "Float", "请求超时秒数", "超时", 60, nil},
		{"allowed_fails", "Integer", "触发冷却的失败次数；0表示关闭", "失败阈值", 3, nil},
		{"cooldown_time", "Float", "冷却秒数；0表示一分钟", "冷却时间", 0, nil},
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

// Callbacks returns the callback view. Its router_settings match the merged document.
// 参数 s（Host）：Callbacks使用的数据面宿主；w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；r（*http.Request）：入站 HTTP 请求。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：gateway/prefs/mount.go
// 测试：无直接单测
func Callbacks(s Host, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	cbs := []any{}
	for _, kind := range []string{"callback", "callbacks"} {
		list, _ := s.RecordStore().ListKV(kind)
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
		"status":                       "success",
		"callbacks":                    cbs,
		"success_callbacks":            cbs,
		"failure_callbacks":            []any{},
		"available_callbacks":          map[string]any{},
		"alerts":                       []any{},
		"active_alerting_destinations": []any{},
		"router_settings":              MergedRouter(s),
	})
}

// List returns settings for the requested config_type. general_settings includes stored_in_db.
// 参数 s（Host）：列出使用的数据面宿主；w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；r（*http.Request）：入站 HTTP 请求。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：gateway/keys/generate.go、gateway/keys/mount.go、gateway/models/list.go、gateway/models/mount.go
// 测试：无直接单测
func List(s Host, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	if r.URL.Query().Get("config_type") == "general_settings" || r.URL.Query().Get("config_type") == "" {
		httpx.WriteJSON(w, 200, GeneralList(s))
		return
	}
	httpx.WriteJSON(w, 200, []map[string]any{
		{
			"field_name": "mcp_internal_ip_ranges", "field_type": "List", "field_value": []any{},
			"field_description": "Internal IP ranges treated as private for MCP", "stored_in_db": false,
			"field_default_value": []any{},
		},
		{
			"field_name": "allow_requests_on_db_unavailable", "field_type": "Boolean", "field_value": false,
			"field_description": "Allow requests when the database is unavailable", "stored_in_db": false,
			"field_default_value": false,
		},
	})
}
