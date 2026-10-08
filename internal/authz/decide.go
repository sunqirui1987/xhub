package authz

import (
	"context"
	"errors"

	"github.com/sunqirui1987/xhub/internal/iam"
	"github.com/sunqirui1987/xhub/internal/logx"
	"xorm.io/builder"
)

// Action is a verb an actor asks to perform.
type Action string

const (
	// Account management.
	ActionUserRead   Action = "user.read"
	ActionUserWrite  Action = "user.write"
	ActionUserCreate Action = "user.create"
	ActionUserDelete Action = "user.delete"
	// ActionUserPassword is setting somebody else's password. It is separate
	// from ActionUserWrite because the people who hold it are not the same: a
	// team administrator resets a member's password without gaining the power
	// to change a role or a budget.
	ActionUserPassword Action = "user.password"

	// Organizations.
	ActionOrgRead  Action = "org.read"
	ActionOrgWrite Action = "org.write"
	// ActionOrgAdmin is organization administration: the roster and the name.
	// Budget and deletion stay ActionOrgWrite, which only a platform
	// administrator holds.
	ActionOrgAdmin Action = "org.admin"

	// Teams.
	ActionTeamRead    Action = "team.read"
	ActionTeamWrite   Action = "team.write"
	ActionTeamCreate  Action = "team.create"
	ActionTeamDelete  Action = "team.delete"
	ActionTeamMove    Action = "team.move"
	ActionTeamBudget  Action = "team.budget"
	ActionTeamModels  Action = "team.models"
	ActionTeamService Action = "team.service_keys"

	// Members.
	ActionMemberRead   Action = "member.read"
	ActionMemberWrite  Action = "member.write"
	ActionMemberRemove Action = "member.remove"

	// Projects.
	ActionProjectRead  Action = "project.read"
	ActionProjectWrite Action = "project.write"

	// Keys.
	ActionKeyRead    Action = "key.read"
	ActionKeyCreate  Action = "key.create"
	ActionKeyWrite   Action = "key.write"
	ActionKeyDelete  Action = "key.delete"
	ActionKeyInspect Action = "key.inspect"

	// Router templates. Selecting one is deliberately its own action rather than
	// reusing team.write: a member may choose which settings their own team
	// routes by, which is a smaller thing than changing the team's budget or its
	// model ceiling, and folding the two together would tie their tightness
	// levels to each other.
	ActionRouteTemplateRead   Action = "route_template.read"
	ActionRouteTemplateWrite  Action = "route_template.write"
	ActionRouteTemplateSelect Action = "route_template.select"

	// Usage, logs and audit.
	ActionUsageRead Action = "usage.read"
	ActionLogRead   Action = "log.read"
	ActionAuditRead Action = "audit.read"

	// Inference.
	ActionInfer Action = "infer"
)

// Authorize decides one action against one object. It returns nil to allow, or ErrUnauthenticated, Err Forbidden, ErrNotFound, or an *InternalError. Ownership is read from the database, never from the caller: an object whose ID is given has its org/team/project and owner resolved here, so a handler cannot widen its own reach by passing a team or owner that is not the stored one.
// 参数 ctx（context.Context）：上下文，取消时停止；g（*Guard）：带当前操作者的鉴权守卫。允许时返回 nil，拒绝时返回禁止或未找到；action（Action）：要判定的动作，例如读日志、管理密钥或发起推理；obj（Object）：要判定的对象，含类型、团队、组织和归属用户。
// 返回 error（error）：失败原因，nil 表示这一步成功。
// 调用：gateway/identity/gate.go、gateway/identity/handlers.go、gateway/identity/members.go、gateway/keys/admin.go
// 测试：authz_test.go
func (z *Authorizer) Authorize(ctx context.Context, g *Guard, action Action, obj Object) error {
	if g == nil {
		return ErrUnauthenticated
	}
	return g.authorize(ctx, action, obj)
}

// 按操作者种类选择判定函数。平台、密钥和用户走不同规则。
// 参数 ctx（context.Context）：上下文，取消时停止；action（Action）：要判定的动作，例如读日志、管理密钥或发起推理；obj（Object）：要判定的对象，含类型、团队、组织和归属用户。
// 返回 error（error）：失败原因，nil 表示这一步成功。
// 调用：仅在 decide.go 内使用
// 测试：无直接单测
func (g *Guard) authorize(ctx context.Context, action Action, obj Object) error {
	a := g.actor
	switch a.Kind {
	case KindMaster:
		// The master credential reaches only the narrow routes the router
		// mounts. Within them it is not a platform administrator session.
		return g.masterOnly(action, obj)
	case KindKey:
		return g.keyOnly(ctx, action, obj)
	case KindSession:
		// fall through to the session matrix below
	default:
		return ErrUnauthenticated
	}

	if a.UserID == "" {
		return ErrUnauthenticated
	}
	obj, err := g.resolve(ctx, obj)
	if err != nil {
		return err
	}
	// A blocked team or organization takes effect on the very next request. Only
	// a platform administrator may still act, and only to lift the block.
	if obj.blocked() && !g.PlatformAdmin() {
		return ErrForbidden
	}
	if err := g.decide(ctx, action, obj); err != nil {
		return err
	}
	// Reading somebody else's log content as a platform administrator is a
	// privileged act and the plan requires it to leave a trail.
	if action == ActionLogRead && g.PlatformAdmin() && obj.OwnerType == iam.OwnerPersonal &&
		obj.OwnerUserID != "" && obj.OwnerUserID != a.UserID {
		if err := g.z.db.RecordAudit(ctx, iam.Actor{ID: a.UserID, Kind: string(KindSession)}, iam.Audit{
			Action: "log.read_content", ObjectType: "log", ObjectID: obj.ID, TeamID: obj.TeamID,
			Detail: map[string]any{"owner_user_id": obj.OwnerUserID},
		}); err != nil {
			// The read was authorized but could not be recorded; refusing it is
			// the only way a privileged read is never unattributable.
			logx.Error("authz log-content audit write failed key_owner=%s err=%v", obj.OwnerUserID, err)
			return &InternalError{Err: err}
		}
	}
	return nil
}

// masterOnly is the master credential's very small matrix: bootstrapping and emergency reads. It may not log into the UI, may not infer, and may not act inside a team as a member.
// 参数 action（Action）：要判定的动作，例如读日志、管理密钥或发起推理；obj（Object）：要判定的对象，含类型、团队、组织和归属用户。
// 返回 error（error）：失败原因，nil 表示这一步成功。
// 调用：仅在 decide.go 内使用
// 测试：无直接单测
func (g *Guard) masterOnly(action Action, obj Object) error {
	switch action {
	case ActionUserRead, ActionAuditRead:
		return nil
	default:
		logx.Error("authz master denied action=%s", action)
		return ErrForbidden
	}
}

// keyOnly is a virtual key's matrix. A key may infer through itself and read itself; it never carries management authority, whoever owns it.
// 参数 ctx（context.Context）：上下文，取消时停止；action（Action）：要判定的动作，例如读日志、管理密钥或发起推理；obj（Object）：要判定的对象，含类型、团队、组织和归属用户。
// 返回 error（error）：失败原因，nil 表示这一步成功。
// 调用：仅在 decide.go 内使用
// 测试：无直接单测
func (g *Guard) keyOnly(ctx context.Context, action Action, obj Object) error {
	a := g.actor
	if a.KeyID == "" {
		return ErrUnauthenticated
	}
	// A key is re-validated on every request: a disabled owner, a membership
	// that was removed, a revoked key, and a blocked team or project all take
	// effect on the very next call rather than when the key was minted.
	if err := g.CheckKey(ctx); err != nil {
		return err
	}
	switch action {
	case ActionInfer:
		if a.KeyTeamID == "" {
			return ErrForbidden
		}
		return nil
	case ActionKeyRead:
		// A key may only read its own row.
		if obj.ID != "" && obj.ID != a.KeyID {
			return ErrNotFound
		}
		return nil
	case ActionUsageRead, ActionLogRead:
		// Scoped by the key in the query filter, never by a caller-supplied id.
		return nil
	default:
		logx.Error("authz key denied action=%s key=%s", action, a.KeyID)
		return ErrForbidden
	}
}

// CheckKey re-reads the live state behind a virtual key: the key row itself, the team and project it is bound to, and, for a personal key, that its owner is still an active account and still a member of the team. Authentication calls this too so there is exactly one definition of "this key still works".
// 参数 ctx（context.Context）：上下文，取消或超时时停止后续工作。
// 返回 error（error）：失败原因，nil 表示这一步成功。
// 调用：gateway/session.go
// 测试：无直接单测
func (g *Guard) CheckKey(ctx context.Context) error {
	db := g.z.db
	if db == nil || g.actor.KeyID == "" {
		return nil
	}
	k, err := db.GetKey(ctx, g.actor.KeyID)
	if err != nil {
		return notFoundOrInternal(err)
	}
	if !k.ActiveKey() {
		return ErrForbidden
	}
	t, err := db.GetTeam(ctx, k.TeamID)
	if err != nil {
		return notFoundOrInternal(err)
	}
	if t.Status != iam.StatusActive {
		return ErrForbidden
	}
	if o, err := db.GetOrg(ctx, t.OrganizationID); err == nil {
		if o.Status == iam.StatusBlocked {
			return ErrForbidden
		}
	} else if err != iam.ErrNotFound {
		return notFoundOrInternal(err)
	}
	if k.ProjectID != nil && *k.ProjectID != "" {
		p, err := db.GetProject(ctx, *k.ProjectID)
		if err != nil {
			return notFoundOrInternal(err)
		}
		if p.Status != iam.StatusActive {
			return ErrForbidden
		}
	}
	if k.OwnerType != iam.OwnerPersonal {
		return nil
	}
	if k.UserID == nil || *k.UserID == "" {
		return ErrForbidden
	}
	u, err := db.GetUser(ctx, *k.UserID)
	if err != nil {
		return notFoundOrInternal(err)
	}
	if !u.Active() {
		return ErrForbidden
	}
	// A personal key dies with the membership it was created under.
	ms, err := db.MemberTeams(ctx, *k.UserID)
	if err != nil {
		return &InternalError{Err: err}
	}
	for _, m := range ms {
		if m.TeamID == k.TeamID {
			return nil
		}
	}
	return ErrForbidden
}

// resolve 从数据库补上对象的归属。判定矩阵只用团队、组织和状态。对象不存在时返回 ErrNotFound。创建请求还没有行，就按调用方已经给出的团队和归属类型判定。
// 参数 ctx（context.Context）：上下文，取消时停止；obj（Object）：要判定的对象，含类型、团队、组织和归属用户。
// 返回 Object（Object）：补上团队状态和组织之后的对象。创建请求没有行时原样返回；error（error）：对象不存在或库读失败。nil 表示补全成功。
// 调用：authorize 在进入判定矩阵之前。
// 测试：无直接单测
func (g *Guard) resolve(ctx context.Context, obj Object) (Object, error) {
	if obj.ID == "" && obj.TeamID == "" && obj.OrgID == "" && obj.ProjectID == "" {
		return obj, nil
	}
	if !obj.resolvable() {
		// A create names a team and has no row yet. Load that team so the
		// decision can see which organization it belongs to. An organization
		// administrator is otherwise invisible on every create.
		if obj.TeamID != "" && g.z != nil && g.z.db != nil {
			t, err := g.z.db.GetTeam(ctx, obj.TeamID)
			if err != nil {
				return Object{}, notFoundOrInternal(err)
			}
			obj.TeamStatus = t.Status
			if obj.OrgID == "" {
				obj.OrgID = t.OrganizationID
			}
			if err := g.fillOrg(ctx, &obj); err != nil {
				return Object{}, err
			}
		}
		return obj, nil
	}
	key := string(obj.Type) + ":" + obj.ID
	if obj.ID != "" {
		if hit, ok := g.resolved[key]; ok {
			return hit, nil
		}
	}
	out, err := g.lookup(ctx, obj)
	if err != nil {
		return Object{}, err
	}
	if obj.ID != "" {
		g.resolved[key] = out
	}
	return out, nil
}

// 按对象类型把团队、密钥或日志补全成带归属的对象，供后面的判定使用。
// 参数 ctx（context.Context）：上下文，取消时停止；obj（Object）：要判定的对象，含类型、团队、组织和归属用户。
// 返回 Object（Object）：按对象类型把团队、密钥或日志补全成带归属的对象，供后面的判定使用。没有命中时为零值；error（error）：失败原因。nil 表示这一步成功。
// 调用：仅在 decide.go 内使用
// 测试：无直接单测
func (g *Guard) lookup(ctx context.Context, obj Object) (Object, error) {
	db := g.z.db
	if db == nil {
		return obj, nil
	}
	switch obj.Type {
	case ObjectTeam, ObjectMember:
		teamID := obj.TeamID
		if teamID == "" {
			teamID = obj.ID
		}
		t, err := db.GetTeam(ctx, teamID)
		if err != nil {
			return Object{}, notFoundOrInternal(err)
		}
		obj.TeamID, obj.OrgID, obj.Status = t.ID, t.OrganizationID, t.Status
		if err := g.fillOrg(ctx, &obj); err != nil {
			return Object{}, err
		}
		return obj, nil
	case ObjectProject:
		p, err := db.GetProject(ctx, obj.ID)
		if err != nil {
			return Object{}, notFoundOrInternal(err)
		}
		obj.ProjectID, obj.TeamID, obj.OrgID, obj.Status = p.ID, p.TeamID, p.OrganizationID, p.Status
		if err := g.fillTeamAndOrg(ctx, &obj); err != nil {
			return Object{}, err
		}
		return obj, nil
	case ObjectOrg:
		o, err := db.GetOrg(ctx, obj.ID)
		if err != nil {
			return Object{}, notFoundOrInternal(err)
		}
		obj.OrgID, obj.OrgStatus = o.ID, o.Status
		return obj, nil

	case ObjectKey:
		k, err := db.GetKey(ctx, obj.ID)
		if err != nil {
			return Object{}, notFoundOrInternal(err)
		}
		obj.TeamID = k.TeamID
		obj.OwnerType = k.OwnerType
		obj.KeyStatus = k.Status
		if k.UserID != nil {
			obj.OwnerUserID = *k.UserID
		}
		if k.ProjectID != nil {
			obj.ProjectID = *k.ProjectID
		}
		if err := g.fillTeamAndOrg(ctx, &obj); err != nil {
			return Object{}, err
		}
		return obj, nil
	case ObjectUser:
		u, err := db.GetUser(ctx, obj.ID)
		if err != nil {
			return Object{}, notFoundOrInternal(err)
		}
		obj.OwnerUserID, obj.Status = u.ID, u.Status
		return obj, nil
	case ObjectUsage, ObjectLog, ObjectAudit, ObjectModel:
		// These are decided from the caller's memberships and the filters the
		// scopes build, so there is no single owning row to resolve.
		return obj, nil
	default:
		// An object type this layer does not know is not an object. Refusing it
		// here keeps a typo in a handler from falling through to a default allow.
		logx.Error("authz unknown object type=%s", obj.Type)
		return Object{}, ErrNotFound
	}
}

// fillTeamAndOrg loads the status of an object's team and organization so a block anywhere up the chain takes effect.
// 参数 ctx（context.Context）：上下文，取消时停止；obj（*Object）：要判定的对象，含类型、团队、组织和归属用户。
// 返回 error（error）：失败原因，nil 表示这一步成功。
// 调用：仅在 decide.go 内使用
// 测试：无直接单测
func (g *Guard) fillTeamAndOrg(ctx context.Context, obj *Object) error {
	if obj.TeamID == "" {
		return nil
	}
	t, err := g.z.db.GetTeam(ctx, obj.TeamID)
	if err != nil {
		return notFoundOrInternal(err)
	}
	obj.TeamStatus, obj.OrgID = t.Status, t.OrganizationID
	return g.fillOrg(ctx, obj)
}

// fillOrg loads the status of an object's organization.
// 参数 ctx（context.Context）：上下文，取消时停止；obj（*Object）：要判定的对象，含类型、团队、组织和归属用户。
// 返回 error（error）：失败原因，nil 表示这一步成功。
// 调用：仅在 decide.go 内使用
// 测试：无直接单测
func (g *Guard) fillOrg(ctx context.Context, obj *Object) error {
	if obj.OrgID == "" {
		return nil
	}
	o, err := g.z.db.GetOrg(ctx, obj.OrgID)
	if err != nil {
		return notFoundOrInternal(err)
	}
	obj.OrgStatus = o.Status
	return nil
}

// 把未找到留成未找到，把其他读取失败包成内部错误并写日志。
// 参数 err（error）：失败原因，nil 表示这一步成功。
// 返回 error（error）：失败原因，nil 表示这一步成功。
// 调用：仅在 decide.go 内使用
// 测试：无直接单测
func notFoundOrInternal(err error) error {
	if err == iam.ErrNotFound {
		return ErrNotFound
	}
	logx.Error("authz ownership read failed err=%v", err)
	return &InternalError{Err: err}
}

// decide is the matrix from the plan. A session actor reaches this with the object's ownership already resolved and with blocks already applied.
// 参数 ctx（context.Context）：上下文，取消时停止；action（Action）：要判定的动作，例如读日志、管理密钥或发起推理；obj（Object）：要判定的对象，含类型、团队、组织和归属用户。
// 返回 error（error）：失败原因，nil 表示这一步成功。
// 调用：仅在 decide.go 内使用
// 测试：authz_test.go
func (g *Guard) decide(ctx context.Context, action Action, obj Object) error {
	admin := g.PlatformAdmin()

	// A create names the team it will land in but has no row to resolve, so the
	// team hint is checked here rather than trusted: a member may not create
	// anything inside a team they do not belong to.
	if action == ActionKeyCreate {
		return g.decideKeyCreate(ctx, obj, admin)
	}

	switch action {
	// ---------- account ----------
	case ActionUserRead, ActionUserWrite, ActionUserCreate, ActionUserDelete:
		return g.decideUser(action, obj, admin)
	case ActionUserPassword:
		return g.decideUserPassword(ctx, obj, admin)

	// ---------- organizations ----------
	case ActionOrgRead:
		if admin || g.InOrg(obj.OrgID) || g.OrgAdminOf(obj.OrgID) {
			return nil
		}
		return ErrNotFound
	case ActionOrgWrite:
		if admin {
			return nil
		}
		return ErrForbidden
	case ActionOrgAdmin:
		if admin || g.OrgAdminOf(obj.OrgID) {
			return nil
		}
		return ErrForbidden
	// ---------- teams ----------
	case ActionTeamRead:
		return g.decideTeamRead(obj, admin)
	case ActionTeamCreate, ActionTeamDelete, ActionTeamMove, ActionTeamBudget, ActionTeamModels:
		if admin {
			return nil
		}
		return ErrForbidden
	case ActionTeamWrite:
		if admin || g.TeamAdminOf(obj.TeamID) || g.OrgAdminOf(obj.OrgID) {
			return nil
		}
		return ErrForbidden
	case ActionTeamService:
		if admin || g.TeamAdminOf(obj.TeamID) || g.OrgAdminOf(obj.OrgID) {
			return nil
		}
		return ErrForbidden

	// ---------- members ----------
	case ActionMemberRead:
		if admin || g.InTeam(obj.TeamID) || g.OrgAdminOf(obj.OrgID) {
			return nil
		}
		return ErrNotFound
	case ActionMemberWrite, ActionMemberRemove:
		if admin || g.TeamAdminOf(obj.TeamID) || g.OrgAdminOf(obj.OrgID) {
			return nil
		}
		return ErrForbidden

	// ---------- projects ----------
	case ActionProjectRead:
		if admin || g.InTeam(obj.TeamID) || g.OrgAdminOf(obj.OrgID) {
			return nil
		}
		return ErrNotFound
	case ActionProjectWrite:
		if admin || g.TeamAdminOf(obj.TeamID) || g.OrgAdminOf(obj.OrgID) {
			return nil
		}
		return ErrForbidden

	// ---------- keys ----------
	case ActionKeyRead:
		return g.decideKeyRead(obj, admin)
	case ActionKeyWrite, ActionKeyDelete:
		// Personal keys are the owner's alone; a team administrator may not
		// touch them, only the service keys of the team.
		if obj.OwnerType == iam.OwnerPersonal {
			if obj.OwnerUserID == g.actor.UserID {
				return nil
			}
			if admin {
				return nil
			}
			return ErrNotFound
		}
		if admin || g.TeamAdminOf(obj.TeamID) {
			return nil
		}
		return ErrForbidden
	case ActionKeyInspect:
		// Reading key metadata without secret material: any member may see the
		// keys of their team that are not personal.
		if admin || g.TeamAdminOf(obj.TeamID) {
			return nil
		}
		if obj.OwnerType == iam.OwnerPersonal && obj.OwnerUserID == g.actor.UserID {
			return nil
		}
		return ErrNotFound

	// ---------- router templates ----------
	case ActionRouteTemplateRead:
		return g.decideRouteTemplateRead(obj, admin)
	case ActionRouteTemplateWrite:
		return g.decideRouteTemplateWrite(obj, admin)
	case ActionRouteTemplateSelect:
		return g.decideRouteTemplateSelect(obj, admin)

	// ---------- usage, logs, audit ----------
	case ActionUsageRead:
		return g.decideUsageRead(obj, admin)
	case ActionLogRead:
		return g.decideLogRead(obj, admin)
	case ActionAuditRead:
		if admin {
			return nil
		}
		return ErrForbidden

	// ---------- inference ----------
	case ActionInfer:
		return g.decideInfer(obj)
	}
	logx.Error("authz unknown action=%s", action)
	return ErrForbidden
}

// 判定用户读写。管理员或本人允许，其他人拒绝。
// 参数 action（Action）：要判定的动作，例如读日志、管理密钥或发起推理；obj（Object）：要判定的对象，含类型、团队、组织和归属用户；admin（bool）：为真时操作者按管理员处理，可以越过普通成员的限制。
// 返回 error（error）：失败原因，nil 表示这一步成功。
// 调用：仅在 decide.go 内使用
// 测试：无直接单测
func (g *Guard) decideUser(action Action, obj Object, admin bool) error {
	self := obj.OwnerUserID != "" && obj.OwnerUserID == g.actor.UserID
	switch action {
	case ActionUserRead, ActionUserWrite:
		// A person's own account, or any account for a platform administrator.
		// Everyone else reaches other people only through the member listing,
		// which returns public fields under the team scope.
		if admin || self {
			return nil
		}
		return ErrNotFound
	default: // create, delete
		if admin {
			return nil
		}
		return ErrForbidden
	}
}

// decideUserPassword 判定修改密码。只有本人或平台管理员可以改，其他人按未找到处理。
// 参数 ctx（context.Context）：上下文，取消时停止；obj（Object）：要判定的对象，含类型、团队、组织和归属用户；admin（bool）：为真时操作者按管理员处理，可以越过普通成员的限制。
// 返回 error（error）：失败原因，nil 表示这一步成功。
// 调用：仅在 decide.go 内使用
// 测试：无直接单测
func (g *Guard) decideUserPassword(ctx context.Context, obj Object, admin bool) error {
	if obj.OwnerUserID == "" {
		return ErrNotFound
	}
	// A person may replace their own password. That is the account menu, not
	// user management.
	if obj.OwnerUserID == g.actor.UserID {
		return nil
	}
	if admin {
		return nil
	}
	if g.z == nil || g.z.db == nil {
		return &InternalError{Err: errors.New("authz: no store for a password decision")}
	}
	target, err := g.z.db.GetUser(ctx, obj.OwnerUserID)
	if err != nil {
		return notFoundOrInternal(err)
	}
	// A team administrator setting a platform administrator's password would
	// own the deployment. Only an administrator may touch one, which was
	// already answered above.
	if target.Role == iam.RoleAdmin {
		return ErrForbidden
	}
	teams, err := g.z.db.MemberTeams(ctx, target.ID)
	if err != nil {
		return &InternalError{Err: err}
	}
	// The target must sit inside something the actor actually runs: a team they
	// administer, or any team of an organization they administer.
	reached := false
	for _, m := range teams {
		if g.TeamAdminOf(m.TeamID) || g.OrgAdminOf(m.OrganizationID) {
			reached = true
			break
		}
	}
	if !reached {
		// The answer is the same whether the account is a stranger or an
		// account the actor merely shares a team with as a peer, so this cannot
		// be used to work out who is in which team.
		return ErrNotFound
	}
	// An organization administrator is not a team administrator's to reset, in
	// any organization. Their reach is the org's whole inventory, which is more
	// than the team the actor runs.
	admins, err := g.z.db.AdminOrgs(ctx, target.ID)
	if err != nil {
		return &InternalError{Err: err}
	}
	if len(admins) == 0 {
		return nil
	}
	for _, orgID := range admins {
		if g.OrgAdminOf(orgID) {
			return nil
		}
	}
	return ErrForbidden
}

// 判定团队读取。管理员、团队成员或组织管理员允许。
// 参数 obj（Object）：要判定的对象，含类型、团队、组织和归属用户；admin（bool）：为真时操作者按管理员处理，可以越过普通成员的限制。
// 返回 error（error）：失败原因，nil 表示这一步成功。
// 调用：仅在 decide.go 内使用
// 测试：无直接单测
func (g *Guard) decideTeamRead(obj Object, admin bool) error {
	if admin || g.InTeam(obj.TeamID) || g.OrgAdminOf(obj.OrgID) {
		return nil
	}
	return ErrNotFound
}

// decideKeyCreate governs minting a key. A create has no row yet, so the only trustworthy inputs are the actor's memberships and the team it hints at. A member may only mint a key for themselves insidea team they belong to; a service key requires team administration.
// 参数 ctx（context.Context）：上下文，取消时停止；obj（Object）：要判定的对象，含类型、团队、组织和归属用户；admin（bool）：为真时操作者按管理员处理，可以越过普通成员的限制。
// 返回 error（error）：失败原因，nil 表示这一步成功。
// 调用：仅在 decide.go 内使用
// 测试：无直接单测
func (g *Guard) decideKeyCreate(ctx context.Context, obj Object, admin bool) error {
	if obj.OwnerUserID != "" && obj.OwnerUserID != g.actor.UserID && !admin {
		return ErrForbidden
	}
	if obj.OwnerType == iam.OwnerService {
		if admin || g.TeamAdminOf(obj.TeamID) || g.OrgAdminOf(obj.OrgID) {
			return nil
		}
		return ErrForbidden
	}
	if admin {
		return nil
	}
	// The team hint must be a team the actor really belongs to, and a project
	// hint must live in that team.
	if obj.TeamID == "" {
		return ErrForbidden
	}
	if err := g.assertLoaded(ctx); err != nil {
		return err
	}
	if !g.InTeam(obj.TeamID) {
		return ErrForbidden
	}
	if obj.ProjectID != "" && g.z != nil && g.z.db != nil {
		p, err := g.z.db.GetProject(ctx, obj.ProjectID)
		if err != nil {
			return notFoundOrInternal(err)
		}
		if p.TeamID != obj.TeamID {
			return ErrForbidden
		}
	}
	return nil
}

// 判定密钥读取。个人密钥只给本人或管理员，团队密钥按团队权限。
// 参数 obj（Object）：要判定的对象，含类型、团队、组织和归属用户；admin（bool）：为真时操作者按管理员处理，可以越过普通成员的限制。
// 返回 error（error）：失败原因，nil 表示这一步成功。
// 调用：仅在 decide.go 内使用
// 测试：无直接单测
func (g *Guard) decideKeyRead(obj Object, admin bool) error {
	if obj.OwnerType == iam.OwnerPersonal {
		// Personal keys are visible to their owner and to a platform
		// administrator alone. A team administrator never sees them.
		if obj.OwnerUserID == g.actor.UserID || admin {
			return nil
		}
		return ErrNotFound
	}
	// A service key is visible to the team's administrators; ordinary members
	// may not see service keys at all.
	if admin || g.TeamAdminOf(obj.TeamID) {
		return nil
	}
	return ErrNotFound
}

// decideUsageRead governs summary reads. Detail scoping is applied in SQL by UsageScope; this decides whether the dimension may be requested at all.
// 参数 obj（Object）：要判定的对象，含类型、团队、组织和归属用户；admin（bool）：为真时操作者按管理员处理，可以越过普通成员的限制。
// 返回 error（error）：失败原因，nil 表示这一步成功。
// 调用：仅在 decide.go 内使用
// 测试：无直接单测
func (g *Guard) decideUsageRead(obj Object, admin bool) error {
	if admin || g.oversees(obj.TeamID, obj.OrgID) {
		return nil
	}
	// A person reads their own rows. A team total, or another person's rows,
	// stays with the team administrator, the organization administrator, or
	// the platform administrator.
	if obj.OwnerUserID != "" && obj.OwnerUserID == g.actor.UserID {
		return nil
	}
	if obj.TeamID == "" && obj.OwnerUserID == "" {
		return nil
	}
	if obj.TeamID != "" && !g.InTeam(obj.TeamID) && !g.OrgAdminOf(obj.OrgID) {
		return ErrNotFound
	}
	return ErrForbidden
}

// 判定日志读取。管理员、有监督权的人，或个人日志的本人可以看。
// 参数 obj（Object）：要判定的对象，含类型、团队、组织和归属用户；admin（bool）：为真时操作者按管理员处理，可以越过普通成员的限制。
// 返回 error（error）：失败原因，nil 表示这一步成功。
// 调用：仅在 decide.go 内使用
// 测试：无直接单测
func (g *Guard) decideLogRead(obj Object, admin bool) error {
	if admin || g.oversees(obj.TeamID, obj.OrgID) {
		return nil
	}
	if obj.OwnerType == iam.OwnerPersonal && obj.OwnerUserID == g.actor.UserID {
		return nil
	}
	return ErrNotFound
}

// oversees reports whether the actor sees every row under this team: they administer the team, or they administer the organization that owns it.
// 参数 teamID（string）：团队 id。空串表示没有指定团队；orgID（string）：组织 id。空串表示不按组织过滤。
// 返回 bool（bool）：操作者管理这个团队，或管理拥有该团队的组织时返回真，因此能看见该团队下的每一行。
// 调用：仅在 decide.go 内使用
// 测试：无直接单测
func (g *Guard) oversees(teamID, orgID string) bool {
	if teamID != "" && g.TeamAdminOf(teamID) {
		return true
	}
	return g.OrgAdminOf(orgID)
}

// 判定推理。调用方必须属于这个团队，平台管理员除外。
// 参数 obj（Object）：要判定的对象，含类型、团队、组织和归属用户。
// 返回 error（error）：失败原因，nil 表示这一步成功。
// 调用：仅在 decide.go 内使用
// 测试：无直接单测
func (g *Guard) decideInfer(obj Object) error {
	// A UI session may infer through the models of a team it belongs to.
	if obj.TeamID != "" && !g.InTeam(obj.TeamID) && !g.PlatformAdmin() {
		return ErrForbidden
	}
	return nil
}

// ---------- list scoping ----------

// Scope is a WHERE fragment that limits a listing to the rows an actor may see.
// It is built as a builder.Cond so it can be injected into xorm queries.
type Scope struct {
	// Cond is the filter. It is never an "invalid" condition except for the
	// explicit unscoped case, because builder.And silently drops invalid
	// conditions and a dropped filter would return every tenant's rows.
	Cond builder.Cond
	// All reports an intentionally unscoped listing (platform administrator).
	All bool
	// Kind names the listing, so callers can pick the SQL shape.
	Kind ScopeKind
	// TeamID, UserID, KeyIDs and OwnerTypes are the resolved dimensions. They
	// are recorded so callers can build joins that the Cond alone cannot
	// express, without re-deriving the policy.
	TeamIDs    []string
	UserID     string
	KeyIDs     []string
	OwnerTypes []string
	// ServiceOnly narrows a team administrator's log view to service keys.
	ServiceOnly bool
}

// ScopeKind names the SQL shape a scope is meant for.
type ScopeKind string

const (
	ScopeNone ScopeKind = ""
	// ScopeOwnKeys lists keys owned by the actor.
	ScopeOwnKeys ScopeKind = "own_keys"
	// ScopeTeamKeys lists the keys of teams the actor may see.
	ScopeTeamKeys ScopeKind = "team_keys"
	// ScopeAllKeys is the platform administrator listing.
	ScopeAllKeys ScopeKind = "all_keys"
	// ScopeOwnUsage lists usage the actor produced.
	ScopeOwnUsage ScopeKind = "own_usage"
	// ScopeTeamUsage lists usage inside teams the actor may see.
	ScopeTeamUsage ScopeKind = "team_usage"
	// ScopeAllUsage is the platform administrator listing.
	ScopeAllUsage ScopeKind = "all_usage"
	// ScopeOwnLogs lists request logs of the actor's own personal keys.
	ScopeOwnLogs ScopeKind = "own_logs"
	// ScopeTeamLogs lists the actor's own logs plus the team's service-key logs.
	ScopeTeamLogs ScopeKind = "team_logs"
	// ScopeAllLogs is the platform administrator listing, audited on read.
	ScopeAllLogs ScopeKind = "all_logs"
)

// deny is the fail-closed filter: a condition that is always false yet always valid, so builder.And keeps it.
// 参数：无。
// 返回 Cond（builder.Cond）：恒为 0=1 的条件。并进别的条件后，整段查询不会命中任何行。
// 调用：authz/scope.go
// 测试：无直接单测
func deny() builder.Cond { return builder.Expr("0=1") }

// KeysScope returns the key-listing scope for an actor.
// 参数 ctx（context.Context）：上下文，取消或超时时停止后续工作。
// 返回 *Scope（*Scope）：这个操作者能列出的密钥范围。主密钥得到恒假条件，避免被当成跨租户列表；error（error）：成员关系读失败。nil 表示范围已经定好。
// 调用：gateway/keys/admin.go、gateway/keys/generate.go、gateway/keys/host.go、gateway/wire.go
// 测试：authz_test.go
func (g *Guard) KeysScope(ctx context.Context) (*Scope, error) {
	if g.actor.Kind == KindMaster {
		// The master credential reaches the bootstrap and emergency routes only.
		// It does not get a listing scope, so a handler that forgets to
		// authorize cannot turn it into a cross-tenant read.
		return &Scope{Cond: deny(), Kind: ScopeNone}, nil
	}
	if g.PlatformAdmin() {
		return &Scope{Cond: builder.NewCond(), All: true, Kind: ScopeAllKeys}, nil
	}
	if g.actor.Kind == KindKey {
		// A key sees itself only, and only while it is still live.
		if err := g.CheckKey(ctx); err != nil {
			return nil, err
		}
		if g.actor.KeyID == "" {
			return &Scope{Cond: deny(), Kind: ScopeOwnKeys}, nil
		}
		return &Scope{Cond: builder.Eq{"id": g.actor.KeyID}, Kind: ScopeOwnKeys, KeyIDs: []string{g.actor.KeyID}}, nil
	}
	teams := g.VisibleTeamIDs()
	if err := g.assertLoaded(ctx); err != nil {
		return nil, err
	}
	return g.keyScopeFor(teams), nil
}

// keyScopeFor combines the personal keys of the actor with the service keys of the teams the actor administers. An ordinary member sees only their own.
// 参数 teams（[]string）：teams列表。空切片表示没有可处理的项。
// 返回 *Scope（*Scope）：自己的个人密钥，加上所管理团队的服务密钥。没有管理的团队时只含个人密钥。
// 调用：仅在 decide.go 内使用
// 测试：无直接单测
func (g *Guard) keyScopeFor(teams []string) *Scope {
	managed := g.ManagedTeamIDs()
	own := builder.Eq{"owner_type": iam.OwnerPersonal, "user_id": g.actor.UserID}
	if len(managed) == 0 {
		return &Scope{Cond: own, Kind: ScopeOwnKeys, UserID: g.actor.UserID,
			OwnerTypes: []string{iam.OwnerPersonal}}
	}
	service := builder.Cond(builder.Eq{"owner_type": iam.OwnerService})
	service = service.And(builder.In("team_id", managed))
	return &Scope{
		Cond:       own.Or(service),
		Kind:       ScopeTeamKeys,
		TeamIDs:    managed,
		UserID:     g.actor.UserID,
		OwnerTypes: []string{iam.OwnerPersonal, iam.OwnerService},
	}
}

// UsageScope returns the usage-listing scope for an actor. teamID optionally narrows to one team, and the guard checks the actor may see it at all.
// 参数 ctx（context.Context）：上下文，取消或超时时停止后续工作；teamID（string）：团队 id。空串表示没有指定团队。
// 返回 *Scope（*Scope）：用量列表的范围。主密钥得到恒假条件。平台管理员不指定团队时能看全部；error（error）：无权看这个团队或成员关系读失败。nil 表示范围已经定好。
// 调用：gateway/usage/activity.go、gateway/usage/entity_activity.go、gateway/usage/host.go、gateway/usage/reports.go
// 测试：authz_test.go
func (g *Guard) UsageScope(ctx context.Context, teamID string) (*Scope, error) {
	if g.actor.Kind == KindMaster {
		return &Scope{Cond: deny(), Kind: ScopeNone}, nil
	}
	if g.PlatformAdmin() {
		if teamID == "" {
			return &Scope{Cond: builder.NewCond(), All: true, Kind: ScopeAllUsage}, nil
		}
		return &Scope{Cond: builder.Eq{"team_id": teamID}, Kind: ScopeAllUsage, TeamIDs: []string{teamID}}, nil
	}
	if err := g.assertLoaded(ctx); err != nil {
		return nil, err
	}
	if g.actor.Kind == KindKey {
		if err := g.CheckKey(ctx); err != nil {
			return nil, err
		}
		return &Scope{Cond: builder.Eq{"key_id": g.actor.KeyID}, Kind: ScopeOwnUsage, KeyIDs: []string{g.actor.KeyID}}, nil
	}
	watch, err := g.oversightTeamIDs(ctx)
	if err != nil {
		return nil, err
	}
	own := builder.Eq{"user_id": g.actor.UserID}
	if teamID != "" {
		if containsID(watch, teamID) {
			return &Scope{Cond: builder.Eq{"team_id": teamID}, Kind: ScopeTeamUsage, TeamIDs: []string{teamID}}, nil
		}
		if g.InTeam(teamID) {
			return &Scope{Cond: own.And(builder.Eq{"team_id": teamID}), Kind: ScopeOwnUsage, TeamIDs: []string{teamID}, UserID: g.actor.UserID}, nil
		}
		return nil, ErrNotFound
	}
	if len(watch) == 0 {
		return &Scope{Cond: own, Kind: ScopeOwnUsage, UserID: g.actor.UserID}, nil
	}
	return &Scope{Cond: own.Or(builder.In("team_id", watch)), Kind: ScopeTeamUsage, TeamIDs: watch, UserID: g.actor.UserID}, nil
}

// LogsScope returns the request-log scope for an actor: own personal logs, plus the service-key logs of the teams the actor administers. A platform administrator sees everything.
// 参数 ctx（context.Context）：上下文，取消或超时时停止后续工作。
// 返回 *Scope（*Scope）：请求日志的范围。本人的个人日志，加上所管理团队的服务密钥日志。平台管理员能看全部；error（error）：成员关系读失败。nil 表示范围已经定好。
// 调用：gateway/usage/host.go、gateway/usage/reports.go、gateway/wire.go
// 测试：authz_test.go
func (g *Guard) LogsScope(ctx context.Context) (*Scope, error) {
	if g.actor.Kind == KindMaster {
		return &Scope{Cond: deny(), Kind: ScopeNone}, nil
	}
	if g.PlatformAdmin() {
		return &Scope{Cond: builder.NewCond(), All: true, Kind: ScopeAllLogs}, nil
	}
	if err := g.assertLoaded(ctx); err != nil {
		return nil, err
	}
	if g.actor.Kind == KindKey {
		if err := g.CheckKey(ctx); err != nil {
			return nil, err
		}
		return &Scope{Cond: builder.Eq{"key_id": g.actor.KeyID}, Kind: ScopeOwnLogs, KeyIDs: []string{g.actor.KeyID}}, nil
	}
	own := builder.Eq{"owner_type": iam.OwnerPersonal, "user_id": g.actor.UserID}
	watch, err := g.oversightTeamIDs(ctx)
	if err != nil {
		return nil, err
	}
	if len(watch) == 0 {
		return &Scope{Cond: own, Kind: ScopeOwnLogs, UserID: g.actor.UserID,
			OwnerTypes: []string{iam.OwnerPersonal}}, nil
	}
	service := builder.Cond(builder.Eq{"owner_type": iam.OwnerService})
	service = service.And(builder.In("team_id", watch))
	return &Scope{
		Cond:        own.Or(service),
		Kind:        ScopeTeamLogs,
		TeamIDs:     watch,
		UserID:      g.actor.UserID,
		OwnerTypes:  []string{iam.OwnerPersonal, iam.OwnerService},
		ServiceOnly: true,
	}, nil
}

// 列出这个用户能够监督的团队 id。没有库或没有用户时为空。
// 参数 ctx（context.Context）：上下文，取消或超时时停止后续工作。
// 返回 []string（[]string）：监督团队标识列表。没有匹配时为空切片；error（error）：失败原因。nil 表示这一步成功。
// 调用：仅在 decide.go 内使用
// 测试：无直接单测
func (g *Guard) oversightTeamIDs(ctx context.Context) ([]string, error) {
	if g.z == nil || g.z.db == nil || g.actor.UserID == "" {
		return nil, nil
	}
	return g.z.db.OversightTeamIDs(ctx, g.actor.UserID)
}

// 判断 id 是否在这组 id 里。
// 参数 ids（[]string）：要保留或查询的一组 id。空切片表示没有可处理的记录；id（string）：包含标识使用的主键。空串表示调用方没有指定记录。
// 返回 bool（bool）：ids 里有这个 id 时为真。
// 调用：仅在 decide.go 内使用
// 测试：无直接单测
func containsID(ids []string, id string) bool {
	for _, cur := range ids {
		if cur == id {
			return true
		}
	}
	return false
}

// TeamFilter returns the team-ID filter for team-scoped listings such as projects and members. A nil slice means "every team", which only a platform administrator receives; an empty slice means "no team
// ", which the store turns into an empty result rather than an unfiltered query.
// 参数：无。
// 返回 []string（[]string）：团队过滤。没有匹配时为空切片。
// 调用：gateway/identity/gate.go、gateway/identity/handlers.go、gateway/wire.go
// 测试：authz_test.go
func (g *Guard) TeamFilter() []string {
	return g.VisibleTeamIDs()
}

// assertLoaded re-reads the membership snapshot when a scoped listing is requested, so a session whose memberships changed mid-request cannot list through a stale snapshot.
// 参数 ctx（context.Context）：上下文，取消或超时时停止后续工作。
// 返回 error（error）：失败原因，nil 表示这一步成功。
// 调用：仅在 decide.go 内使用
// 测试：无直接单测
func (g *Guard) assertLoaded(ctx context.Context) error {
	if g.actor.Kind != KindSession {
		return nil
	}
	if g.z == nil || g.z.db == nil {
		return nil
	}
	u, err := g.z.db.GetUser(ctx, g.actor.UserID)
	if err != nil {
		if err == iam.ErrNotFound {
			return ErrUnauthenticated
		}
		return &InternalError{Err: err}
	}
	if !u.Active() {
		return ErrUnauthenticated
	}
	return nil
}

// decideRouteTemplateRead governs seeing a router template.
//
// Visibility is derived from ownership, which is what makes a template usable by
// the scopes below its owner without exposing it sideways. A platform template
// has no owner and is visible to everyone; an organization's template is visible
// within that organization; a team's template is visible to that team.
//
// An invisible template answers ErrNotFound rather than ErrForbidden. Answering
// "exists but not yours" would confirm the existence of another tenant's
// configuration, which is the same reason the team queries answer 404.
// 参数 obj（Object）：要判定的对象，含类型、团队、组织和归属用户；admin（bool）：为真时操作者按管理员处理，可以越过普通成员的限制。
// 返回 error（error）：失败原因，nil 表示这一步成功。
// 调用：仅在 decide.go 内使用
// 测试：authz_test.go
func (g *Guard) decideRouteTemplateRead(obj Object, admin bool) error {
	if admin {
		return nil
	}
	// Platform templates: no organization, no team. Everyone may read one, because
	// everyone falls back to the platform default and a template is how a
	// replacement for it is described.
	if obj.OrgID == "" && obj.TeamID == "" {
		return nil
	}
	if obj.TeamID != "" {
		// A team template is read by that team's members. An organization
		// administrator reads it too: they administer the team, so hiding the
		// team's configuration from them would contradict that.
		if g.InTeam(obj.TeamID) || g.OrgAdminOf(obj.OrgID) {
			return nil
		}
		return ErrNotFound
	}
	// An organization template is read by anyone in the organization, which
	// includes every team beneath it.
	if g.InOrg(obj.OrgID) || g.OrgAdminOf(obj.OrgID) {
		return nil
	}
	return ErrNotFound
}

// decideRouteTemplateWrite governs creating, editing and deleting a router
// template. Only the owner may write.
//
// This is the half that keeps one tenant from editing another's configuration.
// Reading it from a sibling team is allowed on purpose; editing it is not.
// 参数 obj（Object）：要判定的对象，含类型、团队、组织和归属用户；admin（bool）：为真时操作者按管理员处理，可以越过普通成员的限制。
// 返回 error（error）：失败原因，nil 表示这一步成功。
// 调用：仅在 decide.go 内使用
// 测试：authz_test.go
func (g *Guard) decideRouteTemplateWrite(obj Object, admin bool) error {
	if admin {
		return nil
	}
	// A team template belongs to its team's administrator, and to the
	// organization administrator above it.
	if obj.TeamID != "" {
		if g.TeamAdminOf(obj.TeamID) || g.OrgAdminOf(obj.OrgID) {
			return nil
		}
		return ErrForbidden
	}
	// An organization template, and a platform template, belong to the
	// administrator of that level. A plain member of the organization does not
	// get to author one: writing is where an unvetted configuration enters the
	// pool that everyone below can select.
	if g.OrgAdminOf(obj.OrgID) {
		return nil
	}
	return ErrForbidden
}

// decideRouteTemplateSelect governs pointing a scope at a template.
//
// A member may select for their own team, their own organization and their own
// personal key. Selection is narrower than writing: it picks among
// configurations somebody already approved, and it applies to exactly the scopes
// the caller already belongs to, so it grants no reach they did not have.
//
// A service key is the team's rather than a person's, so selecting for it takes
// the team administrator, matching how service keys are created and rotated.
// 参数 obj（Object）：要判定的对象，含类型、团队、组织和归属用户；admin（bool）：为真时操作者按管理员处理，可以越过普通成员的限制。
// 返回 error（error）：失败原因，nil 表示这一步成功。
// 调用：仅在 decide.go 内使用
// 测试：authz_test.go
func (g *Guard) decideRouteTemplateSelect(obj Object, admin bool) error {
	if admin {
		return nil
	}
	switch obj.Type {
	case ObjectKey:
		if obj.OwnerType == iam.OwnerPersonal {
			if obj.OwnerUserID == g.actor.UserID {
				return nil
			}
			return ErrNotFound
		}
		if g.TeamAdminOf(obj.TeamID) {
			return nil
		}
		return ErrForbidden
	case ObjectTeam:
		if g.InTeam(obj.TeamID) || g.OrgAdminOf(obj.OrgID) {
			return nil
		}
		return ErrNotFound
	case ObjectOrg:
		if g.InOrg(obj.OrgID) || g.OrgAdminOf(obj.OrgID) {
			return nil
		}
		return ErrNotFound
	default:
		return ErrNotFound
	}
}
