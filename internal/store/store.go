// 密钥和花费日志的持久化。只接受 PostgreSQL，拒绝 sqlite 和空的 database_url。
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

	"xorm.io/xorm"
)

// PostgreSQL 存储。读写都走带缓存的 xorm 引擎。DB 只留给连通性检查和测试夹具。
type Store struct {
	Engine *xorm.Engine
	DB     *sql.DB
	stmts  *int64
}

// 虚拟密钥。Models 为空表示不限制模型。Blocked 无效时表示未设置屏蔽。
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

// 打开存储。sqlite、file: 和空 URL 直接拒绝，只接受 postgres:// 或 postgresql://。
func Open(databaseURL string) (*Store, error) {
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

// 虚拟密钥明文的哈希。数据库里只存哈希。
func HashKey(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}

// 生成新的 sk- 明文密钥。调用方负责只展示一次。
func NewPlainKey() string {
	var b [24]byte
	_, _ = rand.Read(b[:])
	return "sk-" + hex.EncodeToString(b[:])
}

// 密钥允许的模型名。空列表表示不限制。
func (k Key) Models() []string {
	var m []string
	_ = json.Unmarshal([]byte(k.ModelsJSON), &m)
	if m == nil {
		return []string{}
	}
	return m
}

// 密钥是否允许这个模型。空列表允许全部。
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

// 可空浮点的数据库参数。
func nullFloat(v sql.NullFloat64) any {
	if !v.Valid {
		return nil
	}
	return v.Float64
}

// 可空整数的数据库参数。
func nullInt(v sql.NullInt64) any {
	if !v.Valid {
		return nil
	}
	return v.Int64
}

// 布尔存成 0 或 1。
func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

// 用明文密钥生成可展示的短名称。
func KeyNameFromPlain(plain string) string {
	if len(plain) < 8 {
		return plain
	}
	return plain[:6] + "..." + plain[len(plain)-4:]
}

// 计算预算重置时间。显式时间优先，否则按 duration 推算。两者都空则无效。
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

// 解析 30s、30m、30h、30d、1mo。无法解析时返回错误。
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
