package server

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/sunqirui1987/xhub/internal/httpx"
	"github.com/sunqirui1987/xhub/internal/store"
)

// nullBoolFrom leaves the field unset when the client omitted it.
// LiteLLM stores that as JSON null, not false.
func nullBoolFrom(body map[string]any, key string) sql.NullBool {
	v, ok := body[key]
	if !ok || v == nil {
		return sql.NullBool{}
	}
	return sql.NullBool{Bool: boolOf(v), Valid: true}
}

func nullBoolJSON(v sql.NullBool) any {
	if !v.Valid {
		return nil
	}
	return v.Bool
}

func (s *Server) keyGenerateServiceAccount(w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	if s.requireManage(w, r) == nil {
		return
	}
	body := readMap(r)
	body["user_id"] = ""
	if str(body["key_type"]) == "" {
		body["key_type"] = "llm_api"
	}
	plain := store.NewPlainKey()
	k, err := keyFromBody(plain, body)
	if err != nil {
		httpx.WriteError(w, 400, "invalid_request", err.Error())
		return
	}
	if err := s.Store.InsertKey(k); err != nil {
		httpx.WriteError(w, 500, "internal", err.Error())
		return
	}
	httpx.WriteJSON(w, 200, keyResponse(k, plain, true))
}

func (s *Server) keyRegenerate(w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	if s.requireManage(w, r) == nil {
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
	k, err := s.Store.GetByHash(hashKeyToken(plain))
	if err != nil {
		httpx.WriteError(w, 400, "invalid_request", "key not found")
		return
	}
	applyKeyPatch(k, body)
	newPlain := str(body["new_key"])
	if newPlain == "" {
		newPlain = store.NewPlainKey()
	}
	_ = s.Store.DeleteHash(k.TokenHash)
	k.TokenHash = store.HashKey(newPlain)
	k.KeyName = store.KeyNameFromPlain(newPlain)
	if err := s.Store.InsertKey(*k); err != nil {
		httpx.WriteError(w, 500, "internal", err.Error())
		return
	}
	httpx.WriteJSON(w, 200, keyResponse(*k, newPlain, true))
}

func (s *Server) keyResetSpend(w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	if s.requireManage(w, r) == nil {
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
	k, err := s.Store.GetByHash(hash)
	if err != nil {
		httpx.WriteError(w, 400, "invalid_request", "key not found")
		return
	}
	resetTo := 0.0
	if _, ok := body["reset_to"]; ok {
		resetTo = parseNullFloat(body["reset_to"]).Float64
	}
	if err := s.Store.SetSpend(hash, resetTo); err != nil {
		httpx.WriteError(w, 400, "invalid_request", err.Error())
		return
	}
	k.Spend = resetTo
	httpx.WriteJSON(w, 200, keyResponse(*k, "", false))
}

func (s *Server) keyAliases(w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	if s.requireManage(w, r) == nil {
		return
	}
	keys, _ := s.Store.ListKeys()
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

func (s *Server) keyHealth(w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	if s.requireMixed(w, r) == nil {
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{
		"key":               "healthy",
		"logging_callbacks": nil,
	})
}

func (s *Server) keyBulkUpdate(w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	if s.requireManage(w, r) == nil {
		return
	}
	body := readMap(r)
	keys := idsFrom(body, "keys", "key")
	n := 0
	out := []map[string]any{}
	for _, p := range keys {
		k, err := s.Store.GetByHash(hashKeyToken(p))
		if err != nil {
			continue
		}
		applyKeyPatch(k, body)
		if err := s.Store.UpdateKey(*k); err != nil {
			continue
		}
		n++
		out = append(out, keyResponse(*k, "", false))
	}
	httpx.WriteJSON(w, 200, map[string]any{"updated": n, "keys": out})
}

func keyFromBody(plain string, body map[string]any) (store.Key, error) {
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

func hashKeyToken(plain string) string {
	if strings.HasPrefix(plain, "sk-") {
		return store.HashKey(plain)
	}
	return plain
}
