// 现网表的行结构。列类型跟 PostgreSQL 里已经在用的表对齐。
package store

import "time"

type userRow struct {
	UserID     string   `xorm:"pk text 'user_id'"`
	UserEmail  string   `xorm:"text 'user_email'"`
	UserRole   string   `xorm:"text 'user_role'"`
	UserAlias  string   `xorm:"text 'user_alias'"`
	ModelsJSON string   `xorm:"text 'models_json'"`
	MaxBudget  *float64 `xorm:"'max_budget'"`
	Spend      float64  `xorm:"'spend'"`
	Password   string   `xorm:"text 'password'"`
	CreatedAt  string   `xorm:"text 'created_at'"`
	ExtraJSON  string   `xorm:"text 'extra_json'"`
}

// 用户表名。
func (userRow) TableName() string { return "users" }

type teamRow struct {
	TeamID         string   `xorm:"pk text 'team_id'"`
	TeamAlias      string   `xorm:"text 'team_alias'"`
	OrganizationID string   `xorm:"text 'organization_id'"`
	ModelsJSON     string   `xorm:"text 'models_json'"`
	MaxBudget      *float64 `xorm:"'max_budget'"`
	Spend          float64  `xorm:"'spend'"`
	CreatedAt      string   `xorm:"text 'created_at'"`
	ExtraJSON      string   `xorm:"text 'extra_json'"`
}

// 团队表名。
func (teamRow) TableName() string { return "teams" }

type orgRow struct {
	ID        string    `xorm:"pk text 'id'"`
	Name      string    `xorm:"text 'name'"`
	Status    string    `xorm:"text 'status'"`
	CreatedAt time.Time `xorm:"'created_at'"`
	UpdatedAt time.Time `xorm:"'updated_at'"`
	ExtraJSON string    `xorm:"text 'extra_json'"`
}

// 组织表名。
func (orgRow) TableName() string { return "organizations" }

type projectRow struct {
	ID             string    `xorm:"pk text 'id'"`
	OrganizationID string    `xorm:"text 'organization_id'"`
	Name           string    `xorm:"text 'name'"`
	Environment    string    `xorm:"text 'environment'"`
	CreatedAt      time.Time `xorm:"'created_at'"`
	UpdatedAt      time.Time `xorm:"'updated_at'"`
	Blocked        int       `xorm:"'blocked'"`
	ExtraJSON      string    `xorm:"text 'extra_json'"`
}

// 项目表名。
func (projectRow) TableName() string { return "projects" }

type budgetRow struct {
	ID               string    `xorm:"pk text 'id'"`
	OrganizationID   string    `xorm:"text 'organization_id'"`
	ProjectID        string    `xorm:"text 'project_id'"`
	Name             string    `xorm:"text 'name'"`
	Status           string    `xorm:"text 'status'"`
	Version          string    `xorm:"text 'version'"`
	ScopeType        string    `xorm:"text 'scope_type'"`
	ScopeID          string    `xorm:"text 'scope_id'"`
	LimitAmount      float64   `xorm:"numeric 'limit_amount'"`
	SpentAmount      float64   `xorm:"numeric 'spent_amount'"`
	Currency         string    `xorm:"text 'currency'"`
	Period           string    `xorm:"text 'period'"`
	SoftLimitPercent *int      `xorm:"'soft_limit_percent'"`
	OveragePolicy    string    `xorm:"text 'overage_policy'"`
	CreatedAt        time.Time `xorm:"'created_at'"`
	UpdatedAt        time.Time `xorm:"'updated_at'"`
	SoftBudget       *float64  `xorm:"'soft_budget'"`
	MaxParallel      *int      `xorm:"'max_parallel_requests'"`
	BudgetResetAt    string    `xorm:"text 'budget_reset_at'"`
	ModelMaxBudget   string    `xorm:"text 'model_max_budget'"`
	TPM              *int      `xorm:"'tpm_limit'"`
	RPM              *int      `xorm:"'rpm_limit'"`
	Duration         string    `xorm:"text 'budget_duration'"`
}

// 预算表名。
func (budgetRow) TableName() string { return "budgets" }

type kvRow struct {
	Kind      string `xorm:"pk text 'kind'"`
	ID        string `xorm:"pk text 'id'"`
	Body      string `xorm:"text 'body'"`
	CreatedAt string `xorm:"text 'created_at'"`
}

// 键值表名。
func (kvRow) TableName() string { return "kv" }

type tokenRow struct {
	Token        string   `xorm:"pk text 'token'"`
	KeyAlias     string   `xorm:"text 'key_alias'"`
	KeyName      string   `xorm:"text 'key_name'"`
	UserID       string   `xorm:"text 'user_id'"`
	TeamID       string   `xorm:"text 'team_id'"`
	OrgID        string   `xorm:"text 'organization_id'"`
	ProjectID    string   `xorm:"text 'project_id'"`
	AgentID      string   `xorm:"text 'agent_id'"`
	BudgetID     string   `xorm:"text 'budget_id'"`
	KeyType      string   `xorm:"text 'key_type'"`
	ModelsJSON   string   `xorm:"text 'models_json'"`
	MaxBudget    *float64 `xorm:"'max_budget'"`
	SoftBudget   *float64 `xorm:"'soft_budget'"`
	Spend        float64  `xorm:"'spend'"`
	TPM          *int     `xorm:"'tpm_limit'"`
	RPM          *int     `xorm:"'rpm_limit'"`
	MaxParallel  *int     `xorm:"'max_parallel_requests'"`
	Blocked      *int     `xorm:"'blocked'"`
	ExpiresAt    string   `xorm:"text 'expires_at'"`
	Duration     string   `xorm:"text 'budget_duration'"`
	ResetAt      string   `xorm:"text 'budget_reset_at'"`
	MetadataJSON string   `xorm:"text 'metadata_json'"`
	TagsJSON     string   `xorm:"text 'tags_json'"`
	CreatedAt    string   `xorm:"text 'created_at'"`
}

// 密钥表名。
func (tokenRow) TableName() string { return "verification_tokens" }

type spendRow struct {
	RequestID  string   `xorm:"pk text 'request_id'"`
	CallType   string   `xorm:"text 'call_type'"`
	Model      string   `xorm:"text 'model'"`
	APIKey     string   `xorm:"text 'api_key'"`
	Prompt     int      `xorm:"'prompt_tokens'"`
	Completion int      `xorm:"'completion_tokens'"`
	Spend      *float64 `xorm:"'spend'"`
	StartTime  string   `xorm:"text 'start_time'"`
	EndTime    string   `xorm:"text 'end_time'"`
	CacheHit   int      `xorm:"'cache_hit'"`
	Status     string   `xorm:"text 'status'"`
}

// 花费日志表名。
func (spendRow) TableName() string { return "spend_logs" }

type proxyModelRow struct {
	ID        string `xorm:"pk text 'id'"`
	ModelName string `xorm:"text 'model_name'"`
	Params    string `xorm:"text 'litellm_params_json'"`
	Info      string `xorm:"text 'model_info_json'"`
	UpdatedAt string `xorm:"text 'updated_at'"`
}

// 代理模型表名。
func (proxyModelRow) TableName() string { return "proxy_models" }

type configRow struct {
	Namespace string `xorm:"pk text 'namespace'"`
	Key       string `xorm:"pk text 'key'"`
	ValueJSON string `xorm:"text 'value_json'"`
}

// 代理配置表名。
func (configRow) TableName() string { return "proxy_config" }
