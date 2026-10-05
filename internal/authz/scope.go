package authz

import (
	"github.com/sunqirui1987/xhub/internal/logx"
	"xorm.io/builder"
	"xorm.io/xorm"
)

// Apply narrows a session to the rows this scope permits. When the scope is
// unscoped it leaves the session alone, which is only ever the case for a
// platform administrator.
//
// This is the only supported way to apply a scope, because it is easy to get
// wrong by hand: builder.And drops any condition whose IsValid() is false, and
// builder.In with an empty list is exactly such a condition. Assigning a raw
// empty scope to a query would therefore return every tenant's rows. Apply
// treats an unscoped-but-not-All scope as a denial instead.
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

// ApplyTeam narrows a session by a column holding team IDs, using the resolved
// team list rather than a stored condition. It is used by listings whose team
// column is not the one the scope's condition names.
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

// ApplyUser narrows a session by an owner column, failing closed when the scope
// carries no owner.
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
