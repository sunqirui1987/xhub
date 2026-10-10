package iam

import (
	"context"
	"fmt"
	"testing"
)

// TestUserDirectoryPagination 前置隔离 PostgreSQL 与六个用户、团队及组织成员；参数 t 管理生命周期。
// 验证总数、稳定多页、范围交集、筛选、非法/越界及取消错误，整个私有 schema 自动清理。
func TestUserDirectoryPagination(t *testing.T) {
	db := testDB(t)
	users := []User{}
	for i := 0; i < 6; i++ {
		users = append(users, User{ID: fmt.Sprintf("u%d", i), Email: fmt.Sprintf("user%d@example.com", i), Name: "directory", Role: RoleUser, Status: StatusActive})
	}
	if _, err := db.Engine.Insert(&users); err != nil {
		t.Fatal(err)
	}
	// 团队和组织成员具有真实外键，必须先创建其组织，避免夹具掩盖分页行为。
	if _, err := db.Engine.Insert(&Organization{ID: "org", Name: "org", Status: StatusActive}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Engine.Insert(&Team{ID: "team", OrganizationID: "org", Name: "team", Status: StatusActive}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Engine.Insert(&TeamMembership{TeamID: "team", UserID: "u1", Role: TeamMember}, &TeamMembership{TeamID: "team", UserID: "u2", Role: TeamMember}, &OrganizationMembership{OrganizationID: "org", UserID: "u3", Role: OrgAdmin}); err != nil {
		t.Fatal(err)
	}
	q := UserListQuery{PlatformAdmin: true, Limit: 2, SortBy: "user_email", SortAsc: true}
	for page := 0; page < 3; page++ {
		q.Offset = page * 2
		rows, total, err := db.ListUsersPage(t.Context(), q)
		if err != nil || total != 6 || len(rows) != 2 || rows[0].ID != fmt.Sprintf("u%d", page*2) {
			t.Fatalf("目录第%d页总数或稳定排序错误: %+v %d %v", page+1, rows, total, err)
		}
	}
	for _, c := range []struct {
		q     UserListQuery
		total int64
	}{
		{UserListQuery{Self: "u0", TeamIDs: []string{"team"}}, 3},
		{UserListQuery{Self: "u0"}, 1},
		{UserListQuery{Self: "u0", UserIDs: []string{"u5"}}, 0},
		{UserListQuery{PlatformAdmin: true, OrganizationIDs: []string{"org"}}, 3},
		{UserListQuery{PlatformAdmin: true, TeamID: "team"}, 2},
		{UserListQuery{PlatformAdmin: true, Search: "u4"}, 1},
		{UserListQuery{PlatformAdmin: true, Email: "USER3@"}, 1},
		{UserListQuery{PlatformAdmin: true, Role: RoleAdmin}, 0},
		{UserListQuery{PlatformAdmin: true, SSOID: "not-stored"}, 0},
		{UserListQuery{PlatformAdmin: true, Limit: -1, Offset: -1}, 6},
		{UserListQuery{PlatformAdmin: true, Limit: 999}, 6},
	} {
		rows, total, err := db.ListUsersPage(t.Context(), c.q)
		if err != nil || total != c.total || int64(len(rows)) != total {
			t.Fatalf("目录条件%+v: %d/%d %v", c.q, len(rows), total, err)
		}
	}
	q.Offset = int(^uint(0) >> 1)
	if rows, total, err := db.ListUsersPage(t.Context(), q); err != nil || total != 6 || len(rows) != 0 {
		t.Fatalf("越界页: %+v %d %v", rows, total, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, _, err := db.ListUsersPage(ctx, q); err == nil {
		t.Fatal("已取消查询必须返回错误")
	}
}

// TestUserDirectoryOrder 前置纯内存条件，验证列映射、同值ID及恶意列回退；无需清理。
func TestUserDirectoryOrder(t *testing.T) {
	for _, c := range []struct {
		q    UserListQuery
		want string
	}{{UserListQuery{}, "created_at DESC, id DESC"}, {UserListQuery{SortBy: "user_email", SortAsc: true}, "email ASC, id ASC"}, {UserListQuery{SortBy: "user_id"}, "id DESC"}, {UserListQuery{SortBy: "id; DROP TABLE users"}, "created_at DESC, id DESC"}} {
		if got := userListOrder(c.q); got != c.want {
			t.Fatalf("安全排序: %q != %q", got, c.want)
		}
	}
}
