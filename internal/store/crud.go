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
// 参数 beans（...any）：bust接到的动态值。类型在函数体内收窄。
// 返回：无。这些表的查询缓存和行缓存已清掉，带模式名和不带模式名的都清。
// 调用：store/keys.go
// 测试：无直接单测
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
// 返回 error（error）：失败原因，nil 表示这一步成功。
// 调用：仅在 crud.go 内使用
// 测试：无直接单测
// 参数 n（int64）：数量；err（error）：失败原因，nil 表示成功。
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
// 参数 kind（string）：分类名，用来选择限额主体、日志类型或官方端点；id（string）：放入KV使用的主键。空串表示调用方没有指定记录；body（string）：已解析或原始的 JSON。
// 返回 error（error）：失败原因，nil 表示这一步成功。
// 调用：gateway/access.go、gateway/config_overrides.go、gateway/family/handlers.go、gateway/models/builtin.go
// 测试：guard_test.go、guardrail_block_test.go
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
// 调用：store/keys.go
// 测试：无直接单测
// 参数 kind（string）：行类型，例如 session 或 credential。id（string）：该类型下的主键。
// 返回：xorm 复合主键，顺序与表上的主键列一致。
func coreIDs(kind, id string) schemas.PK {
	return schemas.PK{kind, id}
}

// GetKV reads one key-value row by kind and id.  The read bypasses the query cache. A session or credential revoked by another gateway process must be observed here, and local cache invalidation cannot provide that guarantee.
// 参数 kind（string）：行类型。id（string）：该类型下的主键。
// 返回：这一行 JSON 解出的对象。没有这一行时 error 为 sql.ErrNoRows，对象为 nil。读走 NoCache，能看见别的进程刚删掉的会话。
// 调用：gateway 读会话、凭证和配置覆盖。
// 测试：builtin_providers_test.go、guard_test.go。
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
// 返回 error（error）：失败原因，nil 表示这一步成功。
// 调用：gateway/access.go、gateway/config_overrides.go、gateway/family/handlers.go、gateway/session.go
// 测试：无直接单测
// 参数 kind（string）：行类型。id（string）：要删的主键。
func (s *Store) DeleteKV(kind, id string) error {
	n, err := s.Engine.ID(coreIDs(kind, id)).Delete(&kvRow{})
	s.bust(new(kvRow))
	return rowsAffected(n, err)
}

// ListKV lists every key-value row of one kind.
// 参数 kind（string）：要列出的行类型。
// 返回：该类型下每一行的 JSON 对象。库错误时切片为 nil。Store 为 nil 时返回空切片和 nil 错误。
// 调用：gateway 列出某类键值行。
// 测试：无直接单测。
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
