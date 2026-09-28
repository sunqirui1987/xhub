// Package keys creates, lists, updates, and deletes virtual keys. The plaintext appears only in the create or rotate response.
package keys

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/sunqirui1987/xhub/internal/httpx"
	"github.com/sunqirui1987/xhub/internal/logx"
	"github.com/sunqirui1987/xhub/internal/store"
	"sync"
)

var logTraceOnceGenerate sync.Once

// Generate creates a virtual key and returns the plaintext only in this response.
func Generate(s Host, w http.ResponseWriter, r *http.Request) {
	logTraceOnceGenerate.Do(func() { logx.Trace("enter keys.Generate") })

	httpx.SetCallID(w, httpx.CallID())
	if s.RequireManage(w, r) == nil {
		return
	}
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && err != io.EOF {
		httpx.WriteError(w, 400, "invalid_request", "invalid json")
		return
	}
	if body == nil {
		body = map[string]any{}
	}
	plain := store.NewPlainKey()
	k, err := FromBody(plain, body)
	if err != nil {
		httpx.WriteError(w, 400, "invalid_request", err.Error())
		return
	}
	if err := s.DB().InsertKey(k); err != nil {
		httpx.WriteError(w, 500, "internal", err.Error())
		return
	}
	httpx.WriteJSON(w, 200, Response(k, plain, true))
}

// List lists virtual keys and does not return plaintext.
func List(s Host, w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	if s.RequireManage(w, r) == nil {
		return
	}
	keys, err := s.DB().ListKeys()
	if err != nil {
		httpx.WriteError(w, 500, "internal", err.Error())
		return
	}
	out := make([]map[string]any, 0, len(keys))
	for _, k := range keys {
		out = append(out, Response(k, "", false))
	}
	httpx.WriteJSON(w, 200, map[string]any{
		"keys":         out,
		"total_count":  len(out),
		"current_page": 1,
		"total_pages":  1,
		"size":         len(out),
	})
}

// Info reads one virtual key.
func Info(s Host, w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	if s.RequireManage(w, r) == nil {
		return
	}
	plain := r.URL.Query().Get("key")
	if r.Method == http.MethodPost {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if v, ok := body["key"].(string); ok {
			plain = v
		}
	}
	if plain == "" {
		httpx.WriteError(w, 400, "invalid_request", "key required")
		return
	}
	hash := store.HashKey(plain)
	if !strings.HasPrefix(plain, "sk-") {
		hash = plain
	}
	k, err := s.DB().GetByHash(hash)
	if err != nil {
		httpx.WriteError(w, 404, "not_found", "key not found")
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"info": Response(*k, "", false)})
}

// Delete removes virtual keys. The plaintext can no longer call inference.
func Delete(s Host, w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	if s.RequireManage(w, r) == nil {
		return
	}
	var body struct {
		Keys []string `json:"keys"`
		Key  string   `json:"key"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	keys := body.Keys
	if body.Key != "" {
		keys = append(keys, body.Key)
	}
	for _, p := range keys {
		hash := store.HashKey(p)
		if !strings.HasPrefix(p, "sk-") {
			hash = p
		}
		_ = s.DB().DeleteHash(hash)
	}
	httpx.WriteJSON(w, 200, map[string]any{"deleted": len(keys)})
}

// Block marks a virtual key blocked.
func Block(s Host, w http.ResponseWriter, r *http.Request) {
	setBlocked(s, w, r, true)
}

// Unblock clears the blocked flag.
func Unblock(s Host, w http.ResponseWriter, r *http.Request) {
	setBlocked(s, w, r, false)
}

// setBlocked sets the blocked flag. A missing key returns 404.
func setBlocked(s Host, w http.ResponseWriter, r *http.Request, blocked bool) {
	httpx.SetCallID(w, httpx.CallID())
	if s.RequireManage(w, r) == nil {
		return
	}
	var body struct {
		Key string `json:"key"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	hash := store.HashKey(body.Key)
	if !strings.HasPrefix(body.Key, "sk-") {
		hash = body.Key
	}
	if err := s.DB().SetBlocked(hash, blocked); err != nil {
		httpx.WriteError(w, 404, "not_found", "key not found")
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"blocked": blocked})
}

// Update changes a virtual key's limits, models, and metadata. The plaintext stays the same.
func Update(s Host, w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	if s.RequireManage(w, r) == nil {
		return
	}
	var body map[string]any
	_ = json.NewDecoder(r.Body).Decode(&body)
	plain := str(body["key"])
	if plain == "" {
		httpx.WriteError(w, 400, "invalid_request", "key required")
		return
	}
	hash := hashKeyToken(plain)
	k, err := s.DB().GetByHash(hash)
	if err != nil {
		httpx.WriteError(w, 404, "not_found", "key not found")
		return
	}
	applyKeyPatch(k, body)
	if err := s.DB().UpdateKey(*k); err != nil {
		httpx.WriteError(w, 400, "invalid_request", err.Error())
		return
	}
	httpx.WriteJSON(w, 200, Response(*k, "", false))
}

// Response is the public JSON for a virtual key. The plaintext is omitted when includePlain is false.
func Response(k store.Key, plain string, includePlain bool) map[string]any {
	created := k.CreatedAt.UTC().Format(time.RFC3339)
	if k.CreatedAt.IsZero() {
		created = time.Now().UTC().Format(time.RFC3339)
	}
	meta := map[string]any{}
	if k.MetadataJSON != "" {
		_ = json.Unmarshal([]byte(k.MetadataJSON), &meta)
	}
	var tags any
	if k.TagsJSON != "" {
		_ = json.Unmarshal([]byte(k.TagsJSON), &tags)
	}
	var expires any
	if k.ExpiresAt.Valid {
		expires = k.ExpiresAt.Time.UTC().Format(time.RFC3339)
	}
	var reset any
	if k.BudgetResetAt.Valid {
		reset = k.BudgetResetAt.Time.UTC().Format(time.RFC3339)
	}
	var dur any
	if k.BudgetDuration != "" {
		dur = k.BudgetDuration
	}
	budgetTable := map[string]any{
		"max_budget":      nullFloatMap(k.MaxBudget),
		"soft_budget":     nullFloatMap(k.SoftBudget),
		"tpm_limit":       nullIntMap(k.TPMLimit),
		"rpm_limit":       nullIntMap(k.RPMLimit),
		"budget_duration": dur,
		"budget_reset_at": reset,
	}
	token := k.TokenHash
	if includePlain {
		token = plain
	}
	m := map[string]any{
		"token_id":               k.TokenHash,
		"token":                  token,
		"key_name":               k.KeyName,
		"key_alias":              k.KeyAlias,
		"user_id":                emptyNil(k.UserID),
		"team_id":                emptyNil(k.TeamID),
		"organization_id":        emptyNil(k.OrganizationID),
		"org_id":                 emptyNil(k.OrganizationID),
		"project_id":             emptyNil(k.ProjectID),
		"agent_id":               emptyNil(k.AgentID),
		"budget_id":              emptyNil(k.BudgetID),
		"models":                 k.Models(),
		"max_budget":             nullFloatMap(k.MaxBudget),
		"soft_budget":            nullFloatMap(k.SoftBudget),
		"spend":                  k.Spend,
		"key_type":               k.KeyType,
		"blocked":                NullBoolJSON(k.Blocked),
		"tpm_limit":              nullIntMap(k.TPMLimit),
		"rpm_limit":              nullIntMap(k.RPMLimit),
		"max_parallel_requests":  nullIntMap(k.MaxParallel),
		"budget_duration":        dur,
		"budget_reset_at":        reset,
		"expires":                expires,
		"metadata":               meta,
		"tags":                   tags,
		"aliases":                map[string]any{},
		"config":                 map[string]any{},
		"permissions":            map[string]any{},
		"allowed_cache_controls": []any{},
		"allowed_routes":         []any{},
		"model_max_budget":       map[string]any{},
		"model_spend":            map[string]any{},
		"created_at":             created,
		"updated_at":             created,
		"created_by":             emptyNil(k.UserID),
		"last_active":            nil,
		"litellm_budget_table":   budgetTable,
	}
	if includePlain {
		m["key"] = plain
	}
	return m
}
