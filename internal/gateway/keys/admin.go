// Package keys generates, rotates, resets spend on, and bulk-updates virtual keys.
package keys

import (
	"net/http"
	"strings"
	"sync"

	"github.com/sunqirui1987/xhub/internal/authz"
	"github.com/sunqirui1987/xhub/internal/gateway/templateauth"
	"github.com/sunqirui1987/xhub/internal/httpx"
	"github.com/sunqirui1987/xhub/internal/iam"
	"github.com/sunqirui1987/xhub/internal/logx"
)

var logTraceOnceAdmin sync.Once

// ServiceAccount generates a service key for a team or one of its projects. It has no owner, so it requires the team's administration rather than mere membership; Authorize makes that decision.
// 参数 s（Host）：ServiceAccount使用的数据面宿主；w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；r（*http.Request）：入站 HTTP 请求。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：gateway/keys/mount.go
// 测试：无直接单测
func ServiceAccount(s Host, w http.ResponseWriter, r *http.Request) {
	logTraceOnceAdmin.Do(func() { logx.Trace("enter keys.ServiceAccount") })

	httpx.SetCallID(w, httpx.CallID())
	p := s.RequireUser(w, r)
	if p == nil {
		return
	}
	body := readMap(r)
	body["owner_type"] = iam.OwnerService
	body["user_id"] = ""
	in, err := keyInputFrom(body, p.UserID)
	if err != nil {
		httpx.WriteError(w, 400, "invalid_request", err.Error())
		return
	}
	obj := authz.Object{Type: authz.ObjectKey, TeamID: in.TeamID, ProjectID: in.ProjectID,
		OwnerType: iam.OwnerService}
	if err := s.Authorize(r, p, authz.ActionKeyCreate, obj); err != nil {
		s.WriteAuthz(w, r, err)
		return
	}
	if err := templateauth.Selection(s, r, p, str(body["route_template_id"])); err != nil {
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

// Regenerate rotates the key plaintext. The old plaintext stops working immediately, and the key's own limits and narrowing are untouched.
// 参数 s（Host）：Regenerate使用的数据面宿主；w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；r（*http.Request）：入站 HTTP 请求。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：gateway/keys/mount.go
// 测试：无直接单测
func Regenerate(s Host, w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	p := s.RequireUser(w, r)
	if p == nil {
		return
	}
	body := readMap(r)
	token := r.PathValue("key")
	if token == "" {
		token = str(body["key"])
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
	if err := s.Authorize(r, p, authz.ActionKeyWrite, authz.Object{Type: authz.ObjectKey, ID: k.ID}); err != nil {
		s.WriteAuthz(w, r, err)
		return
	}
	out, plain, err := s.Identity().RotateKey(r.Context(), actorOf(p), k.ID)
	if err != nil {
		s.WriteIAMError(w, r, err)
		return
	}
	httpx.WriteJSON(w, 200, Response(*out, plain, true))
}

// ResetSpend sets the key spend back to zero, or to reset_to when the body carries one. Historical usage rows are not deleted, so the figures that produced the old total remain.
// 参数 s（Host）：Reset花费使用的数据面宿主；w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；r（*http.Request）：入站 HTTP 请求。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：gateway/keys/mount.go
// 测试：无直接单测
func ResetSpend(s Host, w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	p := s.RequireUser(w, r)
	if p == nil {
		return
	}
	body := readMap(r)
	token := r.PathValue("key")
	if token == "" {
		token = str(body["key"])
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
	if err := s.Authorize(r, p, authz.ActionKeyWrite, authz.Object{Type: authz.ObjectKey, ID: k.ID}); err != nil {
		s.WriteAuthz(w, r, err)
		return
	}
	to := 0.0
	if v := parseFloat(body["reset_to"]); v != nil {
		to = *v
	}
	out, err := s.Identity().ResetKeySpend(r.Context(), actorOf(p), k.ID, to)
	if err != nil {
		s.WriteIAMError(w, r, err)
		return
	}
	httpx.WriteJSON(w, 200, Response(*out, "", false))
}

// Aliases lists the key names the caller may see, for the dashboard pickers.
// 参数 s（Host）：Aliases使用的数据面宿主；w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；r（*http.Request）：入站 HTTP 请求。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：gateway/keys/mount.go
// 测试：无直接单测
func Aliases(s Host, w http.ResponseWriter, r *http.Request) {
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
	search := strings.ToLower(r.URL.Query().Get("search"))
	aliases := []string{}
	seen := map[string]bool{}
	for _, k := range rows {
		if k.Name == "" || seen[k.Name] {
			continue
		}
		if search != "" && !strings.Contains(strings.ToLower(k.Name), search) {
			continue
		}
		seen[k.Name] = true
		aliases = append(aliases, k.Name)
	}
	httpx.WriteJSON(w, 200, map[string]any{
		"aliases":      aliases,
		"total_count":  len(aliases),
		"current_page": 1,
		"total_pages":  1,
		"size":         len(aliases),
	})
}

// Health checks that a credential still passes identification. It is the liveness probe the LiteLLM clients call before their first request.
// 参数 s（Host）：Health使用的数据面宿主；w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；r（*http.Request）：入站 HTTP 请求。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：gateway/keys/mount.go
// 测试：无直接单测
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

// BulkUpdate applies one patch to many keys. A key the caller may not write is skipped rather than failing the batch, and updated is the count that landed.
// 参数 s（Host）：BulkUpdate使用的数据面宿主；w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；r（*http.Request）：入站 HTTP 请求。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：gateway/keys/mount.go
// 测试：无直接单测
func BulkUpdate(s Host, w http.ResponseWriter, r *http.Request) {
	httpx.SetCallID(w, httpx.CallID())
	p := s.RequireUser(w, r)
	if p == nil {
		return
	}
	body := readMap(r)
	tokens := idsFrom(body, "keys", "key")
	if err := templateauth.Selection(s, r, p, str(body["route_template_id"])); err != nil {
		s.WriteAuthz(w, r, err)
		return
	}
	n := 0
	out := []map[string]any{}
	for _, token := range tokens {
		k, err := lookupKey(s, r, token)
		if err != nil {
			continue
		}
		if err := s.Authorize(r, p, authz.ActionKeyWrite, authz.Object{Type: authz.ObjectKey, ID: k.ID}); err != nil {
			continue
		}
		updated, err := s.Identity().UpdateKey(r.Context(), actorOf(p), k.ID, patchFrom(k, body))
		if err != nil {
			continue
		}
		n++
		out = append(out, Response(*updated, "", false))
	}
	httpx.WriteJSON(w, 200, map[string]any{"updated": n, "keys": out})
}

// hashKeyToken is the stored hash of a plaintext key. A value that is already a hash or an id is passed through, so one lookup serves both client styles.
// 参数 token（string）：签名或调用方带来的令牌。不会写入响应正文。
// 返回 string（string）：以 sk- 开头时是存库用的哈希，已经是哈希或 id 时原样返回。
// 调用：gateway/keys/generate.go
// 测试：无直接单测
func hashKeyToken(token string) string {
	if strings.HasPrefix(token, "sk-") {
		return iam.HashKey(token)
	}
	return token
}
