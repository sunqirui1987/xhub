// Package store reads and writes users, teams, organizations, projects, budgets, and key-value rows.
package store

import (
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"github.com/sunqirui1987/xhub/internal/logx"
	"sync"
	"xorm.io/builder"
	"xorm.io/xorm/schemas"
)

var logTraceOnceCrud sync.Once

// nz returns the fallback when the string is empty.
func nz(s, fallback string) string {
	logTraceOnceCrud.Do(func() { logx.Trace("enter store.nz") })

	if s == "" {
		return fallback
	}
	return s
}

// fptr turns a nullable float into a pointer. An invalid value is nil.
func fptr(v sql.NullFloat64) *float64 {
	if !v.Valid {
		return nil
	}
	x := v.Float64
	return &x
}

// nullF turns a float pointer back into a nullable float.
func nullF(p *float64) sql.NullFloat64 {
	if p == nil {
		return sql.NullFloat64{}
	}
	return sql.NullFloat64{Float64: *p, Valid: true}
}

// iptr turns a nullable integer into a pointer. An invalid value is nil.
func iptr(v sql.NullInt64) *int {
	if !v.Valid {
		return nil
	}
	x := int(v.Int64)
	return &x
}

// nullI turns an integer pointer back into a nullable integer.
func nullI(p *int) sql.NullInt64 {
	if p == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(*p), Valid: true}
}

// parseRFC parses an RFC3339 time. An empty string returns the zero time.
func parseRFC(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t
	}
	t, _ := time.Parse(time.RFC3339, s)
	return t
}

// one reads one row by primary key.
func (s *Store) one(bean interface{}, id interface{}) (bool, error) {
	return s.Engine.ID(id).Get(bean)
}

// bust clears the query cache and the row cache for these tables, both with and without a schema in the name.
func (s *Store) bust(beans ...interface{}) {
	if s == nil || s.Engine == nil {
		return
	}
	for _, bean := range beans {
		// The statement cache names tables with a schema, and the engine ClearCache does not. Both are cleared.
		for _, withSchema := range []bool{true, false} {
			name := s.Engine.TableName(bean, withSchema)
			cacher := s.Engine.GetCacher(name)
			if cacher == nil {
				continue
			}
			cacher.ClearIds(name)
			cacher.ClearBeans(name)
		}
	}
}

// rowsAffected returns a no-rows error when no row was changed.
func rowsAffected(n int64, err error) error {
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// userFrom turns a user table row into an entity.
func userFrom(r userRow) Entity {
	e := Entity{
		ID: r.UserID, Email: r.UserEmail, Role: r.UserRole, Alias: r.UserAlias,
		ModelsJSON: nz(r.ModelsJSON, "[]"), MaxBudget: nullF(r.MaxBudget), Spend: r.Spend,
		Password: r.Password, ExtraJSON: r.ExtraJSON, CreatedAt: parseRFC(r.CreatedAt),
	}
	return e
}

// userTo turns a user entity into a table row. An empty model list becomes an empty array.
func userTo(e Entity) userRow {
	if e.ModelsJSON == "" {
		e.ModelsJSON = "[]"
	}
	created := e.CreatedAt
	if created.IsZero() {
		created = time.Now().UTC()
	}
	return userRow{
		UserID: e.ID, UserEmail: e.Email, UserRole: e.Role, UserAlias: e.Alias,
		ModelsJSON: e.ModelsJSON, MaxBudget: fptr(e.MaxBudget), Spend: e.Spend,
		Password: e.Password, CreatedAt: created.UTC().Format(time.RFC3339), ExtraJSON: e.ExtraJSON,
	}
}

// InsertUser inserts a user and clears the user cache.
func (s *Store) InsertUser(e Entity) error {
	_, err := s.Engine.Insert(userTo(e))
	s.bust(new(userRow))
	return err
}

// ListUsers lists every user.
func (s *Store) ListUsers() ([]Entity, error) {
	var rows []userRow
	if err := s.Engine.Find(&rows); err != nil {
		return nil, err
	}
	out := make([]Entity, 0, len(rows))
	for _, r := range rows {
		out = append(out, userFrom(r))
	}
	return out, nil
}

// GetUser reads a user by id. A missing row returns a no-rows error.
func (s *Store) GetUser(id string) (*Entity, error) {
	var r userRow
	ok, err := s.one(&r, id)
	if err != nil || !ok {
		if err == nil {
			err = sql.ErrNoRows
		}
		return nil, err
	}
	e := userFrom(r)
	return &e, nil
}

// FindUserLogin looks up a login user by id first and then by email.
func (s *Store) FindUserLogin(username string) (*Entity, error) {
	if e, err := s.GetUser(username); err == nil {
		return e, nil
	}
	var rows []userRow
	if err := s.Engine.Find(&rows); err != nil {
		return nil, err
	}
	for _, r := range rows {
		if strings.EqualFold(r.UserEmail, username) {
			e := userFrom(r)
			return &e, nil
		}
	}
	return nil, sql.ErrNoRows
}

// UpdateUser updates a user profile. A missing row returns a no-rows error.
func (s *Store) UpdateUser(e Entity) error {
	row := userTo(e)
	n, err := s.Engine.ID(e.ID).Cols("user_email", "user_role", "user_alias", "models_json", "max_budget", "password", "extra_json").Update(&row)
	s.bust(new(userRow))
	return rowsAffected(n, err)
}

// DeleteUser deletes a user by id and clears the cache.
func (s *Store) DeleteUser(id string) error {
	n, err := s.Engine.ID(id).Delete(&userRow{})
	s.bust(new(userRow))
	return rowsAffected(n, err)
}

// AddUserSpend adds a spend delta to a user. A missing user returns a no-rows error.
func (s *Store) AddUserSpend(id string, delta float64) error {
	n, err := s.Engine.ID(id).Incr("spend", delta).Update(&userRow{})
	s.bust(new(userRow))
	return rowsAffected(n, err)
}

// teamFrom turns a team table row into an entity. The organization id is stored on TeamID.
func teamFrom(r teamRow) Entity {
	return Entity{
		ID: r.TeamID, Alias: r.TeamAlias, TeamID: r.OrganizationID, ModelsJSON: nz(r.ModelsJSON, "[]"),
		MaxBudget: nullF(r.MaxBudget), Spend: r.Spend, ExtraJSON: r.ExtraJSON, CreatedAt: parseRFC(r.CreatedAt),
	}
}

// teamTo turns a team entity into a table row. TeamID is written as the organization id.
func teamTo(e Entity) teamRow {
	if e.ModelsJSON == "" {
		e.ModelsJSON = "[]"
	}
	created := e.CreatedAt
	if created.IsZero() {
		created = time.Now().UTC()
	}
	return teamRow{
		TeamID: e.ID, TeamAlias: e.Alias, OrganizationID: e.TeamID, ModelsJSON: e.ModelsJSON,
		MaxBudget: fptr(e.MaxBudget), Spend: e.Spend, CreatedAt: created.UTC().Format(time.RFC3339), ExtraJSON: e.ExtraJSON,
	}
}

// InsertTeam inserts a team and clears the team cache.
func (s *Store) InsertTeam(e Entity) error {
	_, err := s.Engine.Insert(teamTo(e))
	s.bust(new(teamRow))
	return err
}

// ListTeams lists every team.
func (s *Store) ListTeams() ([]Entity, error) {
	var rows []teamRow
	if err := s.Engine.Find(&rows); err != nil {
		return nil, err
	}
	out := make([]Entity, 0, len(rows))
	for _, r := range rows {
		out = append(out, teamFrom(r))
	}
	return out, nil
}

// GetTeam reads a team by id. A missing row returns a no-rows error.
func (s *Store) GetTeam(id string) (*Entity, error) {
	var r teamRow
	ok, err := s.one(&r, id)
	if err != nil || !ok {
		if err == nil {
			err = sql.ErrNoRows
		}
		return nil, err
	}
	e := teamFrom(r)
	return &e, nil
}

// UpdateTeam updates a team. A missing row returns a no-rows error.
func (s *Store) UpdateTeam(e Entity) error {
	row := teamTo(e)
	n, err := s.Engine.ID(e.ID).Cols("team_alias", "organization_id", "models_json", "max_budget", "extra_json").Update(&row)
	s.bust(new(teamRow))
	return rowsAffected(n, err)
}

// DeleteTeam deletes a team by id and clears the cache.
func (s *Store) DeleteTeam(id string) error {
	n, err := s.Engine.ID(id).Delete(&teamRow{})
	s.bust(new(teamRow))
	return rowsAffected(n, err)
}

// AddTeamSpend adds a spend delta to a team. A missing team returns a no-rows error.
func (s *Store) AddTeamSpend(id string, delta float64) error {
	n, err := s.Engine.ID(id).Incr("spend", delta).Update(&teamRow{})
	s.bust(new(teamRow))
	return rowsAffected(n, err)
}

// packOrg turns an organization entity into a table row. Models and spend go into extra_json.
func packOrg(e Entity) orgRow {
	m := e.Extra()
	var models any = []any{}
	if e.ModelsJSON != "" {
		_ = json.Unmarshal([]byte(e.ModelsJSON), &models)
	}
	m["models"] = models
	if e.MaxBudget.Valid {
		m["max_budget"] = e.MaxBudget.Float64
	} else {
		delete(m, "max_budget")
	}
	m["spend"] = e.Spend
	e.SetExtra(m)
	if e.Alias == "" {
		e.Alias = e.ID
	}
	created := e.CreatedAt
	if created.IsZero() {
		created = time.Now().UTC()
	}
	return orgRow{ID: e.ID, Name: e.Alias, Status: "active", CreatedAt: created.UTC(), UpdatedAt: time.Now().UTC(), ExtraJSON: e.ExtraJSON}
}

// orgFrom turns an organization table row into an entity. Spend is read from extra_json.
func orgFrom(r orgRow) Entity {
	e := Entity{ID: r.ID, Alias: r.Name, ExtraJSON: r.ExtraJSON, CreatedAt: r.CreatedAt.UTC()}
	m := e.Extra()
	if raw, ok := m["models"]; ok {
		if b, err := json.Marshal(raw); err == nil {
			e.ModelsJSON = string(b)
		}
	}
	if e.ModelsJSON == "" {
		e.ModelsJSON = "[]"
	}
	if v, ok := m["max_budget"].(float64); ok {
		e.MaxBudget = sql.NullFloat64{Float64: v, Valid: true}
	}
	if v, ok := m["spend"].(float64); ok {
		e.Spend = v
	}
	return e
}

// InsertOrg inserts an organization and clears the organization cache.
func (s *Store) InsertOrg(e Entity) error {
	_, err := s.Engine.Insert(packOrg(e))
	s.bust(new(orgRow))
	return err
}

// GetOrg reads an organization by id. A missing row returns a no-rows error.
func (s *Store) GetOrg(id string) (*Entity, error) {
	var r orgRow
	ok, err := s.one(&r, id)
	if err != nil || !ok {
		if err == nil {
			err = sql.ErrNoRows
		}
		return nil, err
	}
	e := orgFrom(r)
	return &e, nil
}

// ListOrgs lists every organization.
func (s *Store) ListOrgs() ([]Entity, error) {
	var rows []orgRow
	if err := s.Engine.Find(&rows); err != nil {
		return nil, err
	}
	out := make([]Entity, 0, len(rows))
	for _, r := range rows {
		out = append(out, orgFrom(r))
	}
	return out, nil
}

// UpdateOrg updates the organization name and extra fields. Spend stays at the current value in the database.
func (s *Store) UpdateOrg(e Entity) error {
	orgSpendMu.Lock()
	defer orgSpendMu.Unlock()
	cur, err := s.GetOrg(e.ID)
	if err != nil {
		return err
	}
	if e.ExtraJSON == "" {
		e.ExtraJSON = cur.ExtraJSON
	}
	// The spend the caller holds may have been read before a flush. An alias update must not write that stale spend back.
	e.Spend = cur.Spend
	return s.writeOrg(e, cur.CreatedAt)
}

// AddOrgSpend adds a spend delta to an organization. It shares the flush lock so a delta is not dropped.
func (s *Store) AddOrgSpend(id string, delta float64) error {
	orgSpendMu.Lock()
	defer orgSpendMu.Unlock()
	e, err := s.GetOrg(id)
	if err != nil {
		return err
	}
	e.Spend += delta
	return s.writeOrg(*e, e.CreatedAt)
}

// writeOrg writes the organization name and extra_json. The caller must already hold the spend lock.
func (s *Store) writeOrg(e Entity, created time.Time) error {
	row := packOrg(e)
	row.CreatedAt = created
	n, err := s.Engine.ID(e.ID).Cols("name", "extra_json", "updated_at").Update(&row)
	s.bust(new(orgRow))
	return rowsAffected(n, err)
}

var controlOrgChildren = []string{
	"request_logs", "audit_events", "budgets", "guardrails", "api_keys",
	"deployments", "models", "providers", "projects",
}

// DeleteOrg deletes an organization and rows in its child tables. Teams are not deleted.
func (s *Store) DeleteOrg(id string) error {
	sess := s.Engine.NewSession()
	defer sess.Close()
	if err := sess.Begin(); err != nil {
		return err
	}
	for _, table := range controlOrgChildren {
		ok, err := s.Engine.IsTableExist(table)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		if _, err := sess.Table(table).Where(builder.Eq{"organization_id": id}).Delete(&struct{ OrganizationID string }{}); err != nil {
			return err
		}
	}
	n, err := sess.ID(id).Delete(&orgRow{})
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	if err := sess.Commit(); err != nil {
		return err
	}
	s.bust(new(orgRow), new(projectRow), new(budgetRow))
	return nil
}

// projectOrgID is the organization a project belongs to. The team's organization wins, then the extra field.
func (s *Store) projectOrgID(e Entity) string {
	if e.TeamID != "" {
		var team teamRow
		ok, err := s.Engine.ID(e.TeamID).Get(&team)
		if err == nil && ok && team.OrganizationID != "" {
			return team.OrganizationID
		}
	}
	if v, ok := e.Extra()["organization_id"].(string); ok {
		return v
	}
	return ""
}

// packProject turns a project entity into a table row. The team and spend go into extra_json.
func packProject(e Entity, orgID string) projectRow {
	m := e.Extra()
	var models any = []any{}
	if e.ModelsJSON != "" {
		_ = json.Unmarshal([]byte(e.ModelsJSON), &models)
	}
	m["models"] = models
	m["team_id"] = e.TeamID
	m["organization_id"] = orgID
	if e.MaxBudget.Valid {
		m["max_budget"] = e.MaxBudget.Float64
	} else {
		delete(m, "max_budget")
	}
	m["spend"] = e.Spend
	e.SetExtra(m)
	if e.Alias == "" {
		e.Alias = e.ID
	}
	created := e.CreatedAt
	if created.IsZero() {
		created = time.Now().UTC()
	}
	blocked := 0
	if e.Blocked {
		blocked = 1
	}
	return projectRow{
		ID: e.ID, OrganizationID: orgID, Name: e.Alias, Environment: "production",
		CreatedAt: created.UTC(), UpdatedAt: time.Now().UTC(), Blocked: blocked, ExtraJSON: e.ExtraJSON,
	}
}

// projectFrom turns a project table row into an entity and puts the organization id back into the extra fields.
func projectFrom(r projectRow) Entity {
	e := Entity{ID: r.ID, Alias: r.Name, Blocked: r.Blocked != 0, ExtraJSON: r.ExtraJSON, CreatedAt: r.CreatedAt.UTC()}
	m := e.Extra()
	if raw, ok := m["models"]; ok {
		if b, err := json.Marshal(raw); err == nil {
			e.ModelsJSON = string(b)
		}
	}
	if e.ModelsJSON == "" {
		e.ModelsJSON = "[]"
	}
	if v, ok := m["team_id"].(string); ok {
		e.TeamID = v
	}
	if v, ok := m["max_budget"].(float64); ok {
		e.MaxBudget = sql.NullFloat64{Float64: v, Valid: true}
	}
	if v, ok := m["spend"].(float64); ok {
		e.Spend = v
	}
	e.PutExtra("organization_id", r.OrganizationID)
	return e
}

// InsertProject inserts a project and clears the project cache.
func (s *Store) InsertProject(e Entity) error {
	_, err := s.Engine.Insert(packProject(e, s.projectOrgID(e)))
	s.bust(new(projectRow))
	return err
}

// GetProject reads a project by id. A missing row returns a no-rows error.
func (s *Store) GetProject(id string) (*Entity, error) {
	var r projectRow
	ok, err := s.one(&r, id)
	if err != nil || !ok {
		if err == nil {
			err = sql.ErrNoRows
		}
		return nil, err
	}
	e := projectFrom(r)
	return &e, nil
}

// ListProjects lists every project.
func (s *Store) ListProjects() ([]Entity, error) {
	var rows []projectRow
	if err := s.Engine.Find(&rows); err != nil {
		return nil, err
	}
	out := make([]Entity, 0, len(rows))
	for _, r := range rows {
		out = append(out, projectFrom(r))
	}
	return out, nil
}

// UpdateProject updates a project. A missing row returns a no-rows error.
func (s *Store) UpdateProject(e Entity) error {
	cur, err := s.GetProject(e.ID)
	if err != nil {
		return err
	}
	if e.ExtraJSON == "" {
		e.ExtraJSON = cur.ExtraJSON
	}
	row := packProject(e, s.projectOrgID(e))
	row.CreatedAt = cur.CreatedAt
	if row.Environment == "" {
		row.Environment = "production"
	}
	n, err := s.Engine.ID(e.ID).Cols("name", "organization_id", "blocked", "extra_json", "updated_at").Update(&row)
	s.bust(new(projectRow))
	return rowsAffected(n, err)
}

// DeleteProject deletes a project by id and clears the cache.
func (s *Store) DeleteProject(id string) error {
	n, err := s.Engine.ID(id).Delete(&projectRow{})
	s.bust(new(projectRow))
	return rowsAffected(n, err)
}

// budgetTo turns a budget into a control-plane table row. The amount is written to limit_amount.
func budgetTo(b Budget) budgetRow {
	limit := 0.0
	if b.MaxBudget.Valid {
		limit = b.MaxBudget.Float64
	}
	created := b.CreatedAt
	if created.IsZero() {
		created = time.Now().UTC()
	}
	row := budgetRow{
		ID: b.ID, Name: nz(b.ID, "budget"), Status: "active", Version: "1",
		ScopeType: "global", ScopeID: nz(b.ID, "budget"), LimitAmount: limit,
		Currency: "USD", Period: nz(b.Duration, "monthly"), OveragePolicy: "block",
		CreatedAt: created.UTC(), UpdatedAt: time.Now().UTC(),
		SoftBudget: fptr(b.SoftBudget), Duration: b.Duration, ModelMaxBudget: b.ModelMaxJSON,
	}
	if b.TPM.Valid {
		v := int(b.TPM.Int64)
		row.TPM = &v
	}
	if b.RPM.Valid {
		v := int(b.RPM.Int64)
		row.RPM = &v
	}
	if b.MaxParallel.Valid {
		v := int(b.MaxParallel.Int64)
		row.MaxParallel = &v
	}
	if b.ResetAt.Valid {
		row.BudgetResetAt = b.ResetAt.Time.UTC().Format(time.RFC3339)
	}
	return row
}

// budgetFrom turns a budget table row back into the budget the gateway uses.
func budgetFrom(r budgetRow) Budget {
	b := Budget{
		ID: r.ID, MaxBudget: sql.NullFloat64{Float64: r.LimitAmount, Valid: true},
		SoftBudget: nullF(r.SoftBudget), TPM: nullI(r.TPM), RPM: nullI(r.RPM),
		MaxParallel: nullI(r.MaxParallel), Duration: r.Duration, ModelMaxJSON: r.ModelMaxBudget,
		CreatedAt: r.CreatedAt.UTC(),
	}
	if r.BudgetResetAt != "" {
		if t := parseRFC(r.BudgetResetAt); !t.IsZero() {
			b.ResetAt = sql.NullTime{Time: t, Valid: true}
		}
	}
	return b
}

// InsertBudget inserts a budget and clears the budget cache.
func (s *Store) InsertBudget(b Budget) error {
	_, err := s.Engine.Insert(budgetTo(b))
	s.bust(new(budgetRow))
	return err
}

// ListBudgets lists every budget using the public field names.
func (s *Store) ListBudgets() ([]map[string]any, error) {
	var rows []budgetRow
	if err := s.Engine.Find(&rows); err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		out = append(out, budgetMap(budgetFrom(r)))
	}
	return out, nil
}

// GetBudget reads a budget by id. A missing row returns a no-rows error.
func (s *Store) GetBudget(id string) (*Budget, error) {
	var r budgetRow
	ok, err := s.one(&r, id)
	if err != nil || !ok {
		if err == nil {
			err = sql.ErrNoRows
		}
		return nil, err
	}
	b := budgetFrom(r)
	return &b, nil
}

// UpdateBudget updates the budget amount, rate limits, and reset time. A missing row returns a no-rows error.
func (s *Store) UpdateBudget(b Budget) error {
	row := budgetTo(b)
	n, err := s.Engine.ID(b.ID).Cols(
		"limit_amount", "soft_budget", "tpm_limit", "rpm_limit", "max_parallel_requests",
		"budget_duration", "period", "budget_reset_at", "model_max_budget", "updated_at",
	).Update(&row)
	s.bust(new(budgetRow))
	return rowsAffected(n, err)
}

// DeleteBudget deletes a budget by id and clears the cache.
func (s *Store) DeleteBudget(id string) error {
	n, err := s.Engine.ID(id).Delete(&budgetRow{})
	s.bust(new(budgetRow))
	return rowsAffected(n, err)
}

// PutKV writes one key-value row. An existing row is updated and a missing row is inserted.
func (s *Store) PutKV(kind, id, body string) error {
	row := kvRow{Kind: kind, ID: id, Body: body, CreatedAt: time.Now().UTC().Format(time.RFC3339)}
	n, err := s.Engine.ID(coreIDs(kind, id)).Cols("body", "created_at").Update(&row)
	if err != nil {
		return err
	}
	if n == 0 {
		_, err = s.Engine.Insert(&row)
	}
	s.bust(new(kvRow))
	return err
}

// coreIDs builds the composite primary key. The order matches the primary-key columns in the table.
func coreIDs(kind, id string) schemas.PK {
	return schemas.PK{kind, id}
}

// GetKV reads one key-value row by kind and id.
func (s *Store) GetKV(kind, id string) (map[string]any, error) {
	var r kvRow
	ok, err := s.Engine.ID(coreIDs(kind, id)).Get(&r)
	if err != nil || !ok {
		if err == nil {
			err = sql.ErrNoRows
		}
		return nil, err
	}
	var m map[string]any
	if json.Unmarshal([]byte(r.Body), &m) != nil {
		m = map[string]any{"id": id, "body": r.Body}
	}
	if m["id"] == nil {
		m["id"] = id
	}
	return m, nil
}

// DeleteKV deletes one key-value row and clears the cache. A missing row returns a no-rows error.
func (s *Store) DeleteKV(kind, id string) error {
	n, err := s.Engine.ID(coreIDs(kind, id)).Delete(&kvRow{})
	s.bust(new(kvRow))
	return rowsAffected(n, err)
}

// ListKV lists every key-value row of one kind.
func (s *Store) ListKV(kind string) ([]map[string]any, error) {
	var rows []kvRow
	if err := s.Engine.Where(builder.Eq{"kind": kind}).Find(&rows); err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		var m map[string]any
		if json.Unmarshal([]byte(r.Body), &m) != nil {
			m = map[string]any{"id": r.ID, "body": r.Body}
		}
		if m["id"] == nil {
			m["id"] = r.ID
		}
		out = append(out, m)
	}
	return out, nil
}
