// Package store persists keys and spend logs. It accepts PostgreSQL only and rejects sqlite and an empty database_url.
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

	"github.com/sunqirui1987/xhub/internal/logx"
	"sync"
	"xorm.io/xorm"
)

// Store is the PostgreSQL storage. Reads and writes go through the cached xorm engine. DB is only for connectivity checks and test fixtures.
type Store struct {
	Engine *xorm.Engine
	DB     *sql.DB
	stmts  *int64
}

// Key is a virtual key. An empty Models list means no model restriction. An invalid Blocked value means the block flag was not set.
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

var logTraceOnceStore sync.Once

// Open opens storage. sqlite, file:, and an empty URL are rejected. Only postgres:// and postgresql:// are accepted.
func Open(databaseURL string) (*Store, error) {
	logTraceOnceStore.Do(func() { logx.Trace("enter store.Open") })

	u := strings.ToLower(strings.TrimSpace(databaseURL))
	switch {
	case u == "", strings.HasPrefix(u, "sqlite:"), strings.HasPrefix(u, "file:"):
		return nil, fmt.Errorf("sqlite is not supported; set general_settings.database_url to a postgres:// URL")
	case strings.HasPrefix(u, "postgres://"), strings.HasPrefix(u, "postgresql://"):
		engine, db, stmts, err := openEngine(databaseURL)
		if err != nil {
			return nil, err
		}
		return &Store{Engine: engine, DB: db, stmts: stmts}, nil
	default:
		return nil, fmt.Errorf("database_url must be postgres:// or postgresql://")
	}
}

// HashKey hashes a virtual-key plaintext. The database stores only the hash.
func HashKey(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}

// NewPlainKey generates a new sk- plaintext key. The caller must show it only once.
func NewPlainKey() string {
	var b [24]byte
	_, _ = rand.Read(b[:])
	return "sk-" + hex.EncodeToString(b[:])
}

// Models returns the model names this key allows. An empty list means no restriction.
func (k Key) Models() []string {
	var m []string
	_ = json.Unmarshal([]byte(k.ModelsJSON), &m)
	if m == nil {
		return []string{}
	}
	return m
}

// AllowsModel reports whether this key allows the model. An empty list allows every model.
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

// nullFloat is the database argument for a nullable float. An invalid value is nil.
func nullFloat(v sql.NullFloat64) any {
	if !v.Valid {
		return nil
	}
	return v.Float64
}

// nullInt is the database argument for a nullable integer. An invalid value is nil.
func nullInt(v sql.NullInt64) any {
	if !v.Valid {
		return nil
	}
	return v.Int64
}

// boolInt stores a bool as 0 or 1.
func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

// KeyNameFromPlain builds the short display name from a plaintext key.
func KeyNameFromPlain(plain string) string {
	if len(plain) < 8 {
		return plain
	}
	return plain[:6] + "..." + plain[len(plain)-4:]
}

// ResetAtFrom computes the budget reset time. An explicit time wins, otherwise it is derived from duration. Both empty stays invalid.
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

// ParseDuration parses 30s, 30m, 30h, 30d, and 1mo. An unparseable value returns an error.
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
