package iam

import (
	"context"
	"testing"
)

func TestQueryAuditFiltersBeforePagination(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	for _, row := range []struct {
		actor Actor
		event Audit
	}{
		{Actor{ID: "admin-a", Kind: "session"}, Audit{Action: "key.create", ObjectType: "key", ObjectID: "key-1", TeamID: "team-a"}},
		{Actor{ID: "admin-b", Kind: "session"}, Audit{Action: "team.create", ObjectType: "team", ObjectID: "team-b", TeamID: "team-b"}},
		{Actor{ID: "admin-a", Kind: "session"}, Audit{Action: "key.update", ObjectType: "key", ObjectID: "key-1", TeamID: "team-a"}},
		{Actor{ID: "admin-a", Kind: "session"}, Audit{Action: "key.delete", ObjectType: "key", ObjectID: "key-1", TeamID: "team-a"}},
		{Actor{ID: "actor-key", Kind: "key"}, Audit{Action: "log.read", ObjectType: "request_log", ObjectID: "request-1"}},
	} {
		if err := db.RecordAudit(ctx, row.actor, row.event); err != nil {
			t.Fatal(err)
		}
	}
	rows, total, err := db.QueryAudit(ctx, AuditFilter{ActorID: "admin-a", TeamID: "team-a", ObjectType: "key", Search: "key-1"}, 1, 1)
	if err != nil || total != 3 || len(rows) != 1 || rows[0].Action != "key.update" {
		t.Fatalf("filtered page: rows=%+v total=%d err=%v", rows, total, err)
	}
	for _, tc := range []struct {
		filter AuditFilter
		action string
	}{
		{AuditFilter{ObjectID: "key-1", Action: "created"}, "key.create"},
		{AuditFilter{Action: "updated"}, "key.update"},
		{AuditFilter{KeyID: "key-1", Action: "deleted"}, "key.delete"},
		{AuditFilter{ActorKey: "actor-key"}, "log.read"},
		{AuditFilter{Action: "log.read"}, "log.read"},
	} {
		rows, total, err := db.QueryAudit(ctx, tc.filter, 50, 0)
		if err != nil || total != 1 || len(rows) != 1 || rows[0].Action != tc.action {
			t.Fatalf("filter %+v: rows=%+v total=%d err=%v", tc.filter, rows, total, err)
		}
	}
	rows, total, err = db.QueryAudit(ctx, AuditFilter{Search: "missing"}, 50, 0)
	if err != nil || total != 0 || len(rows) != 0 {
		t.Fatalf("empty search: rows=%+v total=%d err=%v", rows, total, err)
	}
}

func TestAuditNamesSurviveRenameAndDeletion(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	user, err := db.CreateUser(ctx, Actor{Kind: "system"}, UserInput{
		Name: "Audit Operator", Email: "audit-operator@example.com", Password: "password123", Role: RoleAdmin,
	})
	if err != nil {
		t.Fatal(err)
	}
	by := Actor{ID: user.ID, Kind: "session"}
	template, err := db.CreateRouteTemplate(ctx, by, TemplateOwner{}, "Original Template", `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpdateRouteTemplate(ctx, by, template.ID, "Renamed Template", `{"num_retries":2}`); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteRouteTemplate(ctx, by, template.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Engine.ID(user.ID).Cols("name", "email").Update(&User{Name: "New Operator Name", Email: "new@example.com"}); err != nil {
		t.Fatal(err)
	}
	rows, total, err := db.QueryAudit(ctx, AuditFilter{ObjectID: template.ID}, 50, 0)
	if err != nil || total != 3 || len(rows) != 3 {
		t.Fatalf("history: rows=%+v total=%d err=%v", rows, total, err)
	}
	for i, expected := range []string{"Renamed Template", "Renamed Template", "Original Template"} {
		if rows[i].ObjectName != expected || rows[i].ActorName != "Audit Operator" || rows[i].ActorEmail != "audit-operator@example.com" {
			t.Fatalf("snapshot %d: %+v", i, rows[i])
		}
	}
	if rows[1].Detail["previous_name"] != "Original Template" || rows[1].Detail["settings_changed"] != true {
		t.Fatalf("rename detail: %+v", rows[1].Detail)
	}
	for _, tc := range []struct {
		search string
		total  int64
	}{
		{"Original Template", 1}, {"Renamed Template", 2}, {"audit-operator@example.com", 3}, {"%", 0},
	} {
		_, total, err := db.QueryAudit(ctx, AuditFilter{ObjectID: template.ID, Search: tc.search}, 1, 0)
		if err != nil || total != tc.total {
			t.Fatalf("search %q: total=%d err=%v", tc.search, total, err)
		}
	}
}
