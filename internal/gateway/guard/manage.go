package guard

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/sunqirui1987/xhub/internal/httpx"
)

// manageMu 串行化管理读改写，保证重名校验、密钥合并与保存处于同一临界区。
var manageMu sync.Mutex

// Manage 处理护栏管理、调试和能力接口；保存前执行校验，读取时遮蔽凭据。
// 参数：s：数据面宿主；w/r：HTTP 响应和请求。调用方必须已通过 RequireManage 管理员鉴权。
// 返回：bool：路径已处理为 true；不属于本模块的路径为 false。错误及结果直接写 HTTP 响应。
// 调用：family.ServeMgmt；不执行团队审核注册。
// 测试：engine_test.go、internal/gateway/guardrail_manage_test.go。
func Manage(s Host, w http.ResponseWriter, r *http.Request) bool {
	path := strings.TrimSuffix(r.URL.Path, "/")
	if path != "/guardrails" && !strings.HasPrefix(path, "/guardrails/") && path != "/v2/guardrails/list" {
		return false
	}
	switch path {
	case "/guardrails/ui/add_guardrail_settings":
		httpx.WriteJSON(w, 200, map[string]any{"engine": "xhub", "supported_modes": []string{"pre_call"}, "supported_entities": []string{}, "supported_actions": []string{"BLOCK", "MASK"}, "pii_entity_categories": []any{}, "guardrail_provider_map": providerDirectory()})
		return true
	case "/guardrails/ui/provider_specific_params":
		httpx.WriteJSON(w, 200, map[string]any{"local": map[string]any{"ui_friendly_name": "Local rules", "blocked_words": map[string]any{"type": "list", "description": "Case-insensitive keywords", "required": false}, "patterns": map[string]any{"type": "list", "description": "Regular expressions (RE2)", "required": false}, "action": map[string]any{"type": "select", "description": "Action on match", "options": []string{"block", "redact"}, "required": true}, "replacement": map[string]any{"type": "str", "description": "Replacement text", "required": false}}})
		return true
	case "/guardrails/submissions":
		httpx.WriteJSON(w, 200, map[string]any{"supported": false, "submissions": []any{}, "summary": map[string]int{"total": 0, "pending_review": 0, "active": 0, "rejected": 0}})
		return true
	case "/guardrails/test_custom_code":
		testCustomCode(w, r)
		return true
	case "/guardrails/register", "/guardrails/validate_blocked_words_file":
		httpx.WriteError(w, 501, "not_implemented", "remote submissions are not implemented; create a local rule or an xgo policy")
		return true
	}
	if strings.HasPrefix(path, "/guardrails/submissions/") || strings.HasPrefix(path, "/guardrails/ui/") {
		httpx.WriteError(w, 501, "not_implemented", "this guardrail capability is not implemented")
		return true
	}
	if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch || r.Method == http.MethodDelete {
		manageMu.Lock()
		defer manageMu.Unlock()
	}
	// 用量接口拥有独立响应合约，由现有用量模块处理。
	if strings.HasPrefix(path, "/guardrails/usage/") {
		return false
	}
	if path == "/guardrails/list" || path == "/v2/guardrails/list" {
		if r.Method != http.MethodGet {
			httpx.WriteError(w, 405, "method_not_allowed", "GET required")
			return true
		}
		if s.RecordStore() == nil {
			rows := configuredRules(s)
			httpx.WriteJSON(w, 200, map[string]any{"guardrails": publicRules(rows), "data": publicRules(rows), "object": "list", "engine": "xhub"})
			return true
		}
	}
	for _, rule := range configuredRules(s) {
		if ruleIdentifies(rule, strings.TrimSuffix(strings.TrimPrefix(path, "/guardrails/"), "/info")) {
			if r.Method == http.MethodGet {
				httpx.WriteJSON(w, 200, publicRule(rule))
			} else {
				httpx.WriteError(w, 400, "invalid_request", "edit config-defined guardrails in YAML")
			}
			return true
		}
	}
	if s.RecordStore() == nil {
		httpx.WriteError(w, 503, "unavailable", "guardrail store unavailable")
		return true
	}
	fail := func(err error) {
		if errors.Is(err, sql.ErrNoRows) {
			httpx.WriteError(w, 404, "not_found", "guardrail not found")
		} else {
			httpx.WriteError(w, 503, "unavailable", "guardrail store unavailable")
		}
	}
	if path == "/guardrails/list" || path == "/v2/guardrails/list" {
		rows, err := storedRules(s)
		if err != nil {
			fail(err)
			return true
		}
		httpx.WriteJSON(w, 200, map[string]any{"guardrails": publicRules(rows), "data": publicRules(rows), "object": "list", "engine": "xhub"})
		return true
	}
	id := strings.TrimSuffix(strings.TrimPrefix(path, "/guardrails/"), "/info")
	if path == "/guardrails" {
		id = ""
	}
	if strings.Contains(id, "/") {
		httpx.WriteError(w, 404, "not_found", "unknown guardrail operation")
		return true
	}
	kind := "guardrails"
	var row map[string]any
	if id != "" {
		var err error
		row, err = s.RecordStore().GetKV(kind, id)
		if errors.Is(err, sql.ErrNoRows) {
			kind = "guardrail"
			row, err = s.RecordStore().GetKV(kind, id)
		}
		if err != nil {
			fail(err)
			return true
		}
	}
	switch r.Method {
	case http.MethodGet:
		if row == nil {
			httpx.WriteError(w, 400, "invalid_request", "guardrail id required")
		} else {
			httpx.WriteJSON(w, 200, publicRule(normalizeRule(row)))
		}
	case http.MethodDelete:
		if id == "" {
			httpx.WriteError(w, 400, "invalid_request", "guardrail id required")
			return true
		}
		if err := s.RecordStore().DeleteKV(kind, id); err != nil {
			fail(err)
			return true
		}
		httpx.WriteJSON(w, 200, map[string]any{"guardrail_id": id, "deleted": 1})
	case http.MethodPost, http.MethodPut, http.MethodPatch:
		var body map[string]any
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<20)).Decode(&body); err != nil || body == nil {
			httpx.WriteError(w, 400, "invalid_request", "JSON object required")
			return true
		}
		if inner, ok := body["guardrail"].(map[string]any); ok {
			body = inner
		}
		if row == nil {
			row = map[string]any{"created_at": time.Now().UTC().Format(time.RFC3339)}
		}
		for key, value := range body {
			if key == "id" || key == "guardrail_id" || key == "created_at" || key == "guardrail_definition_location" {
				continue
			}
			if key == "litellm_params" {
				patch, ok := value.(map[string]any)
				if !ok {
					httpx.WriteError(w, 400, "invalid_request", "litellm_params must be an object")
					return true
				}
				params, _ := row[key].(map[string]any)
				if params == nil {
					params = map[string]any{}
				}
				for name, v := range patch {
					// 浏览器拿到的掩码或留空表示保留原凭据；显式 null 才删除。
					if secretFields[name] && (v == credentialMask || v == "") && params[name] != nil {
						continue
					}
					if v == nil {
						delete(params, name)
					} else {
						params[name] = v
					}
				}
				row[key] = params
			} else {
				row[key] = value
			}
		}
		if err := Validate(row); err != nil {
			httpx.WriteError(w, 400, "invalid_request", err.Error())
			return true
		}
		rows, err := storedRules(s)
		if err != nil {
			fail(err)
			return true
		}
		for _, existing := range rows {
			if ruleIdentifies(existing, findingName(row)) && str(existing["guardrail_id"]) != id {
				httpx.WriteError(w, 409, "conflict", "guardrail name conflicts with an existing name or id")
				return true
			}
		}
		if id == "" {
			id = "guardrail_" + httpx.CallID()[:12]
		}
		row["id"], row["guardrail_id"] = id, id
		row["updated_at"] = time.Now().UTC().Format(time.RFC3339)
		row["guardrail_definition_location"] = "db"
		raw, err := json.Marshal(row)
		if err != nil {
			httpx.WriteError(w, 400, "invalid_request", err.Error())
			return true
		}
		if err := s.RecordStore().PutKV(kind, id, string(raw)); err != nil {
			fail(err)
			return true
		}
		httpx.WriteJSON(w, 200, publicRule(normalizeRule(row)))
	default:
		httpx.WriteError(w, 405, "method_not_allowed", "method not allowed")
	}
	return true
}

// normalizeRule 补齐旧存储字段别名，保证新旧规则具有稳定的管理响应结构。
// 参数：row：可修改的规则 map。
// 返回：原规则对象；补齐 guardrail_id、guardrail_name、litellm_params、定义来源。
// 调用：Manage、storedRules、configuredRules。
// 测试：engine_test.go。
func normalizeRule(row map[string]any) map[string]any {
	if row["guardrail_id"] == nil {
		row["guardrail_id"] = row["id"]
	}
	if row["guardrail_name"] == nil {
		row["guardrail_name"] = row["name"]
	}
	if row["litellm_params"] == nil {
		row["litellm_params"] = map[string]any{}
	}
	if row["guardrail_definition_location"] == nil {
		row["guardrail_definition_location"] = "db"
	}
	return row
}

// storedRules 合并 YAML 规则与两个历史 KV 命名空间，再按优先级排序。
// 参数：s：具备非 nil RecordStore 的宿主；调用方处理存储不可用情况。
// 返回：规则列表和存储错误；读取失败不返回部分规则作为成功结果。
// 调用：Manage、listGuardrails。
// 测试：engine_test.go、internal/gateway/guardrail_manage_test.go。
func storedRules(s Host) ([]map[string]any, error) {
	out := configuredRules(s)
	for _, kind := range []string{"guardrails", "guardrail"} {
		rows, err := s.RecordStore().ListKV(kind)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			out = append(out, normalizeRule(row))
		}
	}
	sortRules(out)
	return out, nil
}
