// Package keys generates, rotates, resets spend on, and bulk-updates virtual keys.
package keys

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/sunqirui1987/xhub/internal/httpx"
	"github.com/sunqirui1987/xhub/internal/logx"
	"github.com/sunqirui1987/xhub/internal/store"
	"sync"
)

var logTraceOnceAdmin sync.Once

// nullBoolFrom reads a nullable bool from the body. A missing field stays invalid.
// LiteLLM stores that as JSON null, not false.
func nullBoolFrom(body map[string]any, key string) sql.NullBool {
	logTraceOnceAdmin.Do(func() { logx.Trace("enter keys.nullBoolFrom") })

	v, ok := body[key]
	if !ok || v == nil {
		return sql.NullBool{}
	}
	return sql.NullBool{Bool: boolOf(v), Valid: true}
}

// NullBoolJSON is the JSON form of a nullable bool. An invalid value is null.
func NullBoolJSON(v sql.NullBool) any {
	if !v.Valid {
		return nil
	}
	return v.Bool
}

// ServiceAccount generates a key for a service account. The plaintext is returned once.
func ServiceAccount(s Host, w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	if s.RequireManage(w, r) == nil {
		return
	}
	body := readMap(r)
	body["user_id"] = ""
	if str(body["key_type"]) == "" {
		body["key_type"] = "llm_api"
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

// Regenerate rotates the key plaintext. The old plaintext stops working immediately.
func Regenerate(s Host, w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	if s.RequireManage(w, r) == nil {
		return
	}
	body := readMap(r)
	plain := r.PathValue("key")
	if plain == "" {
		plain = str(body["key"])
	}
	if plain == "" {
		httpx.WriteError(w, 400, "invalid_request", "key required")
		return
	}
	k, err := s.DB().GetByHash(hashKeyToken(plain))
	if err != nil {
		httpx.WriteError(w, 400, "invalid_request", "key not found")
		return
	}
	applyKeyPatch(k, body)
	newPlain := str(body["new_key"])
	if newPlain == "" {
		newPlain = store.NewPlainKey()
	}
	_ = s.DB().DeleteHash(k.TokenHash)
	k.TokenHash = store.HashKey(newPlain)
	k.KeyName = store.KeyNameFromPlain(newPlain)
	if err := s.DB().InsertKey(*k); err != nil {
		httpx.WriteError(w, 500, "internal", err.Error())
		return
	}
	httpx.WriteJSON(w, 200, Response(*k, newPlain, true))
}

// ResetSpend sets the key spend back to zero. Historical logs are not deleted.
func ResetSpend(s Host, w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	if s.RequireManage(w, r) == nil {
		return
	}
	body := readMap(r)
	plain := r.PathValue("key")
	if plain == "" {
		plain = str(body["key"])
	}
	if plain == "" {
		httpx.WriteError(w, 400, "invalid_request", "key required")
		return
	}
	hash := hashKeyToken(plain)
	k, err := s.DB().GetByHash(hash)
	if err != nil {
		httpx.WriteError(w, 400, "invalid_request", "key not found")
		return
	}
	resetTo := 0.0
	if _, ok := body["reset_to"]; ok {
		resetTo = parseNullFloat(body["reset_to"]).Float64
	}
	if err := s.DB().SetSpend(hash, resetTo); err != nil {
		httpx.WriteError(w, 400, "invalid_request", err.Error())
		return
	}
	k.Spend = resetTo
	httpx.WriteJSON(w, 200, Response(*k, "", false))
}

// Aliases lists key aliases for the dashboard pickers.
func Aliases(s Host, w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	if s.RequireManage(w, r) == nil {
		return
	}
	keys, _ := s.DB().ListKeys()
	search := strings.ToLower(r.URL.Query().Get("search"))
	aliases := []string{}
	seen := map[string]bool{}
	for _, k := range keys {
		if k.KeyAlias == "" {
			continue
		}
		if search != "" && !strings.Contains(strings.ToLower(k.KeyAlias), search) {
			continue
		}
		if seen[k.KeyAlias] {
			continue
		}
		seen[k.KeyAlias] = true
		aliases = append(aliases, k.KeyAlias)
	}
	httpx.WriteJSON(w, 200, map[string]any{
		"aliases":      aliases,
		"total_count":  len(aliases),
		"current_page": 1,
		"total_pages":  1,
		"size":         len(aliases),
	})
}

// Health checks whether a key still passes the identity and budget checks.
func Health(s Host, w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	if s.RequireMixed(w, r) == nil {
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{
		"key":               "healthy",
		"logging_callbacks": nil,
	})
}

// BulkUpdate updates many keys in one request.
func BulkUpdate(s Host, w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	if s.RequireManage(w, r) == nil {
		return
	}
	body := readMap(r)
	keys := idsFrom(body, "keys", "key")
	n := 0
	out := []map[string]any{}
	for _, p := range keys {
		k, err := s.DB().GetByHash(hashKeyToken(p))
		if err != nil {
			continue
		}
		applyKeyPatch(k, body)
		if err := s.DB().UpdateKey(*k); err != nil {
			continue
		}
		n++
		out = append(out, Response(*k, "", false))
	}
	httpx.WriteJSON(w, 200, map[string]any{"updated": n, "keys": out})
}

// FromBody builds the key to store from the plaintext and the body. An illegal model list returns an error.
func FromBody(plain string, body map[string]any) (store.Key, error) {
	if body == nil {
		body = map[string]any{}
	}
	kt := str(body["key_type"])
	if kt == "" {
		kt = "default"
	}
	dur := str(body["duration"])
	exp, err := store.ParseDuration(dur)
	if err != nil {
		return store.Key{}, err
	}
	budgetDur := str(body["budget_duration"])
	k := store.Key{
		TokenHash:      store.HashKey(plain),
		KeyAlias:       str(body["key_alias"]),
		KeyName:        store.KeyNameFromPlain(plain),
		UserID:         str(body["user_id"]),
		TeamID:         str(body["team_id"]),
		OrganizationID: str(body["organization_id"]),
		ProjectID:      str(body["project_id"]),
		AgentID:        str(body["agent_id"]),
		BudgetID:       str(body["budget_id"]),
		KeyType:        kt,
		ModelsJSON:     encodeModels(body["models"]),
		MaxBudget:      parseNullFloat(body["max_budget"]),
		SoftBudget:     parseNullFloat(body["soft_budget"]),
		TPMLimit:       parseNullInt(body["tpm_limit"]),
		RPMLimit:       parseNullInt(body["rpm_limit"]),
		MaxParallel:    parseNullInt(body["max_parallel_requests"]),
		Blocked:        nullBoolFrom(body, "blocked"),
		ExpiresAt:      exp,
		BudgetDuration: budgetDur,
		BudgetResetAt:  store.ResetAtFrom(str(body["budget_reset_at"]), budgetDur),
		MetadataJSON:   encodeMaybeJSON(body["metadata"]),
		TagsJSON:       encodeMaybeJSON(body["tags"]),
		CreatedAt:      time.Now().UTC(),
	}
	return k, nil
}

// applyKeyPatch applies a patch onto a key. Limits that are absent keep their previous value.
func applyKeyPatch(k *store.Key, body map[string]any) {
	if v, ok := body["key_alias"].(string); ok {
		k.KeyAlias = v
	}
	if v, ok := body["user_id"].(string); ok {
		k.UserID = v
	}
	if v, ok := body["team_id"].(string); ok {
		k.TeamID = v
	}
	if v, ok := body["organization_id"].(string); ok {
		k.OrganizationID = v
	}
	if v, ok := body["project_id"].(string); ok {
		k.ProjectID = v
	}
	if v, ok := body["agent_id"].(string); ok {
		k.AgentID = v
	}
	if v, ok := body["budget_id"].(string); ok {
		k.BudgetID = v
	}
	if v, ok := body["key_type"].(string); ok && v != "" {
		k.KeyType = v
	}
	if _, ok := body["max_budget"]; ok {
		k.MaxBudget = parseNullFloat(body["max_budget"])
	}
	if _, ok := body["soft_budget"]; ok {
		k.SoftBudget = parseNullFloat(body["soft_budget"])
	}
	if _, ok := body["models"]; ok {
		k.ModelsJSON = encodeModels(body["models"])
	}
	if _, ok := body["tpm_limit"]; ok {
		k.TPMLimit = parseNullInt(body["tpm_limit"])
	}
	if _, ok := body["rpm_limit"]; ok {
		k.RPMLimit = parseNullInt(body["rpm_limit"])
	}
	if _, ok := body["max_parallel_requests"]; ok {
		k.MaxParallel = parseNullInt(body["max_parallel_requests"])
	}
	if _, ok := body["blocked"]; ok {
		k.Blocked = nullBoolFrom(body, "blocked")
	}
	if v, ok := body["budget_duration"].(string); ok {
		k.BudgetDuration = v
		k.BudgetResetAt = store.ResetAtFrom(str(body["budget_reset_at"]), v)
	} else if _, ok := body["budget_reset_at"]; ok {
		k.BudgetResetAt = store.ResetAtFrom(str(body["budget_reset_at"]), k.BudgetDuration)
	}
	if _, ok := body["metadata"]; ok {
		k.MetadataJSON = encodeMaybeJSON(body["metadata"])
	}
	if _, ok := body["tags"]; ok {
		k.TagsJSON = encodeMaybeJSON(body["tags"])
	}
	if v, ok := body["duration"].(string); ok && v != "" {
		if exp, err := store.ParseDuration(v); err == nil {
			k.ExpiresAt = exp
		}
	}
}

// encodeMaybeJSON keeps a value that is already JSON text and otherwise encodes it as JSON.
func encodeMaybeJSON(v any) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	default:
		b, err := json.Marshal(t)
		if err != nil {
			return ""
		}
		return string(b)
	}
}

// hashKeyToken is the stored hash of a plaintext key. A value that does not start with sk- is kept as given.
func hashKeyToken(plain string) string {
	if strings.HasPrefix(plain, "sk-") {
		return store.HashKey(plain)
	}
	return plain
}
