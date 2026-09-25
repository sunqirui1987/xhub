// 用户、团队、组织、项目、预算和键值的读写。
package store

import (
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"xorm.io/builder"
	"xorm.io/xorm/schemas"
)

// 空字符串时用备用值。
func nz(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

// 把可空浮点收成指针。无效时为 nil。
func fptr(v sql.NullFloat64) *float64 {
	if !v.Valid {
		return nil
	}
	x := v.Float64
	return &x
}

// 把浮点指针还原成可空浮点。
func nullF(p *float64) sql.NullFloat64 {
	if p == nil {
		return sql.NullFloat64{}
	}
	return sql.NullFloat64{Float64: *p, Valid: true}
}

// 把可空整数收成指针。无效时为 nil。
func iptr(v sql.NullInt64) *int {
	if !v.Valid {
		return nil
	}
	x := int(v.Int64)
	return &x
}

// 把整数指针还原成可空整数。
func nullI(p *int) sql.NullInt64 {
	if p == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(*p), Valid: true}
}

// 解析 RFC3339 时间。空字符串得到零时间。
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

// 按主键读一行。
func (s *Store) one(bean interface{}, id interface{}) (bool, error) {
	return s.Engine.ID(id).Get(bean)
}

// 清掉这些表的查询缓存和行缓存。表名带不带 schema 都清。
func (s *Store) bust(beans ...interface{}) {
	if s == nil || s.Engine == nil {
		return
	}
	for _, bean := range beans {
		// 语句缓存的表名带 schema，引擎自带的 ClearCache 不带。两边都清。
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

// 没有改到行时返回无行错误。
func rowsAffected(n int64, err error) error {
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// 把用户表行转成实体。
func userFrom(r userRow) Entity {
	e := Entity{
		ID: r.UserID, Email: r.UserEmail, Role: r.UserRole, Alias: r.UserAlias,
		ModelsJSON: nz(r.ModelsJSON, "[]"), MaxBudget: nullF(r.MaxBudget), Spend: r.Spend,
		Password: r.Password, ExtraJSON: r.ExtraJSON, CreatedAt: parseRFC(r.CreatedAt),
	}
	return e
}

// 把用户实体转成表行。空模型列表补成空数组。
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

// 插入用户，并清掉用户缓存。
func (s *Store) InsertUser(e Entity) error {
	_, err := s.Engine.Insert(userTo(e))
	s.bust(new(userRow))
	return err
}

// 列出全部用户。
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

// 按 id 读取用户。没有这一行时返回无行错误。
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

// 先按 id 找登录用户，找不到再按邮箱找。
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

// 更新用户资料。找不到返回无行。
func (s *Store) UpdateUser(e Entity) error {
	row := userTo(e)
	n, err := s.Engine.ID(e.ID).Cols("user_email", "user_role", "user_alias", "models_json", "max_budget", "password", "extra_json").Update(&row)
	s.bust(new(userRow))
	return rowsAffected(n, err)
}

// 按 id 删除用户，并清掉缓存。
func (s *Store) DeleteUser(id string) error {
	n, err := s.Engine.ID(id).Delete(&userRow{})
	s.bust(new(userRow))
	return rowsAffected(n, err)
}

// 给用户加上一笔花费。用户不存在时返回无行。
func (s *Store) AddUserSpend(id string, delta float64) error {
	n, err := s.Engine.ID(id).Incr("spend", delta).Update(&userRow{})
	s.bust(new(userRow))
	return rowsAffected(n, err)
}

// 把团队表行转成实体。组织 id 放在 TeamID 上。
func teamFrom(r teamRow) Entity {
	return Entity{
		ID: r.TeamID, Alias: r.TeamAlias, TeamID: r.OrganizationID, ModelsJSON: nz(r.ModelsJSON, "[]"),
		MaxBudget: nullF(r.MaxBudget), Spend: r.Spend, ExtraJSON: r.ExtraJSON, CreatedAt: parseRFC(r.CreatedAt),
	}
}

// 把团队实体转成表行。TeamID 写入组织 id。
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

// 插入团队，并清掉团队缓存。
func (s *Store) InsertTeam(e Entity) error {
	_, err := s.Engine.Insert(teamTo(e))
	s.bust(new(teamRow))
	return err
}

// 列出全部团队。
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

// 按 id 读取团队。没有这一行时返回无行错误。
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

// 更新团队。找不到返回无行。
func (s *Store) UpdateTeam(e Entity) error {
	row := teamTo(e)
	n, err := s.Engine.ID(e.ID).Cols("team_alias", "organization_id", "models_json", "max_budget", "extra_json").Update(&row)
	s.bust(new(teamRow))
	return rowsAffected(n, err)
}

// 按 id 删除团队，并清掉缓存。
func (s *Store) DeleteTeam(id string) error {
	n, err := s.Engine.ID(id).Delete(&teamRow{})
	s.bust(new(teamRow))
	return rowsAffected(n, err)
}

// 给团队加上一笔花费。团队不存在时返回无行。
func (s *Store) AddTeamSpend(id string, delta float64) error {
	n, err := s.Engine.ID(id).Incr("spend", delta).Update(&teamRow{})
	s.bust(new(teamRow))
	return rowsAffected(n, err)
}

// 把组织实体收成表行。模型和花费放进 extra_json。
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

// 把组织表行转成实体。花费从 extra_json 读出。
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

// 插入组织，并清掉组织缓存。
func (s *Store) InsertOrg(e Entity) error {
	_, err := s.Engine.Insert(packOrg(e))
	s.bust(new(orgRow))
	return err
}

// 按 id 读取组织。没有这一行时返回无行错误。
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

// 列出全部组织。
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

// 更新组织的名字和额外字段。花费以库里的当前值为准。
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
	// 调用方手里的花费可能是刷盘前读到的。别名更新不能把已入账的花费写回去。
	e.Spend = cur.Spend
	return s.writeOrg(e, cur.CreatedAt)
}

// 给组织加上一笔花费。和刷盘共用一把锁，避免丢掉增量。
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

// 把组织的名字和 extra_json 写回。调用方要已经拿着花费锁。
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

// 删除组织及其子表里的行。团队不删。
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

// 项目所属组织。优先用团队上的组织，其次用额外字段。
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

// 把项目实体收成表行。团队和花费放进 extra_json。
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

// 把项目表行转成实体，并把组织 id 放回额外字段。
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

// 插入项目，并清掉项目缓存。
func (s *Store) InsertProject(e Entity) error {
	_, err := s.Engine.Insert(packProject(e, s.projectOrgID(e)))
	s.bust(new(projectRow))
	return err
}

// 按 id 读取项目。没有这一行时返回无行错误。
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

// 列出全部项目。
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

// 更新项目。找不到返回无行。
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

// 按 id 删除项目，并清掉缓存。
func (s *Store) DeleteProject(id string) error {
	n, err := s.Engine.ID(id).Delete(&projectRow{})
	s.bust(new(projectRow))
	return rowsAffected(n, err)
}

// 把预算转成控制面表行。金额写到 limit_amount。
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

// 把预算表行转回网关用的预算。
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

// 插入预算，并清掉预算缓存。
func (s *Store) InsertBudget(b Budget) error {
	_, err := s.Engine.Insert(budgetTo(b))
	s.bust(new(budgetRow))
	return err
}

// 列出全部预算，按对外字段名返回。
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

// 按 id 读取预算。没有这一行时返回无行错误。
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

// 更新预算额度、限流和重置时间。找不到返回无行。
func (s *Store) UpdateBudget(b Budget) error {
	row := budgetTo(b)
	n, err := s.Engine.ID(b.ID).Cols(
		"limit_amount", "soft_budget", "tpm_limit", "rpm_limit", "max_parallel_requests",
		"budget_duration", "period", "budget_reset_at", "model_max_budget", "updated_at",
	).Update(&row)
	s.bust(new(budgetRow))
	return rowsAffected(n, err)
}

// 按 id 删除预算，并清掉缓存。
func (s *Store) DeleteBudget(id string) error {
	n, err := s.Engine.ID(id).Delete(&budgetRow{})
	s.bust(new(budgetRow))
	return rowsAffected(n, err)
}

// 写入一条键值。已有则更新，没有则插入。
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

// 组合主键。顺序跟表结构里的主键列一致。
func coreIDs(kind, id string) schemas.PK {
	return schemas.PK{kind, id}
}

// 按种类和 id 读取一条键值。
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

// 删除一条键值，并清掉缓存。找不到返回无行。
func (s *Store) DeleteKV(kind, id string) error {
	n, err := s.Engine.ID(coreIDs(kind, id)).Delete(&kvRow{})
	s.bust(new(kvRow))
	return rowsAffected(n, err)
}

// 列出某个种类下的全部键值。
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
