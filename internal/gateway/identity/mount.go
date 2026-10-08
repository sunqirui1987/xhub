// Package identity owns this module's routes. The process mounts the module by name and does not list these paths in a central table.
package identity

import (
	"net/http"

	"github.com/sunqirui1987/xhub/internal/httpx"
	"github.com/sunqirui1987/xhub/internal/logx"
	"sync"
)

var logTraceOnceMount sync.Once

// Module is users, organizations, teams, members, projects, access groups and the audit trail. The process implements Gate. This package does not import gateway. The paths are the LiteLLM-compatible ones the console already calls, mounted over the new authorization. The routes whose subject does not exist in the new model are gone rather than stubbed: there is no budget object to create, no organization member, no account role beyond the two the matrix has, and no bulk role assignment.
// 参数 h（Gate）：带当前操作者的鉴权守卫。允许时返回 nil，拒绝时返回禁止或未找到。
// 返回 httpx.Module（httpx.Module）：可挂到网关上的模块。
// 调用：gateway/family/mount.go、gateway/guard/mount.go、gateway/keys/mount.go、gateway/models/mount.go
// 测试：无直接单测
func Module(h Gate) httpx.Module {
	logTraceOnceMount.Do(func() { logx.Trace("enter identity.Module") })

	traceModule("identity")
	return httpx.Bind("identity", func(reg httpx.Registrar) {
		// Accounts. Only a platform administrator reaches the store; a member
		// reads other people through a team's member list.
		reg.Handle("POST /user/new", func(w http.ResponseWriter, r *http.Request) { UserNew(h, w, r) })
		reg.Handle("GET /user/list", func(w http.ResponseWriter, r *http.Request) { UserList(h, w, r) })
		reg.Handle("GET /user/filter/ui", func(w http.ResponseWriter, r *http.Request) { UserFilterUI(h, w, r) })
		reg.Handle("GET /user/available_users", func(w http.ResponseWriter, r *http.Request) { AvailableUsers(h, w, r) })
		reg.Handle("GET /user/info", func(w http.ResponseWriter, r *http.Request) { UserInfo(h, w, r) })
		reg.Handle("GET /v2/user/info", func(w http.ResponseWriter, r *http.Request) { UserInfo(h, w, r) })
		reg.Handle("POST /user/update", func(w http.ResponseWriter, r *http.Request) { UserUpdate(h, w, r) })
		reg.Handle("POST /user/delete", func(w http.ResponseWriter, r *http.Request) { UserDelete(h, w, r) })
		// Setting somebody else's password. This replaces the reset link the
		// product used to mint: that link led to an onboarding page with no
		// backend behind it, so the password typed into it went nowhere. A
		// platform administrator reaches every account; a team or organization
		// administrator reaches the people under them, and the decision refuses
		// an account that administers the platform or another organization.
		reg.Handle("POST /user/set_password", func(w http.ResponseWriter, r *http.Request) { UserSetPassword(h, w, r) })
		// The path form, so a caller can name the account in the URL.
		reg.Handle("POST /user/{user_id}/password", func(w http.ResponseWriter, r *http.Request) { UserSetPassword(h, w, r) })

		// Organizations. A member sees the organization that owns their team.
		// An organization administrator runs that organization. Budget and
		// deletion stay with the platform administrator.
		reg.Handle("POST /organization/new", func(w http.ResponseWriter, r *http.Request) { OrgNew(h, w, r) })
		reg.Handle("GET /organization/list", func(w http.ResponseWriter, r *http.Request) { OrgList(h, w, r) })
		reg.Handle("GET /organization/info", func(w http.ResponseWriter, r *http.Request) { OrgInfo(h, w, r) })
		reg.Handle("PATCH /organization/update", func(w http.ResponseWriter, r *http.Request) { OrgUpdate(h, w, r) })
		reg.Handle("DELETE /organization/delete", func(w http.ResponseWriter, r *http.Request) { OrgDelete(h, w, r) })
		reg.Handle("POST /organization/member_add", func(w http.ResponseWriter, r *http.Request) { OrgMemberAdd(h, w, r) })
		reg.Handle("POST /organization/member_delete", func(w http.ResponseWriter, r *http.Request) { OrgMemberRemove(h, w, r) })
		reg.Handle("DELETE /organization/member_delete", func(w http.ResponseWriter, r *http.Request) { OrgMemberRemove(h, w, r) })

		// Teams.
		reg.Handle("POST /team/new", func(w http.ResponseWriter, r *http.Request) { TeamNew(h, w, r) })
		reg.Handle("GET /team/list", func(w http.ResponseWriter, r *http.Request) { TeamList(h, w, r) })
		reg.Handle("GET /v2/team/list", func(w http.ResponseWriter, r *http.Request) { TeamListV2(h, w, r) })
		reg.Handle("GET /team/available", func(w http.ResponseWriter, r *http.Request) { TeamAvailable(h, w, r) })
		reg.Handle("GET /team/info", func(w http.ResponseWriter, r *http.Request) { TeamInfo(h, w, r) })
		reg.Handle("POST /team/update", func(w http.ResponseWriter, r *http.Request) { TeamUpdate(h, w, r) })
		reg.Handle("POST /team/delete", func(w http.ResponseWriter, r *http.Request) { TeamDelete(h, w, r) })
		reg.Handle("POST /team/move", func(w http.ResponseWriter, r *http.Request) { TeamMove(h, w, r) })
		reg.Handle("GET /team/models", func(w http.ResponseWriter, r *http.Request) { TeamModels(h, w, r) })

		// Members.
		reg.Handle("GET /team/member_list", func(w http.ResponseWriter, r *http.Request) { TeamMembers(h, w, r) })
		reg.Handle("POST /team/member_add", func(w http.ResponseWriter, r *http.Request) { TeamMemberAdd(h, w, r) })
		reg.Handle("POST /team/member_update", func(w http.ResponseWriter, r *http.Request) { TeamMemberUpdate(h, w, r) })
		reg.Handle("POST /team/member_delete", func(w http.ResponseWriter, r *http.Request) { TeamMemberRemove(h, w, r) })

		// Projects.
		reg.Handle("POST /project/new", func(w http.ResponseWriter, r *http.Request) { ProjectNew(h, w, r) })
		reg.Handle("GET /project/list", func(w http.ResponseWriter, r *http.Request) { ProjectList(h, w, r) })
		reg.Handle("POST /project/list", func(w http.ResponseWriter, r *http.Request) { ProjectList(h, w, r) })
		reg.Handle("GET /project/info", func(w http.ResponseWriter, r *http.Request) { ProjectInfo(h, w, r) })
		reg.Handle("POST /project/info", func(w http.ResponseWriter, r *http.Request) { ProjectInfo(h, w, r) })
		reg.Handle("POST /project/update", func(w http.ResponseWriter, r *http.Request) { ProjectUpdate(h, w, r) })
		reg.Handle("POST /project/delete", func(w http.ResponseWriter, r *http.Request) { ProjectDelete(h, w, r) })

		// Named router settings templates. Editing one is the platform
		// administrator's; selecting one on an organization, a team or a key is
		// that scope's own administrator's, decided per object inside the
		// handler.
		reg.Handle("GET /route_template/list", func(w http.ResponseWriter, r *http.Request) { RouteTemplateList(h, w, r) })
		reg.Handle("POST /route_template/new", func(w http.ResponseWriter, r *http.Request) { RouteTemplateCreate(h, w, r) })
		reg.Handle("GET /route_template/{template_id}", func(w http.ResponseWriter, r *http.Request) { RouteTemplateGet(h, w, r) })
		reg.Handle("POST /route_template/{template_id}/update", func(w http.ResponseWriter, r *http.Request) { RouteTemplateUpdate(h, w, r) })
		reg.Handle("POST /route_template/{template_id}/delete", func(w http.ResponseWriter, r *http.Request) { RouteTemplateDelete(h, w, r) })
		reg.Handle("GET /route_template/{template_id}/usage", func(w http.ResponseWriter, r *http.Request) { RouteTemplateUsage(h, w, r) })
		// One binding endpoint for all three scopes: the scope is in the body,
		// because the difference between them is one name and one id.
		reg.Handle("GET /route_template/binding", func(w http.ResponseWriter, r *http.Request) { RouteTemplateBinding(h, w, r) })
		reg.Handle("POST /route_template/binding", func(w http.ResponseWriter, r *http.Request) { RouteTemplateBinding(h, w, r) })

		// Audit.
		reg.Handle("GET /audit/logs", func(w http.ResponseWriter, r *http.Request) { AuditLog(h, w, r) })
	})
}
