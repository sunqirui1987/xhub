package store

import (
	"database/sql"
	"encoding/json"
	"time"
)

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

func (e *Entity) SetExtra(m map[string]any) {
	if m == nil {
		e.ExtraJSON = ""
		return
	}
	b, _ := json.Marshal(m)
	e.ExtraJSON = string(b)
}

func (e *Entity) PutExtra(key string, val any) {
	m := e.Extra()
	m[key] = val
	e.SetExtra(m)
}

func (e Entity) ExtraBool(key string) bool {
	v, _ := e.Extra()[key].(bool)
	return v
}

func (e Entity) ExtraList(key string) []any {
	v, _ := e.Extra()[key].([]any)
	if v == nil {
		return []any{}
	}
	return v
}

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

func (s *Store) migrateIdentity() error {
	_, err := s.DB.Exec(`
CREATE TABLE IF NOT EXISTS users (
  user_id TEXT PRIMARY KEY, user_email TEXT, user_role TEXT, user_alias TEXT,
  models_json TEXT NOT NULL DEFAULT '[]', max_budget REAL, spend REAL NOT NULL DEFAULT 0,
  password TEXT, created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS teams (
  team_id TEXT PRIMARY KEY, team_alias TEXT, organization_id TEXT,
  models_json TEXT NOT NULL DEFAULT '[]', max_budget REAL, spend REAL NOT NULL DEFAULT 0, created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS organizations (
  organization_id TEXT PRIMARY KEY, organization_alias TEXT,
  models_json TEXT NOT NULL DEFAULT '[]', max_budget REAL, spend REAL NOT NULL DEFAULT 0, created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS projects (
  project_id TEXT PRIMARY KEY, project_alias TEXT, team_id TEXT,
  models_json TEXT NOT NULL DEFAULT '[]', max_budget REAL, spend REAL NOT NULL DEFAULT 0,
  blocked INTEGER NOT NULL DEFAULT 0, created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS budgets (
  budget_id TEXT PRIMARY KEY, max_budget REAL, soft_budget REAL, tpm_limit INTEGER, rpm_limit INTEGER,
  max_parallel_requests INTEGER, budget_duration TEXT, budget_reset_at TEXT, model_max_budget TEXT, created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS kv (
  kind TEXT NOT NULL, id TEXT NOT NULL, body TEXT NOT NULL, created_at TEXT NOT NULL, PRIMARY KEY (kind, id)
);
`)
	if err != nil {
		return err
	}
	_, _ = s.DB.Exec(`ALTER TABLE users ADD COLUMN password TEXT`)
	_, _ = s.DB.Exec(`ALTER TABLE projects ADD COLUMN blocked INTEGER NOT NULL DEFAULT 0`)
	_, _ = s.DB.Exec(`ALTER TABLE budgets ADD COLUMN soft_budget REAL`)
	_, _ = s.DB.Exec(`ALTER TABLE budgets ADD COLUMN max_parallel_requests INTEGER`)
	_, _ = s.DB.Exec(`ALTER TABLE budgets ADD COLUMN budget_reset_at TEXT`)
	_, _ = s.DB.Exec(`ALTER TABLE budgets ADD COLUMN model_max_budget TEXT`)
	_, _ = s.DB.Exec(`ALTER TABLE users ADD COLUMN extra_json TEXT`)
	_, _ = s.DB.Exec(`ALTER TABLE teams ADD COLUMN extra_json TEXT`)
	_, _ = s.DB.Exec(`ALTER TABLE organizations ADD COLUMN extra_json TEXT`)
	_, _ = s.DB.Exec(`ALTER TABLE projects ADD COLUMN extra_json TEXT`)
	return nil
}

func (e Entity) Models() []string {
	var m []string
	_ = json.Unmarshal([]byte(e.ModelsJSON), &m)
	if m == nil {
		return []string{}
	}
	return m
}

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

func (s *Store) InsertUser(e Entity) error {
	_, err := s.DB.Exec(`INSERT INTO users (user_id,user_email,user_role,user_alias,models_json,max_budget,spend,password,created_at,extra_json) VALUES (?,?,?,?,?,?,?,?,?,?)`,
		e.ID, e.Email, e.Role, e.Alias, e.ModelsJSON, nullFloat(e.MaxBudget), e.Spend, e.Password, e.CreatedAt.UTC().Format(time.RFC3339), e.ExtraJSON)
	return err
}

func (s *Store) ListUsers() ([]Entity, error) {
	return s.list(`SELECT user_id,user_email,user_role,user_alias,models_json,max_budget,spend,created_at,extra_json FROM users`, "user")
}

func (s *Store) GetUser(id string) (*Entity, error) {
	return s.scanUser(`SELECT user_id,user_email,user_role,user_alias,models_json,max_budget,spend,password,created_at,extra_json FROM users WHERE user_id=?`, id)
}

func (s *Store) FindUserLogin(username string) (*Entity, error) {
	e, err := s.GetUser(username)
	if err == nil {
		return e, nil
	}
	return s.scanUser(`SELECT user_id,user_email,user_role,user_alias,models_json,max_budget,spend,password,created_at,extra_json FROM users WHERE lower(user_email)=lower(?)`, username)
}

func (s *Store) scanUser(q string, arg any) (*Entity, error) {
	row := s.DB.QueryRow(q, arg)
	var e Entity
	var created string
	var pw, extra sql.NullString
	if err := row.Scan(&e.ID, &e.Email, &e.Role, &e.Alias, &e.ModelsJSON, &e.MaxBudget, &e.Spend, &pw, &created, &extra); err != nil {
		return nil, err
	}
	if pw.Valid {
		e.Password = pw.String
	}
	if extra.Valid {
		e.ExtraJSON = extra.String
	}
	e.CreatedAt, _ = time.Parse(time.RFC3339, created)
	return &e, nil
}

func (s *Store) UpdateUser(e Entity) error {
	res, err := s.DB.Exec(`UPDATE users SET user_email=?,user_role=?,user_alias=?,models_json=?,max_budget=?,password=?,extra_json=? WHERE user_id=?`,
		e.Email, e.Role, e.Alias, e.ModelsJSON, nullFloat(e.MaxBudget), e.Password, e.ExtraJSON, e.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) DeleteUser(id string) error {
	res, err := s.DB.Exec(`DELETE FROM users WHERE user_id=?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) InsertTeam(e Entity) error {
	_, err := s.DB.Exec(`INSERT INTO teams (team_id,team_alias,organization_id,models_json,max_budget,spend,created_at,extra_json) VALUES (?,?,?,?,?,?,?,?)`,
		e.ID, e.Alias, e.TeamID, e.ModelsJSON, nullFloat(e.MaxBudget), e.Spend, e.CreatedAt.UTC().Format(time.RFC3339), e.ExtraJSON)
	return err
}

func (s *Store) ListTeams() ([]Entity, error) {
	rows, err := s.DB.Query(`SELECT team_id,team_alias,organization_id,models_json,max_budget,spend,created_at,extra_json FROM teams`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Entity
	for rows.Next() {
		var e Entity
		var created string
		var extra sql.NullString
		if err := rows.Scan(&e.ID, &e.Alias, &e.TeamID, &e.ModelsJSON, &e.MaxBudget, &e.Spend, &created, &extra); err != nil {
			return nil, err
		}
		e.CreatedAt, _ = time.Parse(time.RFC3339, created)
		if extra.Valid {
			e.ExtraJSON = extra.String
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Store) GetTeam(id string) (*Entity, error) {
	row := s.DB.QueryRow(`SELECT team_id,team_alias,organization_id,models_json,max_budget,spend,created_at,extra_json FROM teams WHERE team_id=?`, id)
	var e Entity
	var created string
	var extra sql.NullString
	if err := row.Scan(&e.ID, &e.Alias, &e.TeamID, &e.ModelsJSON, &e.MaxBudget, &e.Spend, &created, &extra); err != nil {
		return nil, err
	}
	e.CreatedAt, _ = time.Parse(time.RFC3339, created)
	if extra.Valid {
		e.ExtraJSON = extra.String
	}
	return &e, nil
}

func (s *Store) UpdateTeam(e Entity) error {
	res, err := s.DB.Exec(`UPDATE teams SET team_alias=?,organization_id=?,models_json=?,max_budget=?,extra_json=? WHERE team_id=?`,
		e.Alias, e.TeamID, e.ModelsJSON, nullFloat(e.MaxBudget), e.ExtraJSON, e.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) DeleteTeam(id string) error {
	res, err := s.DB.Exec(`DELETE FROM teams WHERE team_id=?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) AddTeamSpend(id string, delta float64) error {
	_, err := s.DB.Exec(`UPDATE teams SET spend = spend + ? WHERE team_id=?`, delta, id)
	return err
}

func (s *Store) GetOrg(id string) (*Entity, error) {
	return s.getOne(`SELECT organization_id,'','',organization_alias,models_json,max_budget,spend,created_at,extra_json FROM organizations WHERE organization_id=?`, id, "org")
}

func (s *Store) AddUserSpend(id string, delta float64) error {
	_, err := s.DB.Exec(`UPDATE users SET spend = spend + ? WHERE user_id=?`, delta, id)
	return err
}

func (s *Store) AddOrgSpend(id string, delta float64) error {
	_, err := s.DB.Exec(`UPDATE organizations SET spend = spend + ? WHERE organization_id=?`, delta, id)
	return err
}

func (s *Store) InsertOrg(e Entity) error {
	_, err := s.DB.Exec(`INSERT INTO organizations (organization_id,organization_alias,models_json,max_budget,spend,created_at,extra_json) VALUES (?,?,?,?,?,?,?)`,
		e.ID, e.Alias, e.ModelsJSON, nullFloat(e.MaxBudget), e.Spend, e.CreatedAt.UTC().Format(time.RFC3339), e.ExtraJSON)
	return err
}

func (s *Store) ListOrgs() ([]Entity, error) {
	return s.list(`SELECT organization_id,'' ,'',organization_alias,models_json,max_budget,spend,created_at,extra_json FROM organizations`, "org")
}

func (s *Store) UpdateOrg(e Entity) error {
	res, err := s.DB.Exec(`UPDATE organizations SET organization_alias=?,models_json=?,max_budget=?,extra_json=? WHERE organization_id=?`,
		e.Alias, e.ModelsJSON, nullFloat(e.MaxBudget), e.ExtraJSON, e.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) DeleteOrg(id string) error {
	res, err := s.DB.Exec(`DELETE FROM organizations WHERE organization_id=?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) InsertProject(e Entity) error {
	blocked := 0
	if e.Blocked {
		blocked = 1
	}
	_, err := s.DB.Exec(`INSERT INTO projects (project_id,project_alias,team_id,models_json,max_budget,spend,blocked,created_at) VALUES (?,?,?,?,?,?,?,?)`,
		e.ID, e.Alias, e.TeamID, e.ModelsJSON, nullFloat(e.MaxBudget), e.Spend, blocked, e.CreatedAt.UTC().Format(time.RFC3339))
	return err
}

func (s *Store) GetProject(id string) (*Entity, error) {
	row := s.DB.QueryRow(`SELECT project_id,project_alias,team_id,models_json,max_budget,spend,created_at,blocked FROM projects WHERE project_id=?`, id)
	var e Entity
	var created string
	var blocked int
	if err := row.Scan(&e.ID, &e.Alias, &e.TeamID, &e.ModelsJSON, &e.MaxBudget, &e.Spend, &created, &blocked); err != nil {
		return nil, err
	}
	e.Blocked = blocked != 0
	e.CreatedAt, _ = time.Parse(time.RFC3339, created)
	return &e, nil
}

func (s *Store) UpdateProject(e Entity) error {
	blocked := 0
	if e.Blocked {
		blocked = 1
	}
	res, err := s.DB.Exec(`UPDATE projects SET project_alias=?,team_id=?,models_json=?,max_budget=?,blocked=? WHERE project_id=?`,
		e.Alias, e.TeamID, e.ModelsJSON, nullFloat(e.MaxBudget), blocked, e.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) DeleteProject(id string) error {
	res, err := s.DB.Exec(`DELETE FROM projects WHERE project_id=?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) ListProjects() ([]Entity, error) {
	rows, err := s.DB.Query(`SELECT project_id,project_alias,team_id,models_json,max_budget,spend,created_at,blocked FROM projects`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Entity
	for rows.Next() {
		var e Entity
		var created string
		var blocked int
		if err := rows.Scan(&e.ID, &e.Alias, &e.TeamID, &e.ModelsJSON, &e.MaxBudget, &e.Spend, &created, &blocked); err != nil {
			return nil, err
		}
		e.Blocked = blocked != 0
		e.CreatedAt, _ = time.Parse(time.RFC3339, created)
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Store) InsertBudget(b Budget) error {
	reset := ""
	if b.ResetAt.Valid {
		reset = b.ResetAt.Time.UTC().Format(time.RFC3339)
	}
	_, err := s.DB.Exec(`INSERT INTO budgets (budget_id,max_budget,soft_budget,tpm_limit,rpm_limit,max_parallel_requests,budget_duration,budget_reset_at,model_max_budget,created_at) VALUES (?,?,?,?,?,?,?,?,?,?)`,
		b.ID, nullFloat(b.MaxBudget), nullFloat(b.SoftBudget), nullInt(b.TPM), nullInt(b.RPM), nullInt(b.MaxParallel), b.Duration, reset, b.ModelMaxJSON, time.Now().UTC().Format(time.RFC3339))
	return err
}

func (s *Store) ListBudgets() ([]map[string]any, error) {
	rows, err := s.DB.Query(`SELECT budget_id,max_budget,soft_budget,tpm_limit,rpm_limit,max_parallel_requests,budget_duration,budget_reset_at,model_max_budget,created_at FROM budgets`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		b, err := scanBudget(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, budgetMap(*b))
	}
	if out == nil {
		out = []map[string]any{}
	}
	return out, rows.Err()
}

func (s *Store) GetBudget(id string) (*Budget, error) {
	row := s.DB.QueryRow(`SELECT budget_id,max_budget,soft_budget,tpm_limit,rpm_limit,max_parallel_requests,budget_duration,budget_reset_at,model_max_budget,created_at FROM budgets WHERE budget_id=?`, id)
	return scanBudget(row)
}

func (s *Store) UpdateBudget(b Budget) error {
	reset := ""
	if b.ResetAt.Valid {
		reset = b.ResetAt.Time.UTC().Format(time.RFC3339)
	}
	res, err := s.DB.Exec(`UPDATE budgets SET max_budget=?,soft_budget=?,tpm_limit=?,rpm_limit=?,max_parallel_requests=?,budget_duration=?,budget_reset_at=?,model_max_budget=? WHERE budget_id=?`,
		nullFloat(b.MaxBudget), nullFloat(b.SoftBudget), nullInt(b.TPM), nullInt(b.RPM), nullInt(b.MaxParallel), b.Duration, reset, b.ModelMaxJSON, b.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) DeleteBudget(id string) error {
	res, err := s.DB.Exec(`DELETE FROM budgets WHERE budget_id=?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func scanBudget(row rowScanner) (*Budget, error) {
	var b Budget
	var reset, modelMax, created sql.NullString
	if err := row.Scan(&b.ID, &b.MaxBudget, &b.SoftBudget, &b.TPM, &b.RPM, &b.MaxParallel, &b.Duration, &reset, &modelMax, &created); err != nil {
		return nil, err
	}
	if reset.Valid && reset.String != "" {
		if t, err := time.Parse(time.RFC3339, reset.String); err == nil {
			b.ResetAt = sql.NullTime{Time: t, Valid: true}
		}
	}
	if modelMax.Valid {
		b.ModelMaxJSON = modelMax.String
	}
	if created.Valid && created.String != "" {
		if t, err := time.Parse(time.RFC3339, created.String); err == nil {
			b.CreatedAt = t
		}
	}
	return &b, nil
}

func (b Budget) Public() map[string]any {
	return budgetMap(b)
}

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

func (s *Store) PutKV(kind, id, body string) error {
	_, err := s.DB.Exec(`INSERT OR REPLACE INTO kv (kind,id,body,created_at) VALUES (?,?,?,?)`, kind, id, body, time.Now().UTC().Format(time.RFC3339))
	return err
}

func (s *Store) GetKV(kind, id string) (map[string]any, error) {
	row := s.DB.QueryRow(`SELECT body FROM kv WHERE kind=? AND id=?`, kind, id)
	var body string
	if err := row.Scan(&body); err != nil {
		return nil, err
	}
	var m map[string]any
	if json.Unmarshal([]byte(body), &m) != nil {
		m = map[string]any{"id": id, "body": body}
	}
	if m["id"] == nil {
		m["id"] = id
	}
	return m, nil
}

func (s *Store) DeleteKV(kind, id string) error {
	res, err := s.DB.Exec(`DELETE FROM kv WHERE kind=? AND id=?`, kind, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) ListKV(kind string) ([]map[string]any, error) {
	rows, err := s.DB.Query(`SELECT id, body FROM kv WHERE kind=?`, kind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var id, body string
		if err := rows.Scan(&id, &body); err != nil {
			return nil, err
		}
		var m map[string]any
		if json.Unmarshal([]byte(body), &m) != nil {
			m = map[string]any{"id": id, "body": body}
		}
		if m["id"] == nil {
			m["id"] = id
		}
		out = append(out, m)
	}
	if out == nil {
		out = []map[string]any{}
	}
	return out, rows.Err()
}

func (s *Store) ListSpendLogs() ([]map[string]any, error) {
	rows, err := s.DB.Query(`SELECT request_id,call_type,model,api_key,prompt_tokens,completion_tokens,spend,start_time,end_time,cache_hit FROM spend_logs`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var id, ct, model, ak, st, et string
		var pt, ctok, hit int
		var sp float64
		if err := rows.Scan(&id, &ct, &model, &ak, &pt, &ctok, &sp, &st, &et, &hit); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{
			"request_id": id, "call_type": ct, "model": model, "api_key": ak,
			"prompt_tokens": pt, "completion_tokens": ctok, "spend": sp,
			"startTime": st, "endTime": et, "cache_hit": hit != 0,
		})
	}
	if out == nil {
		out = []map[string]any{}
	}
	return out, rows.Err()
}

func (s *Store) list(q, kind string) ([]Entity, error) {
	rows, err := s.DB.Query(q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Entity
	for rows.Next() {
		var e Entity
		var created string
		var extra sql.NullString
		if err := rows.Scan(&e.ID, &e.Email, &e.Role, &e.Alias, &e.ModelsJSON, &e.MaxBudget, &e.Spend, &created, &extra); err != nil {
			return nil, err
		}
		if extra.Valid {
			e.ExtraJSON = extra.String
		}
		e.CreatedAt, _ = time.Parse(time.RFC3339, created)
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Store) getOne(q, id, kind string) (*Entity, error) {
	row := s.DB.QueryRow(q, id)
	var e Entity
	var created string
	var extra sql.NullString
	if err := row.Scan(&e.ID, &e.Email, &e.Role, &e.Alias, &e.ModelsJSON, &e.MaxBudget, &e.Spend, &created, &extra); err != nil {
		return nil, err
	}
	if extra.Valid {
		e.ExtraJSON = extra.String
	}
	e.CreatedAt, _ = time.Parse(time.RFC3339, created)
	return &e, nil
}
