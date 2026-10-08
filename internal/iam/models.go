package iam

import (
	"crypto/rand"
	"encoding/hex"
	"time"

	"github.com/sunqirui1987/xhub/internal/logx"
)

// Time is an alias used by the join-projection structs below.
type Time = time.Time

// Table beans. Column names and types match schema.sql, which owns the
// constraints; xorm owns every read and write.

const (
	RoleAdmin = "admin"
	RoleUser  = "user"

	TeamAdmin  = "team_admin"
	TeamMember = "member"

	// OrgAdmin administers one organization. It is a membership rather than an
	// account role: the same person can administer one organization and be an
	// ordinary member of another.
	OrgAdmin  = "org_admin"
	OrgMember = "member"

	StatusActive   = "active"
	StatusDisabled = "disabled"
	StatusBlocked  = "blocked"
	StatusRevoked  = "revoked"

	OwnerPersonal = "personal"
	OwnerService  = "service"
	// OwnerSession marks usage produced by a UI session rather than a key. It
	// never appears on api_keys; the database CHECK keeps the two sets apart.
	OwnerSession = "session"
)

// User is a platform account. Role is the only account-level grant.
type User struct {
	ID             string    `xorm:"pk 'id'" json:"id"`
	Email          string    `xorm:"'email'" json:"email"`
	Name           string    `xorm:"'name'" json:"name"`
	PasswordHash   string    `xorm:"'password_hash'" json:"-"`
	Role           string    `xorm:"'role'" json:"role"`
	Status         string    `xorm:"'status'" json:"status"`
	SessionVersion int       `xorm:"'session_version'" json:"-"`
	MaxBudget      *float64  `xorm:"'max_budget'" json:"max_budget"`
	Spend          float64   `xorm:"'spend'" json:"spend"`
	CreatedAt      time.Time `xorm:"created 'created_at'" json:"created_at"`
	UpdatedAt      time.Time `xorm:"updated 'updated_at'" json:"-"`
}

// 告诉 xorm 这个结构体对应数据库表 users。
// 参数：无。
// 返回 string（string）：xorm 使用的表名 users。这个结构体的行都进这张表。
// 调用：xorm 在映射这张表时。
// 测试：无直接单测
func (User) TableName() string { return "users" }

// Active reports an account that may sign in and use keys.
// 参数：无。
// 返回 bool（bool）：记录存在且状态为启用时为真。接收者为 nil 时为假。
// 调用：auth/auth.go、authz/decide.go、gateway/access.go、gateway/limits.go
// 测试：无直接单测
func (u *User) Active() bool { return u != nil && u.Status == StatusActive }

// Admin reports an active platform administrator.
// 参数：无。
// 返回 bool（bool）：这个用户是启用中的平台管理员时返回真。
// 调用：iam/users.go
// 测试：无直接单测
func (u *User) Admin() bool { return u.Active() && u.Role == RoleAdmin }

// Organization groups teams. Only platform administrators manage it.
type Organization struct {
	ID        string   `xorm:"pk 'id'" json:"id"`
	Name      string   `xorm:"'name'" json:"name"`
	Status    string   `xorm:"'status'" json:"status"`
	MaxBudget *float64 `xorm:"'max_budget'" json:"max_budget"`
	Spend     float64  `xorm:"'spend'" json:"spend"`
	// RouteTemplateID is the named router settings this organization selects.
	// Empty means it selects nothing and inherits from the platform default.
	RouteTemplateID *string   `xorm:"'route_template_id'" json:"route_template_id,omitempty"`
	CreatedAt       time.Time `xorm:"created 'created_at'" json:"created_at"`
	UpdatedAt       time.Time `xorm:"updated 'updated_at'" json:"-"`
}

// 告诉 xorm 这个结构体对应数据库表 organizations。
// 参数：无。
// 返回 string（string）：xorm 使用的表名 organizations。这个结构体的行都进这张表。
// 调用：xorm 在映射这张表时。
// 测试：无直接单测
func (Organization) TableName() string { return "organizations" }

// Team belongs to exactly one organization.
type Team struct {
	ID             string `xorm:"pk 'id'" json:"id"`
	OrganizationID string `xorm:"'organization_id'" json:"organization_id"`
	Name           string `xorm:"'name'" json:"name"`
	Description    string `xorm:"'description'" json:"description"`
	Status         string `xorm:"'status'" json:"status"`
	// Models is the team's model set and the ceiling for everything under it.
	// An empty list means no restriction, so a team created a moment ago can
	// reach the deployment's models; a team that must reach nothing is blocked.
	Models    []string `xorm:"json 'models'" json:"models"`
	MaxBudget *float64 `xorm:"'max_budget'" json:"max_budget"`
	Spend     float64  `xorm:"'spend'" json:"spend"`
	// RouteTemplateID is the named router settings this team selects. Empty means
	// it selects nothing and inherits from its organization.
	RouteTemplateID *string   `xorm:"'route_template_id'" json:"route_template_id,omitempty"`
	CreatedAt       time.Time `xorm:"created 'created_at'" json:"created_at"`
	UpdatedAt       time.Time `xorm:"updated 'updated_at'" json:"-"`
}

// 告诉 xorm 这个结构体对应数据库表 teams。
// 参数：无。
// 返回 string（string）：xorm 使用的表名 teams。这个结构体的行都进这张表。
// 调用：xorm 在映射这张表时。
// 测试：无直接单测
func (Team) TableName() string { return "teams" }

// TeamMembership is one row of team_members, the only membership source.
type TeamMembership struct {
	TeamID    string    `xorm:"pk 'team_id'"`
	UserID    string    `xorm:"pk 'user_id'"`
	Role      string    `xorm:"'role'"`
	CreatedAt time.Time `xorm:"created 'created_at'"`
}

// 告诉 xorm 这个结构体对应数据库表 team_members。
// 参数：无。
// 返回 string（string）：xorm 使用的表名 team_members。这个结构体的行都进这张表。
// 调用：xorm 在映射这张表时。
// 测试：无直接单测
func (TeamMembership) TableName() string { return "team_members" }

// OrganizationMembership is one row of organization_members. Only an
// organization administrator has a row that matters; everyone else reaches an
// organization through their teams.
type OrganizationMembership struct {
	OrganizationID string    `xorm:"pk 'organization_id'"`
	UserID         string    `xorm:"pk 'user_id'"`
	Role           string    `xorm:"'role'"`
	CreatedAt      time.Time `xorm:"created 'created_at'"`
}

// 告诉 xorm 这个结构体对应数据库表 organization_members。
// 参数：无。
// 返回 string（string）：xorm 使用的表名 organization_members。这个结构体的行都进这张表。
// 调用：xorm 在映射这张表时。
// 测试：无直接单测
func (OrganizationMembership) TableName() string { return "organization_members" }

// Project belongs to exactly one team; its organization is the team's.
type Project struct {
	ID             string    `xorm:"pk 'id'" json:"id"`
	TeamID         string    `xorm:"'team_id'" json:"team_id"`
	Name           string    `xorm:"'name'" json:"name"`
	Status         string    `xorm:"'status'" json:"status"`
	Models         []string  `xorm:"json 'models'" json:"models"`
	MaxBudget      *float64  `xorm:"'max_budget'" json:"max_budget"`
	Spend          float64   `xorm:"'spend'" json:"spend"`
	CreatedAt      time.Time `xorm:"created 'created_at'" json:"created_at"`
	UpdatedAt      time.Time `xorm:"updated 'updated_at'" json:"-"`
	OrganizationID string    `xorm:"-" json:"organization_id"`
}

// 告诉 xorm 这个结构体对应数据库表 projects。
// 参数：无。
// 返回 string（string）：xorm 使用的表名 projects。这个结构体的行都进这张表。
// 调用：xorm 在映射这张表时。
// 测试：无直接单测
func (Project) TableName() string { return "projects" }

// AuditEntry is one audit-log row.
type AuditEntry struct {
	ActorName  string         `xorm:"-" json:"actor_name"`
	ActorEmail string         `xorm:"-" json:"actor_email"`
	ObjectName string         `xorm:"-" json:"object_name"`
	ID         int64          `xorm:"pk autoincr 'id'" json:"id"`
	TS         time.Time      `xorm:"created 'ts'" json:"ts"`
	ActorID    string         `xorm:"'actor_id'" json:"actor_id"`
	ActorKind  string         `xorm:"'actor_kind'" json:"actor_kind"`
	Action     string         `xorm:"'action'" json:"action"`
	ObjectType string         `xorm:"'object_type'" json:"object_type"`
	ObjectID   string         `xorm:"'object_id'" json:"object_id"`
	TeamID     string         `xorm:"'team_id'" json:"team_id"`
	Detail     map[string]any `xorm:"json 'detail'" json:"detail"`
}

// 告诉 xorm 这个结构体对应数据库表 audit_logs。
// 参数：无。
// 返回 string（string）：xorm 使用的表名 audit_logs。这个结构体的行都进这张表。
// 调用：xorm 在映射这张表时。
// 测试：无直接单测
func (AuditEntry) TableName() string { return "audit_logs" }

type bootstrapState struct {
	ID          int       `xorm:"pk 'id'"`
	CompletedAt time.Time `xorm:"created 'completed_at'"`
}

// 告诉 xorm 这个结构体对应数据库表 bootstrap_state。
// 参数：无。
// 返回 string（string）：xorm 使用的表名 bootstrap_state。这个结构体的行都进这张表。
// 调用：xorm 在映射这张表时。
// 测试：无直接单测
func (bootstrapState) TableName() string { return "bootstrap_state" }

// newID returns a random 128-bit hex identifier.
// 参数：无。
// 返回 string（string）：随机 128 位十六进制 id。
// 调用：iam/keys.go、iam/teams.go、iam/users.go
// 测试：无直接单测
func newID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		logx.Error("iam id generation failed: %v", err)
		panic(err)
	}
	return hex.EncodeToString(b[:])
}
