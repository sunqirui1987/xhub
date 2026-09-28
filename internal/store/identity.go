// Package store holds users, teams, organizations, projects, and budgets. Spend increments and model allow-lists live here too.
package store

import (
	"database/sql"
	"encoding/json"
	"github.com/sunqirui1987/xhub/internal/logx"
	"sync"
	"time"
)

// Entity is one user, team, organization, or project row. Extra fields stay in JSON instead of fixed columns.
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

var logTraceOnceIdentity sync.Once

// Extra parses the extra JSON. An empty or corrupt value returns an empty map, not nil, so the caller can range over it.
func (e Entity) Extra() map[string]any {
	logTraceOnceIdentity.Do(func() { logx.Trace("enter store.Extra") })

	m := map[string]any{}
	if e.ExtraJSON != "" {
		_ = json.Unmarshal([]byte(e.ExtraJSON), &m)
	}
	if m == nil {
		m = map[string]any{}
	}
	return m
}

// SetExtra replaces the extra JSON as a whole. A nil map clears ExtraJSON.
func (e *Entity) SetExtra(m map[string]any) {
	if m == nil {
		e.ExtraJSON = ""
		return
	}
	b, _ := json.Marshal(m)
	e.ExtraJSON = string(b)
}

// PutExtra changes one key in the extra JSON and keeps the other keys.
func (e *Entity) PutExtra(key string, val any) {
	m := e.Extra()
	m[key] = val
	e.SetExtra(m)
}

// ExtraBool reads a boolean from the extra JSON. A missing value is false.
func (e Entity) ExtraBool(key string) bool {
	v, _ := e.Extra()[key].(bool)
	return v
}

// ExtraList reads a list from the extra JSON. A missing or wrong-typed value is an empty slice.
func (e Entity) ExtraList(key string) []any {
	v, _ := e.Extra()[key].([]any)
	if v == nil {
		return []any{}
	}
	return v
}

// Budget is a named budget. Amounts use DOUBLE PRECISION so PostgreSQL REAL does not skew the spend.
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

// Models returns the model names this entity allows. An empty list means no restriction.
func (e Entity) Models() []string {
	var m []string
	_ = json.Unmarshal([]byte(e.ModelsJSON), &m)
	if m == nil {
		return []string{}
	}
	return m
}

// AllowsModel reports whether this entity allows the model. An empty list allows every model.
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

// Public is the budget JSON shown to callers. An internal null stays null instead of omitting the field.
func (b Budget) Public() map[string]any {
	return budgetMap(b)
}

// budgetMap is the internal budget map. Callers outside the package still use Public.
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
