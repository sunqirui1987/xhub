// Package store reads and writes key-value documents: UI settings, credentials,
// invitations, cache settings, and the session records the console signs in
// with.
package store

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/sunqirui1987/xhub/internal/logx"
	"sync"
	"xorm.io/builder"
	"xorm.io/xorm/schemas"
)

var logTraceOnceCrud sync.Once

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

// PutKV writes one key-value row. An existing row is updated and a missing row is inserted.
func (s *Store) PutKV(kind, id, body string) error {
	logTraceOnceCrud.Do(func() { logx.Trace("enter store.PutKV") })

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
//
// The read bypasses the query cache. A session or credential revoked by another
// gateway process must be observed here, and local cache invalidation cannot
// provide that guarantee.
func (s *Store) GetKV(kind, id string) (map[string]any, error) {
	if s == nil {
		return nil, sql.ErrNoRows
	}
	var r kvRow
	ok, err := s.Engine.NoCache().ID(coreIDs(kind, id)).Get(&r)
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
	if s == nil {
		return nil, nil
	}
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
