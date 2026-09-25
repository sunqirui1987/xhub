// 用户、团队、组织、项目和预算的表。花费累加和模型允许列表也在这里。
package store

import (
	"database/sql"
	"encoding/json"
	"time"
)

// 用户、团队、组织或项目的一行。额外字段在 JSON，不拆成固定列。
type Entity struct {
	ID         string
	Alias      string
	ModelsJSON string
	MaxBudget  sql.NullFloat64
	Spend      float64
	TeamID     string
	Email      string
	Role       string
	Password   string
	Blocked    bool
	ExtraJSON  string
	CreatedAt  time.Time
}

// 解析额外 JSON。空或损坏时返回空表，不返回 nil 给调用方遍历。
func (e Entity) Extra() map[string]any {
	m := map[string]any{}
	if e.ExtraJSON != "" {
		_ = json.Unmarshal([]byte(e.ExtraJSON), &m)
	}
	if m == nil {
		m = map[string]any{}
	}
	return m
}

// 整体替换额外 JSON。
func (e *Entity) SetExtra(m map[string]any) {
	if m == nil {
		e.ExtraJSON = ""
		return
	}
	b, _ := json.Marshal(m)
	e.ExtraJSON = string(b)
}

// 只改额外 JSON 里的一个键，其它键保留。
func (e *Entity) PutExtra(key string, val any) {
	m := e.Extra()
	m[key] = val
	e.SetExtra(m)
}

// 读取额外 JSON 里的布尔值。缺失时为 false。
func (e Entity) ExtraBool(key string) bool {
	v, _ := e.Extra()[key].(bool)
	return v
}

// 读取额外 JSON 里的列表。缺失或类型不对时为空切片。
func (e Entity) ExtraList(key string) []any {
	v, _ := e.Extra()[key].([]any)
	if v == nil {
		return []any{}
	}
	return v
}

// 命名预算。金额用 DOUBLE PRECISION，避免 PostgreSQL 的 REAL 把花费比歪。
type Budget struct {
	ID           string
	MaxBudget    sql.NullFloat64
	SoftBudget   sql.NullFloat64
	TPM          sql.NullInt64
	RPM          sql.NullInt64
	MaxParallel  sql.NullInt64
	Duration     string
	ResetAt      sql.NullTime
	ModelMaxJSON string
	CreatedAt    time.Time
}

// 实体允许的模型名。空列表表示不限制。
func (e Entity) Models() []string {
	var m []string
	_ = json.Unmarshal([]byte(e.ModelsJSON), &m)
	if m == nil {
		return []string{}
	}
	return m
}

// 实体是否允许这个模型。空列表允许全部。
func (e Entity) AllowsModel(alias string) bool {
	ms := e.Models()
	if len(ms) == 0 {
		return true
	}
	for _, x := range ms {
		if x == alias || x == "*" {
			return true
		}
	}
	return false
}

// 预算的对外 JSON。内部空值用 null，而不是省略字段。
func (b Budget) Public() map[string]any {
	return budgetMap(b)
}

// 预算的内部 map。对外仍用 Public。
func budgetMap(b Budget) map[string]any {
	created := time.Now().UTC().Format(time.RFC3339)
	if !b.CreatedAt.IsZero() {
		created = b.CreatedAt.UTC().Format(time.RFC3339)
	}
	item := map[string]any{
		"budget_id":             b.ID,
		"max_budget":            nullFloat(b.MaxBudget),
		"soft_budget":           nullFloat(b.SoftBudget),
		"tpm_limit":             nullInt(b.TPM),
		"rpm_limit":             nullInt(b.RPM),
		"max_parallel_requests": nullInt(b.MaxParallel),
		"budget_duration":       nil,
		"budget_reset_at":       nil,
		"model_max_budget":      nil,
		"created_at":            created,
		"updated_at":            created,
	}
	if b.Duration != "" {
		item["budget_duration"] = b.Duration
	}
	if b.ResetAt.Valid {
		item["budget_reset_at"] = b.ResetAt.Time.UTC().Format(time.RFC3339)
	}
	if b.ModelMaxJSON != "" {
		var mm any
		if json.Unmarshal([]byte(b.ModelMaxJSON), &mm) == nil {
			item["model_max_budget"] = mm
		}
	}
	return item
}
