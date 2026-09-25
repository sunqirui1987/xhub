// 把 Redis 里的花费批次写入 PostgreSQL。同一请求重放不会再次累加。
package store

import (
	"database/sql"
	"sync"
	"sync/atomic"
	"time"

	"xorm.io/xorm"
)

// orgSpendMu 把组织 extra_json 里的花费读改写串起来。
// xorm 的 ForUpdate 只对 MySQL 生成锁，PostgreSQL 上两路同时刷会丢掉增量。
var orgSpendMu sync.Mutex

// 一批请求日志里的一行。从 Redis 刷进 PostgreSQL。
type SpendLogRow struct {
	RequestID  string
	CallType   string
	Model      string
	APIKey     string
	Prompt     int
	Completion int
	Spend      sql.NullFloat64
	Start      time.Time
	End        time.Time
	CacheHit   bool
	Status     string
	TeamID     string
	UserID     string
	OrgID      string
}

// 返回打开引擎以来真正执行过的 SQL 条数。
func (s *Store) Statements() int64 {
	if s == nil || s.stmts == nil {
		return 0
	}
	return atomic.LoadInt64(s.stmts)
}

// 把 SQL 计数清零，用来观察下一次读取有没有打到数据库。
func (s *Store) ResetStatements() {
	if s == nil || s.stmts == nil {
		return
	}
	atomic.StoreInt64(s.stmts, 0)
}

// 写入还没见过的请求日志，并把花费加到密钥、团队、用户和组织上。
func (s *Store) ApplySpendBatch(rows []SpendLogRow) error {
	if s == nil {
		return sql.ErrConnDone
	}
	if len(rows) == 0 {
		return nil
	}
	orgSpendMu.Lock()
	defer orgSpendMu.Unlock()
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.RequestID)
	}
	sess := s.Engine.NewSession()
	defer sess.Close()
	if err := sess.Begin(); err != nil {
		return err
	}
	var have []spendRow
	if err := sess.In("request_id", ids).Cols("request_id").Find(&have); err != nil {
		return err
	}
	seen := map[string]struct{}{}
	for _, row := range have {
		seen[row.RequestID] = struct{}{}
	}
	fresh := make([]spendRow, 0, len(rows))
	keys := map[string]float64{}
	teams := map[string]float64{}
	users := map[string]float64{}
	orgs := map[string]float64{}
	for _, row := range rows {
		if _, ok := seen[row.RequestID]; ok || row.RequestID == "" {
			continue
		}
		status := row.Status
		if status == "" {
			status = "success"
		}
		fresh = append(fresh, spendRow{
			RequestID: row.RequestID, CallType: row.CallType, Model: row.Model, APIKey: row.APIKey,
			Prompt: row.Prompt, Completion: row.Completion, Spend: fptr(row.Spend),
			StartTime: row.Start.UTC().Format(time.RFC3339Nano), EndTime: row.End.UTC().Format(time.RFC3339Nano),
			CacheHit: boolInt(row.CacheHit), Status: status,
		})
		if !row.Spend.Valid || row.Spend.Float64 == 0 {
			continue
		}
		if row.APIKey != "" {
			keys[row.APIKey] += row.Spend.Float64
		}
		if row.TeamID != "" {
			teams[row.TeamID] += row.Spend.Float64
		}
		if row.UserID != "" {
			users[row.UserID] += row.Spend.Float64
		}
		if row.OrgID != "" {
			orgs[row.OrgID] += row.Spend.Float64
		}
	}
	if len(fresh) > 0 {
		if _, err := sess.Insert(&fresh); err != nil {
			return err
		}
	}
	if err := addMappedSpend(sess, keys, teams, users, orgs); err != nil {
		return err
	}
	if err := sess.Commit(); err != nil {
		return err
	}
	s.bust(new(spendRow), new(tokenRow), new(teamRow), new(userRow), new(orgRow))
	return nil
}

// 在当前事务里按汇总结果增加花费。组织花费写在 extra_json 里。
func addMappedSpend(sess *xorm.Session, keys, teams, users, orgs map[string]float64) error {
	for id, delta := range keys {
		if _, err := sess.ID(id).Incr("spend", delta).Update(&tokenRow{}); err != nil {
			return err
		}
	}
	for id, delta := range teams {
		if _, err := sess.ID(id).Incr("spend", delta).Update(&teamRow{}); err != nil {
			return err
		}
	}
	for id, delta := range users {
		if _, err := sess.ID(id).Incr("spend", delta).Update(&userRow{}); err != nil {
			return err
		}
	}
	for id, delta := range orgs {
		var row orgRow
		ok, err := sess.ID(id).Get(&row)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		e := orgFrom(row)
		e.Spend += delta
		packed := packOrg(e)
		packed.CreatedAt = row.CreatedAt
		if _, err := sess.ID(id).Cols("extra_json", "updated_at").Update(&packed); err != nil {
			return err
		}
	}
	return nil
}
