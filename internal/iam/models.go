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
	OrgAdmin     = "org_admin"
	OrgMember    = "member"

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

func (User) TableName() string { return "users" }

// Active reports an account that may sign in and use keys.
func (u *User) Active() bool { return u != nil && u.Status == StatusActive }

// Admin reports an active platform administrator.
func (u *User) Admin() bool { return u.Active() && u.Role == RoleAdmin }

// Organization groups teams. Only platform administrators manage it.
type Organization struct {
	ID        string    `xorm:"pk 'id'" json:"id"`
	Name      string    `xorm:"'name'" json:"name"`
	Status    string    `xorm:"'status'" json:"status"`
	MaxBudget *float64  `xorm:"'max_budget'" json:"max_budget"`
	Spend     float64   `xorm:"'spend'" json:"spend"`
	CreatedAt time.Time `xorm:"created 'created_at'" json:"created_at"`
	UpdatedAt time.Time `xorm:"updated 'updated_at'" json:"-"`
}

func (Organization) TableName() string { return "organizations" }

// Team belongs to exactly one organization.
type Team struct {
	ID             string    `xorm:"pk 'id'" json:"id"`
	OrganizationID string    `xorm:"'organization_id'" json:"organization_id"`
	Name           string    `xorm:"'name'" json:"name"`
	Description    string    `xorm:"'description'" json:"description"`
	Status         string    `xorm:"'status'" json:"status"`
	// Models is the team's model set and the ceiling for everything under it.
	// An empty list means no restriction, so a team created a moment ago can
	// reach the deployment's models; a team that must reach nothing is blocked.
	Models    []string  `xorm:"json 'models'" json:"models"`
	MaxBudget *float64  `xorm:"'max_budget'" json:"max_budget"`
	Spend     float64   `xorm:"'spend'" json:"spend"`
	CreatedAt time.Time `xorm:"created 'created_at'" json:"created_at"`
	UpdatedAt time.Time `xorm:"updated 'updated_at'" json:"-"`
}

func (Team) TableName() string { return "teams" }

// TeamMembership is one row of team_members, the only membership source.
type TeamMembership struct {
	TeamID    string    `xorm:"pk 'team_id'"`
	UserID    string    `xorm:"pk 'user_id'"`
	Role      string    `xorm:"'role'"`
	CreatedAt time.Time `xorm:"created 'created_at'"`
}

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

func (Project) TableName() string { return "projects" }

// AuditEntry is one audit-log row.
type AuditEntry struct {
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

func (AuditEntry) TableName() string { return "audit_logs" }

type bootstrapState struct {
	ID          int       `xorm:"pk 'id'"`
	CompletedAt time.Time `xorm:"created 'completed_at'"`
}

func (bootstrapState) TableName() string { return "bootstrap_state" }

// newID returns a random 128-bit hex identifier.
func newID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		logx.Error("iam id generation failed: %v", err)
		panic(err)
	}
	return hex.EncodeToString(b[:])
}
