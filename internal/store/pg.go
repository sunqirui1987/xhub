package store

import (
	"database/sql"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// sqlDB is the subset of *sql.DB the store uses. Queries keep '?' placeholders;
// the postgres wrapper rewrites them to $1, $2, ...
type sqlDB interface {
	Exec(query string, args ...any) (sql.Result, error)
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
	Begin() (*sql.Tx, error)
	Ping() error
	Close() error
}

type pgDB struct {
	*sql.DB
}

func (d pgDB) Exec(query string, args ...any) (sql.Result, error) {
	return d.DB.Exec(rewritePlaceholders(query), args...)
}

func (d pgDB) Query(query string, args ...any) (*sql.Rows, error) {
	return d.DB.Query(rewritePlaceholders(query), args...)
}

func (d pgDB) QueryRow(query string, args ...any) *sql.Row {
	return d.DB.QueryRow(rewritePlaceholders(query), args...)
}

func rewritePlaceholders(q string) string {
	var b strings.Builder
	b.Grow(len(q) + 8)
	n := 0
	inQuote := false
	for i := 0; i < len(q); i++ {
		c := q[i]
		if c == '\'' {
			inQuote = !inQuote
			b.WriteByte(c)
			continue
		}
		if c == '?' && !inQuote {
			n++
			b.WriteByte('$')
			b.WriteString(strconv.Itoa(n))
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}

func quoteIdent(name string) (string, error) {
	if name == "" || len(name) > 63 {
		return "", fmt.Errorf("invalid schema name")
	}
	for _, c := range name {
		if c != '_' && (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') {
			return "", fmt.Errorf("invalid schema name")
		}
	}
	return `"` + name + `"`, nil
}

func openPostgres(databaseURL string) (*Store, error) {
	u, err := url.Parse(databaseURL)
	if err != nil {
		return nil, err
	}
	schema := u.Query().Get("search_path")
	adminURL := databaseURL
	if schema != "" {
		ident, err := quoteIdent(schema)
		if err != nil {
			return nil, err
		}
		q := u.Query()
		q.Del("search_path")
		u.RawQuery = q.Encode()
		admin, err := sql.Open("pgx", u.String())
		if err != nil {
			return nil, err
		}
		if _, err := admin.Exec(`CREATE SCHEMA IF NOT EXISTS ` + ident); err != nil {
			admin.Close()
			return nil, err
		}
		admin.Close()
		adminURL = databaseURL
	}
	db, err := sql.Open("pgx", adminURL)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(20)
	s := &Store{DB: pgDB{DB: db}}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	if err := s.migrateIdentity(); err != nil {
		db.Close()
		return nil, err
	}
	if err := s.migrateProxyModels(); err != nil {
		db.Close()
		return nil, err
	}
	if err := s.migrateConfig(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}
