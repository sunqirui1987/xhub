package store

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type Store struct {
	DB    sqlDB
	stmts int64
}

type Key struct {
	TokenHash      string
	KeyAlias       string
	KeyName        string
	UserID         string
	TeamID         string
	OrganizationID string
	ProjectID      string
	AgentID        string
	BudgetID       string
	KeyType        string
	ModelsJSON     string
	MaxBudget      sql.NullFloat64
	SoftBudget     sql.NullFloat64
	Spend          float64
	TPMLimit       sql.NullInt64
	RPMLimit       sql.NullInt64
	MaxParallel    sql.NullInt64
	Blocked        sql.NullBool
	ExpiresAt      sql.NullTime
	BudgetDuration string
	BudgetResetAt  sql.NullTime
	MetadataJSON   string
	TagsJSON       string
	CreatedAt      time.Time
}

func Open(databaseURL string) (*Store, error) {
	u := strings.ToLower(strings.TrimSpace(databaseURL))
	switch {
	case u == "", strings.HasPrefix(u, "sqlite:"), strings.HasPrefix(u, "file:"):
		return nil, fmt.Errorf("sqlite is not supported; set general_settings.database_url to a postgres:// URL")
	case strings.HasPrefix(u, "postgres://"), strings.HasPrefix(u, "postgresql://"):
		return openPostgres(databaseURL)
	default:
		return nil, fmt.Errorf("database_url must be postgres:// or postgresql://")
	}
}

func (s *Store) migrate() error {
	_, err := s.DB.Exec(`
CREATE TABLE IF NOT EXISTS verification_tokens (
  token TEXT PRIMARY KEY,
  key_alias TEXT,
  key_name TEXT,
  user_id TEXT,
  team_id TEXT,
  organization_id TEXT,
  project_id TEXT,
  agent_id TEXT,
  budget_id TEXT,
  key_type TEXT NOT NULL DEFAULT 'default',
  models_json TEXT NOT NULL DEFAULT '[]',
  max_budget DOUBLE PRECISION,
  soft_budget DOUBLE PRECISION,
  spend DOUBLE PRECISION NOT NULL DEFAULT 0,
  tpm_limit INTEGER,
  rpm_limit INTEGER,
  max_parallel_requests INTEGER,
  blocked INTEGER,
  expires_at TEXT,
  budget_duration TEXT,
  budget_reset_at TEXT,
  metadata_json TEXT,
  tags_json TEXT,
  created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS spend_logs (
  request_id TEXT PRIMARY KEY,
  call_type TEXT,
  model TEXT,
  api_key TEXT,
  prompt_tokens INTEGER,
  completion_tokens INTEGER,
  spend DOUBLE PRECISION,
  start_time TEXT,
  end_time TEXT,
  cache_hit INTEGER NOT NULL DEFAULT 0,
  status TEXT
);
`)
	if err != nil {
		return err
	}
	for _, q := range []string{
		`ALTER TABLE verification_tokens ADD COLUMN agent_id TEXT`,
		`ALTER TABLE verification_tokens ADD COLUMN budget_id TEXT`,
		`ALTER TABLE verification_tokens ADD COLUMN soft_budget DOUBLE PRECISION`,
		`ALTER TABLE verification_tokens ADD COLUMN budget_duration TEXT`,
		`ALTER TABLE verification_tokens ADD COLUMN budget_reset_at TEXT`,
		`ALTER TABLE verification_tokens ADD COLUMN metadata_json TEXT`,
		`ALTER TABLE verification_tokens ADD COLUMN tags_json TEXT`,
		`ALTER TABLE spend_logs ADD COLUMN status TEXT`,
	} {
		_, _ = s.DB.Exec(q)
	}
	return nil
}

// relaxBlockedNull lets an omitted blocked field stay NULL, matching LiteLLM.
// SQLite cannot drop NOT NULL in place, so the table is rebuilt when needed.
func (s *Store) relaxBlockedNull() error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`CREATE TABLE verification_tokens_relaxed (
  token TEXT PRIMARY KEY,
  key_alias TEXT,
  key_name TEXT,
  user_id TEXT,
  team_id TEXT,
  organization_id TEXT,
  project_id TEXT,
  agent_id TEXT,
  budget_id TEXT,
  key_type TEXT NOT NULL DEFAULT 'default',
  models_json TEXT NOT NULL DEFAULT '[]',
  max_budget DOUBLE PRECISION,
  soft_budget DOUBLE PRECISION,
  spend DOUBLE PRECISION NOT NULL DEFAULT 0,
  tpm_limit INTEGER,
  rpm_limit INTEGER,
  max_parallel_requests INTEGER,
  blocked INTEGER,
  expires_at TEXT,
  budget_duration TEXT,
  budget_reset_at TEXT,
  metadata_json TEXT,
  tags_json TEXT,
  created_at TEXT NOT NULL
)`); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO verification_tokens_relaxed (
		token, key_alias, key_name, user_id, team_id, organization_id, project_id, agent_id, budget_id,
		key_type, models_json, max_budget, soft_budget, spend, tpm_limit, rpm_limit, max_parallel_requests,
		blocked, expires_at, budget_duration, budget_reset_at, metadata_json, tags_json, created_at
	) SELECT
		token, key_alias, key_name, user_id, team_id, organization_id, project_id, agent_id, budget_id,
		key_type, models_json, max_budget, soft_budget, spend, tpm_limit, rpm_limit, max_parallel_requests,
		blocked, expires_at, budget_duration, budget_reset_at, metadata_json, tags_json, created_at
	FROM verification_tokens`); err != nil {
		return err
	}
	if _, err := tx.Exec(`DROP TABLE verification_tokens`); err != nil {
		return err
	}
	if _, err := tx.Exec(`ALTER TABLE verification_tokens_relaxed RENAME TO verification_tokens`); err != nil {
		return err
	}
	return tx.Commit()
}

func HashKey(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}

func NewPlainKey() string {
	var b [24]byte
	_, _ = rand.Read(b[:])
	return "sk-" + hex.EncodeToString(b[:])
}

const keySelect = `token, key_alias, key_name, user_id, team_id, organization_id, project_id, agent_id, budget_id,
		key_type, models_json, max_budget, soft_budget, spend, tpm_limit, rpm_limit, max_parallel_requests,
		blocked, expires_at, budget_duration, budget_reset_at, metadata_json, tags_json, created_at`

func (s *Store) InsertKey(k Key) error {
	var exp any
	if k.ExpiresAt.Valid {
		exp = k.ExpiresAt.Time.UTC().Format(time.RFC3339)
	}
	reset := ""
	if k.BudgetResetAt.Valid {
		reset = k.BudgetResetAt.Time.UTC().Format(time.RFC3339)
	}
	_, err := s.DB.Exec(`INSERT INTO verification_tokens (
		token, key_alias, key_name, user_id, team_id, organization_id, project_id, agent_id, budget_id,
		key_type, models_json, max_budget, soft_budget, spend, tpm_limit, rpm_limit, max_parallel_requests,
		blocked, expires_at, budget_duration, budget_reset_at, metadata_json, tags_json, created_at
	) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		k.TokenHash, k.KeyAlias, k.KeyName, k.UserID, k.TeamID, k.OrganizationID, k.ProjectID, k.AgentID, k.BudgetID,
		k.KeyType, k.ModelsJSON, nullFloat(k.MaxBudget), nullFloat(k.SoftBudget), k.Spend, nullInt(k.TPMLimit), nullInt(k.RPMLimit),
		nullInt(k.MaxParallel), nullBoolInt(k.Blocked), exp, k.BudgetDuration, reset, k.MetadataJSON, k.TagsJSON, k.CreatedAt.UTC().Format(time.RFC3339),
	)
	return err
}

func (s *Store) UpdateKey(k Key) error {
	var exp any
	if k.ExpiresAt.Valid {
		exp = k.ExpiresAt.Time.UTC().Format(time.RFC3339)
	}
	reset := ""
	if k.BudgetResetAt.Valid {
		reset = k.BudgetResetAt.Time.UTC().Format(time.RFC3339)
	}
	res, err := s.DB.Exec(`UPDATE verification_tokens SET
		key_alias=?, user_id=?, team_id=?, organization_id=?, project_id=?, agent_id=?, budget_id=?,
		key_type=?, models_json=?, max_budget=?, soft_budget=?, tpm_limit=?, rpm_limit=?, max_parallel_requests=?,
		blocked=?, expires_at=?, budget_duration=?, budget_reset_at=?, metadata_json=?, tags_json=?
		WHERE token=?`,
		k.KeyAlias, k.UserID, k.TeamID, k.OrganizationID, k.ProjectID, k.AgentID, k.BudgetID,
		k.KeyType, k.ModelsJSON, nullFloat(k.MaxBudget), nullFloat(k.SoftBudget), nullInt(k.TPMLimit), nullInt(k.RPMLimit),
		nullInt(k.MaxParallel), nullBoolInt(k.Blocked), exp, k.BudgetDuration, reset, k.MetadataJSON, k.TagsJSON, k.TokenHash,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) GetByHash(hash string) (*Key, error) {
	return s.scanOne(`SELECT `+keySelect+` FROM verification_tokens WHERE token = ?`, hash)
}

func (s *Store) ListKeys() ([]Key, error) {
	rows, err := s.DB.Query(`SELECT ` + keySelect + ` FROM verification_tokens ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Key
	for rows.Next() {
		k, err := scanKey(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *k)
	}
	return out, rows.Err()
}

func (s *Store) DeleteHash(hash string) error {
	_, err := s.DB.Exec(`DELETE FROM verification_tokens WHERE token = ?`, hash)
	return err
}

func (s *Store) SetBlocked(hash string, blocked bool) error {
	_, err := s.DB.Exec(`UPDATE verification_tokens SET blocked = ? WHERE token = ?`, boolInt(blocked), hash)
	return err
}

func nullBoolInt(v sql.NullBool) any {
	if !v.Valid {
		return nil
	}
	return boolInt(v.Bool)
}

func (s *Store) AddSpend(hash string, delta float64) error {
	_, err := s.DB.Exec(`UPDATE verification_tokens SET spend = spend + ? WHERE token = ?`, delta, hash)
	return err
}

func (s *Store) InsertSpendLog(requestID, callType, model, apiKeyHash string, prompt, completion int, spend sql.NullFloat64, start, end time.Time, cacheHit bool, status string) error {
	if status == "" {
		status = "success"
	}
	var spendVal any
	if spend.Valid {
		spendVal = spend.Float64
	}
	_, err := s.DB.Exec(`INSERT INTO spend_logs (
		request_id, call_type, model, api_key, prompt_tokens, completion_tokens, spend, start_time, end_time, cache_hit, status
	) VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		requestID, callType, model, apiKeyHash, prompt, completion, spendVal,
		start.UTC().Format(time.RFC3339Nano), end.UTC().Format(time.RFC3339Nano), boolInt(cacheHit), status,
	)
	return err
}

type rowScanner interface {
	Scan(dest ...any) error
}

func (s *Store) scanOne(q string, arg any) (*Key, error) {
	row := s.DB.QueryRow(q, arg)
	return scanKey(row)
}

func scanKey(row rowScanner) (*Key, error) {
	var k Key
	var exp, created, reset, agent, budgetID, meta, tags, dur sql.NullString
	var blocked sql.NullInt64
	err := row.Scan(&k.TokenHash, &k.KeyAlias, &k.KeyName, &k.UserID, &k.TeamID, &k.OrganizationID, &k.ProjectID,
		&agent, &budgetID, &k.KeyType, &k.ModelsJSON, &k.MaxBudget, &k.SoftBudget, &k.Spend, &k.TPMLimit, &k.RPMLimit, &k.MaxParallel,
		&blocked, &exp, &dur, &reset, &meta, &tags, &created)
	if err != nil {
		return nil, err
	}
	if blocked.Valid {
		k.Blocked = sql.NullBool{Bool: blocked.Int64 != 0, Valid: true}
	}
	k.AgentID = agent.String
	k.BudgetID = budgetID.String
	k.BudgetDuration = dur.String
	k.MetadataJSON = meta.String
	k.TagsJSON = tags.String
	if exp.Valid && exp.String != "" {
		t, err := time.Parse(time.RFC3339, exp.String)
		if err == nil {
			k.ExpiresAt = sql.NullTime{Time: t, Valid: true}
		}
	}
	if reset.Valid && reset.String != "" {
		if t, err := time.Parse(time.RFC3339, reset.String); err == nil {
			k.BudgetResetAt = sql.NullTime{Time: t, Valid: true}
		}
	}
	if created.Valid && created.String != "" {
		if t, err := time.Parse(time.RFC3339, created.String); err == nil {
			k.CreatedAt = t
		}
	}
	return &k, nil
}

func (s *Store) SetSpend(hash string, spend float64) error {
	res, err := s.DB.Exec(`UPDATE verification_tokens SET spend = ? WHERE token = ?`, spend, hash)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (k Key) Models() []string {
	var m []string
	_ = json.Unmarshal([]byte(k.ModelsJSON), &m)
	if m == nil {
		return []string{}
	}
	return m
}

func (k Key) AllowsModel(alias string) bool {
	ms := k.Models()
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

func nullFloat(v sql.NullFloat64) any {
	if !v.Valid {
		return nil
	}
	return v.Float64
}

func nullInt(v sql.NullInt64) any {
	if !v.Valid {
		return nil
	}
	return v.Int64
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func KeyNameFromPlain(plain string) string {
	if len(plain) < 8 {
		return plain
	}
	return plain[:6] + "..." + plain[len(plain)-4:]
}

func ResetAtFrom(explicit, duration string) sql.NullTime {
	explicit = strings.TrimSpace(explicit)
	if explicit != "" {
		if t, err := time.Parse(time.RFC3339, explicit); err == nil {
			return sql.NullTime{Time: t.UTC(), Valid: true}
		}
	}
	t, err := ParseDuration(duration)
	if err != nil {
		return sql.NullTime{}
	}
	return t
}

func ParseDuration(d string) (sql.NullTime, error) {
	d = strings.TrimSpace(d)
	if d == "" {
		return sql.NullTime{}, nil
	}
	now := time.Now().UTC()
	var n int
	var unit string
	_, err := fmt.Sscanf(d, "%d%s", &n, &unit)
	if err != nil {
		return sql.NullTime{}, err
	}
	var exp time.Time
	switch unit {
	case "s":
		exp = now.Add(time.Duration(n) * time.Second)
	case "m":
		exp = now.Add(time.Duration(n) * time.Minute)
	case "h":
		exp = now.Add(time.Duration(n) * time.Hour)
	case "d":
		exp = now.AddDate(0, 0, n)
	default:
		return sql.NullTime{}, fmt.Errorf("invalid duration")
	}
	return sql.NullTime{Time: exp, Valid: true}, nil
}
