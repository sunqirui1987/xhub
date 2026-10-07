package authz

import (
	"github.com/sunqirui1987/xhub/internal/logx"
	"xorm.io/builder"
	"xorm.io/xorm"
)

// Apply 把鉴权范围收成 SQL 条件，接到当前查询上。范围为空时条件恒为假，避免查出全部行。
// 参数 session（*xorm.Session）：正在构造的 xorm 查询。过滤条件接到这条查询上，不是会话 id。
// 返回 *xorm.Session（*xorm.Session）：接上范围条件后的同一条查询。调用方继续用它取行。
// 调用：用量和日志列表在执行 SQL 之前。
// 测试：无直接单测
func (s *Scope) Apply(session *xorm.Session) *xorm.Session {
	if s == nil {
		logx.Error("authz scope missing; denying listing")
		return session.Where(deny())
	}
	if s.All {
		return session
	}
	if s.Cond == nil || !s.Cond.IsValid() {
		// A scope that is neither explicitly unscoped nor a real filter must
		// deny, never fall through to an unfiltered query.
		logx.Error("authz scope %q has no condition; denying listing", s.Kind)
		return session.Where(deny())
	}
	return session.And(s.Cond)
}

// ApplyTeam narrows a session by a column holding team IDs, using the resolved team list rather than a stored condition. It is used by listings whose team column is not the one the scope's condition names.
// 参数 session（*xorm.Session）：正在构造的 xorm 查询。过滤条件接到这条查询上，不是会话 id；column（string）：存放团队 id 的列名。
// 返回 *xorm.Session（*xorm.Session）：接上团队条件后的同一条查询。
// 调用：团队列表，列名和范围条件里的列不一致时。
// 测试：authz_test.go
func (s *Scope) ApplyTeam(session *xorm.Session, column string) *xorm.Session {
	if s == nil {
		return session.Where(deny())
	}
	if s.All {
		return session
	}
	if len(s.TeamIDs) == 0 {
		// Fail closed. builder.In would emit 0=1 here, but it also reports the
		// condition as invalid, so it must be written explicitly.
		return session.Where(deny())
	}
	return session.And(builder.In(column, s.TeamIDs))
}

// ApplyUser narrows a session by an owner column, failing closed when the scope carries no owner.
// 参数 session（*xorm.Session）：正在构造的 xorm 查询。过滤条件接到这条查询上，不是会话 id；column（string）：存放归属用户 id 的列名。
// 返回 *xorm.Session（*xorm.Session）：接上用户条件后的同一条查询。没有归属用户时条件恒为假。
// 调用：按归属用户缩小的列表。
// 测试：authz_test.go
func (s *Scope) ApplyUser(session *xorm.Session, column string) *xorm.Session {
	if s == nil {
		return session.Where(deny())
	}
	if s.All {
		return session
	}
	if s.UserID == "" {
		return session.Where(deny())
	}
	return session.And(builder.Expr(column+" = ?", s.UserID))
}
