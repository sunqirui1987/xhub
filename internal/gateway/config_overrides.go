package gateway

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/sunqirui1987/xhub/internal/httpx"
	"github.com/sunqirui1987/xhub/internal/logx"
	"sync"
)

var logTraceOnceConfigOverrides sync.Once

// configOverride reads and stores Hashicorp Vault or CyberArk settings. An empty store is a config object with no values, not a 404.
// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态码和正文写在这里；r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文。
// 调用：仅在 config_overrides.go 内使用
// 测试：无直接单测
// 返回：无。状态码和正文写进调用方的响应。
func (s *Server) configOverride(w http.ResponseWriter, r *http.Request) {
	logTraceOnceConfigOverrides.Do(func() { logx.Trace("enter gateway.configOverride") })

	if s.requireManage(w, r) == nil {
		return
	}
	kind := "cyberark"
	if strings.Contains(r.URL.Path, "hashicorp_vault") {
		kind = "hashicorp_vault"
	}
	if strings.HasSuffix(r.URL.Path, "/test_connection") {
		s.testConfigOverride(w, kind)
		return
	}
	switch r.Method {
	case http.MethodDelete:
		_ = s.Store.DeleteKV("config_overrides", kind)
		httpx.WriteJSON(w, 200, configOverrideBody(kind, map[string]any{}, true))
	case http.MethodPost:
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body == nil {
			body = map[string]any{}
		}
		delete(body, "id")
		delete(body, "kind")
		raw, _ := json.Marshal(map[string]any{"values": body})
		if err := s.Store.PutKV("config_overrides", kind, string(raw)); err != nil {
			httpx.WriteError(w, 500, "internal", err.Error())
			return
		}
		httpx.WriteJSON(w, 200, configOverrideBody(kind, body, false))
	default:
		values := map[string]any{}
		if saved, err := s.Store.GetKV("config_overrides", kind); err == nil {
			if raw, ok := saved["values"].(map[string]any); ok {
				values = raw
			}
		}
		httpx.WriteJSON(w, 200, configOverrideBody(kind, values, false))
	}
}

// testConfigOverride reports whether a secret backend is configured. It does not dial an external vault.
// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；kind（string）：分类名，用来选择限额主体、日志类型或官方端点。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：仅在 config_overrides.go 内使用
// 测试：无直接单测
func (s *Server) testConfigOverride(w http.ResponseWriter, kind string) {
	values := map[string]any{}
	if saved, err := s.Store.GetKV("config_overrides", kind); err == nil {
		if raw, ok := saved["values"].(map[string]any); ok {
			values = raw
		}
	}
	message := "No " + kind + " configuration found"
	status := "unconfigured"
	if len(values) > 0 {
		message = kind + " configuration is stored locally"
		status = "ok"
	}
	body := configOverrideBody(kind, values, false)
	body["status"] = status
	body["message"] = message
	httpx.WriteJSON(w, 200, body)
}

// configOverrideBody is the settings document the admin tabs render.
// 参数 kind（string）：分类名，用来选择限额主体、日志类型或官方端点；values（map[string]any）：配置Override正文读到的 JSON 对象。缺键表示没有该字段；deleted（bool）：为真时走deleted这一支。为假时保持原来的路径。
// 返回 map[string]any（map[string]any）：给响应或报表用的 JSON 对象。键是前端已经约定的字段，缺键表示这项没有数据。
// 调用：仅在 config_overrides.go 内使用
// 测试：无直接单测
func configOverrideBody(kind string, values map[string]any, deleted bool) map[string]any {
	if values == nil {
		values = map[string]any{}
	}
	body := map[string]any{
		"config_type":  kind,
		"field_schema": configOverrideSchema(kind),
		"values":       values,
	}
	if deleted {
		body["deleted"] = true
	}
	return body
}

// configOverrideSchema lists the fields the admin tab can show for this backend.
// 参数 kind（string）：分类名，用来选择限额主体、日志类型或官方端点。
// 返回 map[string]any（map[string]any）：配置Override模式的字段表。缺键表示上游或库里没有这个字段。
// 调用：仅在 config_overrides.go 内使用
// 测试：无直接单测
func configOverrideSchema(kind string) map[string]any {
	if kind == "hashicorp_vault" {
		return map[string]any{
			"vault_addr":        map[string]any{"type": "string"},
			"vault_token":       map[string]any{"type": "string"},
			"vault_namespace":   map[string]any{"type": "string"},
			"vault_mount_name":  map[string]any{"type": "string"},
			"vault_path_prefix": map[string]any{"type": "string"},
		}
	}
	return map[string]any{
		"cyberark_api_base": map[string]any{"type": "string"},
		"cyberark_account":  map[string]any{"type": "string"},
		"cyberark_username": map[string]any{"type": "string"},
		"cyberark_api_key":  map[string]any{"type": "string"},
	}
}
