// Package keys creates, lists, updates, and deletes virtual keys. The plaintext appears only in the create or rotate response.
package keys

import (
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/sunqirui1987/xhub/internal/auth"
	"github.com/sunqirui1987/xhub/internal/authz"
	"github.com/sunqirui1987/xhub/internal/httpx"
	"github.com/sunqirui1987/xhub/internal/iam"
	"github.com/sunqirui1987/xhub/internal/logx"
)

var logTraceOnceGenerate sync.Once

// Generate creates a virtual key and returns the plaintext only in this
// response. A member may mint a personal key for themselves inside a team they
// belong to; a service key needs the team's administration, and the decision is
// made by Authorize rather than here.
func Generate(s Host, w http.ResponseWriter, r *http.Request) {
	logTraceOnceGenerate.Do(func() { logx.Trace("enter keys.Generate") })

	httpx.SetCallID(w, httpx.CallID())
	p := s.RequireUser(w, r)
	if p == nil {
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
	in, err := keyInputFrom(body, p.UserID)
	if err != nil {
		httpx.WriteError(w, 400, "invalid_request", err.Error())
		return
	}
	obj := authz.Object{Type: authz.ObjectKey, TeamID: in.TeamID, ProjectID: in.ProjectID,
		OwnerType: in.OwnerType, OwnerUserID: in.UserID}
	if err := s.Authorize(r, p, authz.ActionKeyCreate, obj); err != nil {
		s.WriteAuthz(w, r, err)
		return
	}
	k, plain, err := s.Identity().CreateKey(r.Context(), actorOf(p), in)
	if err != nil {
		s.WriteIAMError(w, r, err)
		return
	}
	httpx.WriteJSON(w, 200, Response(*k, plain, true))
}

// List lists the keys the caller may see and never returns plaintext. The rows
// are narrowed in SQL by the scope, so a handler bug cannot widen the listing.
func List(s Host, w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	p := s.RequireUser(w, r)
	if p == nil {
		return
	}
	sc, err := s.KeysScope(r, p)
	if err != nil {
		s.WriteAuthz(w, r, err)
		return
	}
	rows, err := s.Identity().ListKeys(r.Context(), keyFilter(sc))
	if err != nil {
		s.WriteIAMError(w, r, err)
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, k := range rows {
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
	p := s.RequireUser(w, r)
	if p == nil {
		return
	}
	token := r.URL.Query().Get("key")
	if r.Method == http.MethodPost {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if v, ok := body["key"].(string); ok {
			token = v
		}
	}
	if token == "" {
		httpx.WriteError(w, 400, "invalid_request", "key required")
		return
	}
	k, err := lookupKey(s, r, token)
	if err != nil {
		s.WriteIAMError(w, r, err)
		return
	}
	// The guard re-reads the row, so a key the caller may not see is not found
	// rather than forbidden: existence itself is not disclosed.
	if err := s.Authorize(r, p, authz.ActionKeyRead, authz.Object{Type: authz.ObjectKey, ID: k.ID}); err != nil {
		s.WriteAuthz(w, r, err)
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"info": Response(*k, "", false)})
}

// Delete removes virtual keys. The plaintext can no longer call inference.
func Delete(s Host, w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	p := s.RequireUser(w, r)
	if p == nil {
		return
	}
	ids, ok := authorizedKeys(s, w, r, p, authz.ActionKeyDelete)
	if !ok {
		return
	}
	n := 0
	for _, id := range ids {
		if err := s.Identity().DeleteKey(r.Context(), actorOf(p), id); err != nil {
			s.WriteIAMError(w, r, err)
			return
		}
		n++
	}
	httpx.WriteJSON(w, 200, map[string]any{"deleted": n})
}

// Block marks a virtual key blocked.
func Block(s Host, w http.ResponseWriter, r *http.Request) {
	setBlocked(s, w, r, true)
}

// Unblock clears the blocked flag.
func Unblock(s Host, w http.ResponseWriter, r *http.Request) {
	setBlocked(s, w, r, false)
}

// setBlocked sets or clears the blocked status. A key the caller may not write
// returns 404.
func setBlocked(s Host, w http.ResponseWriter, r *http.Request, blocked bool) {
	httpx.SetCallID(w, httpx.CallID())
	p := s.RequireUser(w, r)
	if p == nil {
		return
	}
	var body struct {
		Key string `json:"key"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body.Key == "" {
		httpx.WriteError(w, 400, "invalid_request", "key required")
		return
	}
	k, err := lookupKey(s, r, body.Key)
	if err != nil {
		s.WriteIAMError(w, r, err)
		return
	}
	if err := s.Authorize(r, p, authz.ActionKeyWrite, authz.Object{Type: authz.ObjectKey, ID: k.ID}); err != nil {
		s.WriteAuthz(w, r, err)
		return
	}
	status := iam.StatusActive
	if blocked {
		status = iam.StatusBlocked
	}
	if _, err := s.Identity().SetKeyStatus(r.Context(), actorOf(p), k.ID, status); err != nil {
		s.WriteIAMError(w, r, err)
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"blocked": blocked})
}

// Update changes a virtual key's name, narrowing, and limits. The plaintext
// stays the same, and a field the caller left out keeps its value.
func Update(s Host, w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	p := s.RequireUser(w, r)
	if p == nil {
		return
	}
	var body map[string]any
	_ = json.NewDecoder(r.Body).Decode(&body)
	token := str(body["key"])
	if token == "" {
		httpx.WriteError(w, 400, "invalid_request", "key required")
		return
	}
	k, err := lookupKey(s, r, token)
	if err != nil {
		s.WriteIAMError(w, r, err)
		return
	}
	if err := s.Authorize(r, p, authz.ActionKeyWrite, authz.Object{Type: authz.ObjectKey, ID: k.ID}); err != nil {
		s.WriteAuthz(w, r, err)
		return
	}
	out, err := s.Identity().UpdateKey(r.Context(), actorOf(p), k.ID, patchFrom(k, body))
	if err != nil {
		s.WriteIAMError(w, r, err)
		return
	}
	httpx.WriteJSON(w, 200, Response(*out, "", false))
}

// Response is the public JSON for a virtual key. The plaintext is omitted when
// includePlain is false; the stored hash never appears, because the hash is what
// authentication compares and publishing it would leak the credential.
func Response(k iam.Key, plain string, includePlain bool) map[string]any {
	created := k.CreatedAt.UTC().Format(time.RFC3339)
	if k.CreatedAt.IsZero() {
		created = time.Now().UTC().Format(time.RFC3339)
	}
	updated := created
	if !k.UpdatedAt.IsZero() {
		updated = k.UpdatedAt.UTC().Format(time.RFC3339)
	}
	var expires any
	if k.ExpiresAt != nil {
		expires = k.ExpiresAt.UTC().Format(time.RFC3339)
	}
	var lastActive any
	if k.LastUsedAt != nil {
		lastActive = k.LastUsedAt.UTC().Format(time.RFC3339)
	}
	token := k.KeyPrefix + "..."
	if includePlain {
		token = plain
	}
	m := map[string]any{
		"token_id":         k.ID,
		"token":            token,
		"key_name":         k.Name,
		"key_alias":        k.Name,
		"key_prefix":       k.KeyPrefix,
		"owner_type":       k.OwnerType,
		"user_id":          emptyNil(deref(k.UserID)),
		"team_id":          emptyNil(k.TeamID),
		"project_id":       emptyNil(deref(k.ProjectID)),
		"created_by":       emptyNil(deref(k.CreatedBy)),
		"models":           nonNilStrings(k.Models),
		"max_budget":       floatJSON(k.MaxBudget),
		"spend":            k.Spend,
		"tpm_limit":        intJSON(k.TPMLimit),
		"rpm_limit":        intJSON(k.RPMLimit),
		"status":           k.Status,
		"blocked":          k.Status == iam.StatusBlocked,
		"expires":          expires,
		"last_active":      lastActive,
		"created_at":       created,
		"updated_at":       updated,
	}
	if includePlain {
		m["key"] = plain
	}
	return m
}

// actionKeys resolves the key tokens in a body and returns the IDs the caller is
// permitted to act on. A token that names nothing the caller may see, or one
// that is refused, has already produced the response and returns ok false.
func authorizedKeys(s Host, w http.ResponseWriter, r *http.Request, p *auth.Principal, action authz.Action) ([]string, bool) {
	var body struct {
		Keys []string `json:"keys"`
		Key  string   `json:"key"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	tokens := body.Keys
	if body.Key != "" {
		tokens = append(tokens, body.Key)
	}
	if len(tokens) == 0 {
		httpx.WriteError(w, 400, "invalid_request", "key required")
		return nil, false
	}
	ids := make([]string, 0, len(tokens))
	for _, token := range tokens {
		k, err := lookupKey(s, r, token)
		if err != nil {
			s.WriteIAMError(w, r, err)
			return nil, false
		}
		if err := s.Authorize(r, p, action, authz.Object{Type: authz.ObjectKey, ID: k.ID}); err != nil {
			s.WriteAuthz(w, r, err)
			return nil, false
		}
		ids = append(ids, k.ID)
	}
	return ids, true
}

// lookupKey finds the key a token names. The token may be a plaintext
// credential, the stored hash, or the key ID, because the dashboard carries the
// ID while the LiteLLM-compatible clients carry the plaintext.
func lookupKey(s Host, r *http.Request, token string) (*iam.Key, error) {
	ctx := r.Context()
	if k, err := s.Identity().KeyByHash(ctx, hashKeyToken(token)); err == nil {
		return k, nil
	} else if err != iam.ErrNotFound {
		return nil, err
	}
	return s.Identity().GetKey(ctx, token)
}

// keyFilter turns a key-listing scope into the store's filter. Every branch is
// fail-closed: a scope that names no row shape matches nothing rather than
// everything.
func keyFilter(sc *authz.Scope) iam.KeyFilter {
	switch sc.Kind {
	case authz.ScopeAllKeys:
		return iam.KeyFilter{}
	case authz.ScopeTeamKeys:
		return iam.KeyFilter{OwnOrService: true, UserID: sc.UserID, TeamIDs: sc.TeamIDs}
	default:
		if len(sc.KeyIDs) > 0 {
			return iam.KeyFilter{KeyIDs: sc.KeyIDs}
		}
		if sc.UserID == "" {
			return iam.KeyFilter{KeyIDs: []string{}}
		}
		return iam.KeyFilter{OwnerType: iam.OwnerPersonal, UserID: sc.UserID}
	}
}

// keyInputFrom reads a new key from a request body. The owner default is a
// personal key for the caller, which is what a member minting their own key
// means; a service key must be asked for by name.
func keyInputFrom(body map[string]any, defaultUserID string) (iam.KeyInput, error) {
	in := iam.KeyInput{
		OwnerType:      str(body["owner_type"]),
		UserID:         str(body["user_id"]),
		TeamID:         str(body["team_id"]),
		ProjectID:      str(body["project_id"]),
		Name:           str(body["key_alias"]),
		Models:         stringList(body["models"]),
		MaxBudget:      parseFloat(body["max_budget"]),
		TPMLimit:       parseInt(body["tpm_limit"]),
		RPMLimit:       parseInt(body["rpm_limit"]),
	}
	if in.Name == "" {
		in.Name = str(body["key_name"])
	}
	if in.OwnerType == "" {
		in.OwnerType = iam.OwnerPersonal
		if in.UserID == "" {
			in.UserID = defaultUserID
		}
	}
	if in.OwnerType == iam.OwnerPersonal && in.UserID == "" {
		return in, errUserRequired
	}
	if in.OwnerType == iam.OwnerService {
		in.UserID = ""
	}
	if in.TeamID == "" {
		return in, errTeamRequired
	}
	exp, err := iam.ParseExpiry(str(body["duration"]))
	if err != nil {
		return in, err
	}
	in.ExpiresAt = exp
	return in, nil
}

// patchFrom applies a partial body onto the stored key, so a field the caller
// left out keeps its value instead of being written as empty.
func patchFrom(cur *iam.Key, body map[string]any) iam.KeyInput {
	in := iam.KeyInput{Name: cur.Name, Models: cur.Models,
		MaxBudget: cur.MaxBudget, TPMLimit: cur.TPMLimit, RPMLimit: cur.RPMLimit}
	if v, ok := body["key_alias"].(string); ok {
		in.Name = v
	}
	if _, ok := body["models"]; ok {
		in.Models = stringList(body["models"])
	}
	if _, ok := body["max_budget"]; ok {
		in.MaxBudget = parseFloat(body["max_budget"])
	}
	if _, ok := body["tpm_limit"]; ok {
		in.TPMLimit = parseInt(body["tpm_limit"])
	}
	if _, ok := body["rpm_limit"]; ok {
		in.RPMLimit = parseInt(body["rpm_limit"])
	}
	if _, ok := body["duration"]; ok {
		if exp, err := iam.ParseExpiry(str(body["duration"])); err == nil {
			in.ExpiresAt = exp
		}
	}
	return in
}

// actorOf is the audit actor for a key-level write.
func actorOf(p *auth.Principal) iam.Actor {
	return iam.Actor{ID: p.UserID, Kind: string(p.Kind)}
}

// nonNilStrings keeps the JSON list shape stable: a key with no narrowing emits
// an empty list rather than null.
func nonNilStrings(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
}

var (
	errUserRequired = errString("user_id required for a personal key")
	errTeamRequired = errString("team_id required")
)

// errString is a plain request error, so a bad body is a 400 and not a 500.
type errString string

func (e errString) Error() string { return string(e) }
