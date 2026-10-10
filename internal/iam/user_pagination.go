package iam

import (
	"context"
	"strings"

	"xorm.io/builder"
	"xorm.io/xorm"
)

// UserListQuery 是用户目录分页条件；调用方先传授权范围，再传可收窄的筛选。
// PlatformAdmin 仅由服务端身份设置；Self/TeamIDs 限定非管理员可见用户，客户端不能扩大范围。
type UserListQuery struct {
	PlatformAdmin                              bool
	Self                                       string
	TeamIDs                                    []string
	Search, Email, Role, TeamID, SSOID, SortBy string
	UserIDs, OrganizationIDs                   []string
	SortAsc                                    bool
	Limit, Offset                              int
}

// userListSession 为一次目录读取构建独立 SQL 会话，参数 ctx/q 是上下文和已授权条件。
// 返回会话供计数或取页，调用者负责关闭；全部外部值绑定参数，筛选始终与授权范围取交集。
func (db *DB) userListSession(ctx context.Context, q UserListQuery) *xorm.Session {
	s := db.session(ctx)
	if !q.PlatformAdmin {
		cond := builder.Eq{"id": q.Self}
		if len(q.TeamIDs) == 0 {
			s = s.Where(cond)
		} else {
			s = s.Where(builder.Or(cond, builder.In("id", builder.Select("user_id").From("team_members").Where(builder.In("team_id", q.TeamIDs)))))
		}
	}
	if q.Search != "" {
		like := "%" + q.Search + "%"
		s = s.And("(id ILIKE ? OR email ILIKE ? OR name ILIKE ?)", like, like, like)
	}
	if q.Email != "" {
		s = s.And("email ILIKE ?", "%"+q.Email+"%")
	}
	if len(q.UserIDs) > 0 {
		s = s.And(builder.In("id", q.UserIDs))
	}
	if q.Role != "" {
		s = s.And("role = ?", q.Role)
	}
	if q.TeamID != "" {
		s = s.And("id IN (SELECT user_id FROM team_members WHERE team_id = ?)", q.TeamID)
	}
	if len(q.OrganizationIDs) > 0 {
		orgMembers := builder.Select("user_id").From("organization_members").Where(builder.In("organization_id", q.OrganizationIDs))
		teamMembers := builder.Select("user_id").From("team_members").Where(builder.In("team_id", builder.Select("id").From("teams").Where(builder.In("organization_id", q.OrganizationIDs))))
		s = s.And(builder.Or(builder.In("id", orgMembers), builder.In("id", teamMembers)))
	}
	// 当前身份存储没有 SSO ID；指定此条件应为空，不能忽略后展示无关用户。
	if q.SSOID != "" {
		s = s.And("1 = 0")
	}
	return s
}

// userListOrder 根据前端列名生成白名单排序 SQL；参数 q 携带列名和方向，返回稳定排序。
// ListUsersPage 调用；未知或恶意列名退回创建时间，同值按 ID 排序，常量列不影响确定性。
func userListOrder(q UserListQuery) string {
	field := map[string]string{"user_id": "id", "user_email": "email", "user_alias": "name", "user_role": "role", "status": "status", "spend": "spend", "max_budget": "max_budget", "created_at": "created_at", "updated_at": "updated_at"}[q.SortBy]
	if field == "" {
		field = "created_at"
	}
	direction := " DESC"
	if q.SortAsc {
		direction = " ASC"
	}
	if field == "id" {
		return field + direction
	}
	return field + direction + ", id" + direction
}

// ListUsersPage 在同一授权和筛选条件下计数后取页；ctx 支持取消，q 指定范围、排序及偏移。
// 返回用户、完整总数、错误；空/越界页返回空切片，页大小默认100且上限500，负偏移归零。
// 用户列表路由调用；只读数据库，SQL错误向上传递，不修改权限或用户记录。
func (db *DB) ListUsersPage(ctx context.Context, q UserListQuery) ([]User, int64, error) {
	q.Search, q.Email = strings.TrimSpace(q.Search), strings.TrimSpace(q.Email)
	if q.Limit <= 0 || q.Limit > 500 {
		q.Limit = 100
	}
	if q.Offset < 0 {
		q.Offset = 0
	}
	count := db.userListSession(ctx, q)
	defer count.Close()
	total, err := count.Count(new(User))
	if err != nil {
		return nil, 0, err
	}
	rows := []User{}
	if int64(q.Offset) >= total {
		return rows, total, nil
	}
	page := db.userListSession(ctx, q)
	defer page.Close()
	err = page.OrderBy(userListOrder(q)).Limit(q.Limit, q.Offset).Find(&rows)
	return rows, total, err
}
