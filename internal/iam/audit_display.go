package iam

import (
	"context"
	"strings"

	"github.com/sunqirui1987/xhub/internal/logx"
	"xorm.io/xorm"
)

// These are fixed schema names, never table names supplied by a request.
var auditNameSources = []struct {
	table string
	types []string
}{
	{"users", []string{"user", "team_member"}},
	{"organizations", []string{"organization", "organizations"}},
	{"teams", []string{"team", "teams"}},
	{"projects", []string{"project"}},
	{"api_keys", []string{"key", "api_keys"}},
	{"route_templates", []string{"route_template"}},
}

// auditText reads an optional immutable display snapshot from audit detail.
// 参数 detail（map[string]any）、key（string）：事件详情和字段名。
// 返回 string：字符串快照或空值。
// 调用：enrichAudit。
// 测试：audit_test.go
func auditText(detail map[string]any, key string) string {
	value, _ := detail[key].(string)
	return value
}

// Historical snapshots win over current names. Old records without snapshots
// use batch lookups restricted to the current page, including deleted actors.
// 参数 ctx（context.Context）、rows（[]AuditEntry）：请求上下文和已授权事件页。
// 返回 error：补充旧事件显示名时的数据库错误。
// 调用：审计日志分页读取。
// 测试：audit_test.go
func (db *DB) enrichAudit(ctx context.Context, rows []AuditEntry) error {
	actorIDs := make([]string, 0, len(rows))
	for i := range rows {
		row := &rows[i]
		row.ActorName = auditText(row.Detail, "actor_name")
		row.ActorEmail = auditText(row.Detail, "actor_email")
		row.ObjectName = auditText(row.Detail, "object_name")
		if row.ActorKind == "session" && row.ActorID != "" {
			actorIDs = append(actorIDs, row.ActorID)
		}
	}
	if len(actorIDs) > 0 {
		var users []User
		s := db.session(ctx)
		err := s.In("id", actorIDs).Cols("id", "name", "email").Find(&users)
		s.Close()
		if err != nil {
			logx.Error("audit actor name lookup failed: %v", err)
			return err
		}
		byID := make(map[string]User, len(users))
		for _, user := range users {
			byID[user.ID] = user
		}
		for i := range rows {
			row := &rows[i]
			if row.ActorKind != "session" {
				continue
			}
			user := byID[row.ActorID]
			if _, saved := row.Detail["actor_name"]; !saved {
				row.ActorName = user.Name
			}
			if _, saved := row.Detail["actor_email"]; !saved {
				row.ActorEmail = user.Email
			}
		}
	}
	for _, source := range auditNameSources {
		ids := make([]string, 0, len(rows))
		matches := func(objectType string) bool {
			for _, kind := range source.types {
				if kind == objectType {
					return true
				}
			}
			return false
		}
		for _, row := range rows {
			if matches(row.ObjectType) && row.ObjectName == "" && row.ObjectID != "" {
				ids = append(ids, row.ObjectID)
			}
			if source.table == "api_keys" && row.ActorKind == "key" && row.ActorName == "" && row.ActorID != "" {
				ids = append(ids, row.ActorID)
			}
		}
		if len(ids) == 0 {
			continue
		}
		var names []struct {
			ID   string `xorm:"'id'"`
			Name string `xorm:"'name'"`
		}
		s := db.session(ctx)
		err := s.Table(source.table).In("id", ids).Select("id, name").Find(&names)
		s.Close()
		if err != nil {
			logx.Error("audit object name lookup failed for %s: %v", source.table, err)
			return err
		}
		byID := make(map[string]string, len(names))
		for _, row := range names {
			byID[row.ID] = row.Name
		}
		for i := range rows {
			row := &rows[i]
			if matches(row.ObjectType) && row.ObjectName == "" {
				row.ObjectName = byID[row.ObjectID]
			}
			if source.table == "api_keys" && row.ActorKind == "key" && row.ActorName == "" {
				row.ActorName = byID[row.ActorID]
			}
		}
	}
	return nil
}

// Search the visible identities and event codes before counting/pagination.
// strpos treats %, _ and other characters literally rather than as wildcards.
// 参数 query（*xorm.Session）、search（string）：授权查询与搜索词。
// 返回：无；在查询上追加可见字段条件。
// 调用：审计日志分页读取。
// 测试：audit_test.go
func applyAuditSearch(query *xorm.Session, search string) {
	parts := []string{}
	args := []any{}
	for _, expression := range []string{
		"CAST(audit_logs.id AS TEXT)", "audit_logs.object_id", "audit_logs.actor_id", "audit_logs.action",
		"audit_logs.detail->>'object_name'", "audit_logs.detail->>'actor_name'", "audit_logs.detail->>'actor_email'",
	} {
		parts = append(parts, "strpos(lower("+expression+"), lower(?)) > 0")
		args = append(args, search)
	}
	parts = append(parts, "EXISTS (SELECT 1 FROM users u WHERE audit_logs.actor_kind = 'session' AND u.id = audit_logs.actor_id AND (strpos(lower(u.name), lower(?)) > 0 OR strpos(lower(u.email), lower(?)) > 0))")
	args = append(args, search, search)
	for _, source := range auditNameSources {
		kinds := make([]string, len(source.types))
		for i, kind := range source.types {
			kinds[i] = "'" + kind + "'"
		}
		parts = append(parts, "EXISTS (SELECT 1 FROM "+source.table+" n WHERE audit_logs.object_type IN ("+strings.Join(kinds, ",")+") AND n.id = audit_logs.object_id AND strpos(lower(n.name), lower(?)) > 0)")
		args = append(args, search)
	}
	parts = append(parts, "EXISTS (SELECT 1 FROM api_keys k WHERE audit_logs.actor_kind = 'key' AND k.id = audit_logs.actor_id AND strpos(lower(k.name), lower(?)) > 0)")
	args = append(args, search)
	query.And("("+strings.Join(parts, " OR ")+")", args...)
}
