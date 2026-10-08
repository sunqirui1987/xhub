package usage

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"sync"

	"github.com/sunqirui1987/xhub/internal/authz"
	"github.com/sunqirui1987/xhub/internal/httpx"
	"github.com/sunqirui1987/xhub/internal/iam"
	"github.com/sunqirui1987/xhub/internal/logx"
)

// entityDim is the breakdown.entities key the usage page groups by.
// entityNone leaves that map empty.
type entityDim int

const (
	entityNone entityDim = iota
	entityTeam
	entityOrg
	entityUser
)

// entityBucket is one team, organization, or user inside a day.
type entityBucket struct {
	metric
	teamAlias string
	keyAlias  string
	keys      map[string]*keyMetric
}

// entityLabels are display names for ids that already appear in the scoped
// rows. They are not a second query for "who exists".
type entityLabels struct {
	users map[string]iam.User
	teams map[string]iam.Team
	orgs  map[string]iam.Organization
}

var logTraceOnceEntity sync.Once

// TeamDailyActivity is GET /team/daily/activity. The breakdown is per team, paged by day, and limited to the teams the caller may see.
// 参数 s（Host）：团队按天活动使用的数据面宿主；w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；r（*http.Request）：入站 HTTP 请求。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：gateway/usage/mount.go
// 测试：无直接单测
func TeamDailyActivity(s Host, w http.ResponseWriter, r *http.Request) {
	logTraceOnceEntity.Do(func() { logx.Trace("enter usage.TeamDailyActivity") })
	writeDailyActivity(s, w, r, entityTeam, false)
}

// TeamDailyActivityAggregated is GET /team/daily/activity/aggregated. The usage page loads this first for "团队用量". One response covers the whole range.
// 参数 s（Host）：团队按天活动聚合使用的数据面宿主；w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；r（*http.Request）：入站 HTTP 请求。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：gateway/usage/mount.go
// 测试：无直接单测
func TeamDailyActivityAggregated(s Host, w http.ResponseWriter, r *http.Request) {
	writeDailyActivity(s, w, r, entityTeam, true)
}

// OrganizationDailyActivity is GET /organization/daily/activity. The breakdown is per organization, for "组织用量".
// 参数 s（Host）：组织按天活动使用的数据面宿主；w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；r（*http.Request）：入站 HTTP 请求。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：gateway/usage/mount.go
// 测试：无直接单测
func OrganizationDailyActivity(s Host, w http.ResponseWriter, r *http.Request) {
	writeDailyActivity(s, w, r, entityOrg, false)
}

// writeDailyActivity answers a daily-activity route. The scope is applied first. team_ids, organization_ids and exclude_team_ids only narrow it.
// 参数 s（Host）：写入按天活动使用的数据面宿主；w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；r（*http.Request）：入站 HTTP 请求；dim（entityDim）：聚合维度，例如团队、组织、用户或密钥；aggregated（bool）：为真时返回汇总值，而不是按天拆开。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：gateway/usage/activity.go
// 测试：无直接单测
func writeDailyActivity(s Host, w http.ResponseWriter, r *http.Request, dim entityDim, aggregated bool) {
	rows, labels, ok := loadScopedActivity(s, w, r, dim)
	if !ok {
		return
	}
	page := 1
	if !aggregated {
		page = pageFromQuery(r)
	}
	httpx.WriteJSON(w, 200, activityBody(rows, page, aggregated, dim, labels))
}

// TeamSpendByUser is GET /team/spend/by_user. Each row is one user inside one team, still inside the caller's usage scope.
// 参数 s（Host）：团队花费按用户使用的数据面宿主；w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；r（*http.Request）：入站 HTTP 请求。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：gateway/usage/mount.go
// 测试：无直接单测
func TeamSpendByUser(s Host, w http.ResponseWriter, r *http.Request) {
	rows, labels, ok := loadScopedActivity(s, w, r, entityTeam)
	if !ok {
		return
	}
	// The team labels name the team column. User labels are a second read of
	// the same rows, so an address is shown only for someone already in scope.
	users, err := loadLabels(s.Identity(), r.Context(), rows, entityUser)
	if err != nil {
		s.WriteIAMError(w, r, err)
		return
	}
	labels.users = users.users
	httpx.WriteJSON(w, 200, teamSpendByUserBody(r, rows, labels))
}

// loadScopedActivity reads the events the caller may see. A team_id keeps the single-team check UsageScope already performs. A team_ids or organization_ids list is intersected with what the caller can see; asking only for things they cannot see is a 404, not an unfiltered read.
// 参数 s（Host）：载入Scoped活动使用的数据面宿主；w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；r（*http.Request）：入站 HTTP 请求；dim（entityDim）：聚合维度，例如团队、组织、用户或密钥。
// 返回 []activityRow（[]activityRow）：调用方看得见的用量行。没有匹配时为空切片；entityLabels（entityLabels）：这些行对应的团队、组织或用户展示名。查不到时是零值；bool（bool）：调用方可以看这些用量、查询可以继续时返回真。只问看不见的团队时是 404，不是不过滤的全量读取。
// 调用：仅在 entity_activity.go 内使用
// 测试：无直接单测
func loadScopedActivity(s Host, w http.ResponseWriter, r *http.Request, dim entityDim) ([]activityRow, entityLabels, bool) {
	p := s.RequireUser(w, r)
	if p == nil {
		return nil, entityLabels{}, false
	}
	raw := r.URL.Query()
	list := csvIDs(raw.Get("team_ids"))
	scopeTeam := ""
	if len(list) == 0 {
		scopeTeam = strings.TrimSpace(raw.Get("team_id"))
	}
	sc, err := s.UsageScope(r, p, scopeTeam)
	if err != nil {
		s.WriteAuthz(w, r, err)
		return nil, entityLabels{}, false
	}
	orgs := csvIDs(raw.Get("organization_ids"))
	list, orgs, err = narrowRequested(s, r, p.UserID, sc, list, orgs)
	if err != nil {
		if authz.IsNotFound(err) {
			s.WriteAuthz(w, r, err)
		} else {
			s.WriteIAMError(w, r, err)
		}
		return nil, entityLabels{}, false
	}
	q := activityQuery(r, sc)
	q.TeamIDs = list
	q.ExcludeTeamIDs = csvIDs(raw.Get("exclude_team_ids"))
	q.OrganizationIDs = orgs
	rows, err := loadActivityQuery(s, r, q)
	if err != nil {
		s.WriteIAMError(w, r, err)
		return nil, entityLabels{}, false
	}
	labels, err := loadLabels(s.Identity(), r.Context(), rows, dim)
	if err != nil {
		s.WriteIAMError(w, r, err)
		return nil, entityLabels{}, false
	}
	return rows, labels, true
}

// narrowRequested drops ids the caller cannot see. A platform administrator is not narrowed. An empty request stays empty, which means "no extra filter". A request whose every id is invisible is not found: falling through would drop the filter and return the caller's whole scope.
// 参数 s（Host）：narrowRequested使用的数据面宿主；r（*http.Request）：入站 HTTP 请求；userID（string）：用户 id。空串表示没有指定用户；sc（*authz.Scope）：narrowRequested使用的权限范围；teams（[]string）：teams列表。空切片表示没有可处理的项；orgs（[]string）：orgs列表。空切片表示没有可处理的项。
// 返回 []string（[]string）：narrowRequested。没有匹配时为 nil 或空切片，调用方按长度判断；[]string（[]string）：narrowRequested。没有匹配时为 nil 或空切片，调用方按长度判断；error（error）：失败原因。nil 表示这一步成功。
// 调用：仅在 entity_activity.go 内使用
// 测试：无直接单测
func narrowRequested(s Host, r *http.Request, userID string, sc *authz.Scope, teams, orgs []string) ([]string, []string, error) {
	if sc == nil || sc.All || (len(teams) == 0 && len(orgs) == 0) {
		return teams, orgs, nil
	}
	db := s.Identity()
	if db == nil {
		return teams, orgs, nil
	}
	if len(teams) > 0 {
		visible, err := db.ListVisibleTeams(r.Context(), userID, "")
		if err != nil {
			return nil, nil, err
		}
		seen := map[string]struct{}{}
		for _, team := range visible {
			seen[team.ID] = struct{}{}
		}
		teams, err = keepVisible(teams, seen)
		if err != nil {
			return nil, nil, err
		}
	}
	if len(orgs) > 0 {
		rows, err := db.ListOrgs(r.Context(), userID)
		if err != nil {
			return nil, nil, err
		}
		seen := map[string]struct{}{}
		for _, org := range rows {
			seen[org.ID] = struct{}{}
		}
		orgs, err = keepVisible(orgs, seen)
		if err != nil {
			return nil, nil, err
		}
	}
	return teams, orgs, nil
}

// 只保留对方有权看见的 id，其余丢掉。
// 参数 ids（[]string）：要保留或查询的一组 id。空切片表示没有可处理的记录；seen（map[string]值）：保留可见使用的map[string]值。
// 返回 []string（[]string）：保留可见。没有匹配时为 nil 或空切片，调用方按长度判断；error（error）：失败原因。nil 表示这一步成功。
// 调用：仅在 entity_activity.go 内使用
// 测试：activity_test.go
func keepVisible(ids []string, seen map[string]struct{}) ([]string, error) {
	kept := make([]string, 0, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			kept = append(kept, id)
		}
	}
	if len(kept) == 0 {
		return nil, authz.ErrNotFound
	}
	return kept, nil
}

// 把逗号分隔的 id 收成去重后的列表。空白输入返回 nil。
// 参数 raw（string）：逗号分隔标识列表使用的原始内容。空串表示调用方没有提供这项。
// 返回 []string（[]string）：逗号分隔标识列表。没有匹配时为空切片。
// 调用：仅在 entity_activity.go 内使用
// 测试：无直接单测
func csvIDs(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if _, ok := seen[part]; ok {
			continue
		}
		seen[part] = struct{}{}
		out = append(out, part)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// 按聚合维度从用量行取出团队、组织、用户或密钥 id。
// 参数 row（activityRow）：从用量或目录读出的用量行；dim（entityDim）：聚合维度，例如团队、组织、用户或密钥。
// 返回 string（string）：按聚合维度从用量行取出团队、组织、用户或密钥 id。没有可用值时返回空串。
// 调用：仅在 entity_activity.go 内使用
// 测试：无直接单测
func entityKey(row activityRow, dim entityDim) string {
	switch dim {
	case entityTeam:
		return row.teamID
	case entityOrg:
		return row.organizationID
	case entityUser:
		if row.userID != "" {
			return row.userID
		}
		if row.apiKey != "" {
			return "key:" + row.apiKey
		}
		return ""
	default:
		return ""
	}
}

// 把这一行累加进对应维度的实体桶。id 为空时跳过。
// 参数 day（*dayMetric）：累加实体使用的dayMetric；dim（entityDim）：聚合维度，例如团队、组织、用户或密钥；row（activityRow）：从用量或目录读出的用量行。
// 返回：无。这一行已累加进对应维度的实体桶。id 为空时跳过。
// 调用：gateway/usage/activity.go
// 测试：无直接单测
func addEntity(day *dayMetric, dim entityDim, row activityRow) {
	id := entityKey(row, dim)
	if id == "" {
		return
	}
	if day.entities == nil {
		day.entities = map[string]*entityBucket{}
	}
	bucket := day.entities[id]
	if bucket == nil {
		bucket = &entityBucket{keys: map[string]*keyMetric{}}
		day.entities[id] = bucket
	}
	bucket.add(row.prompt, row.completion, row.cacheRead, row.spend, row.success)
	if bucket.teamAlias == "" {
		bucket.teamAlias = row.teamAlias
	}
	if bucket.keyAlias == "" {
		bucket.keyAlias = row.keyAlias
	}
	if row.apiKey != "" {
		addKey(bucket.keys, row.apiKey, row)
	}
}

// 把实体桶收成报表。维度为空时返回空对象。
// 参数 in（map[string]*entityBucket）：调用方提交的map[string]*entityBucket。字段为空表示这项不改；dim（entityDim）：聚合维度，例如团队、组织、用户或密钥；labels（entityLabels）：实体JSON使用的id 到展示名。
// 返回 map[string]any（map[string]any）：给响应或报表用的 JSON 对象。键是前端已经约定的字段，缺键表示这项没有数据。
// 调用：gateway/usage/activity.go
// 测试：无直接单测
func entitiesJSON(in map[string]*entityBucket, dim entityDim, labels entityLabels) map[string]any {
	out := map[string]any{}
	if dim == entityNone {
		return out
	}
	for id, bucket := range in {
		out[id] = map[string]any{
			"metrics":           bucket.json(),
			"metadata":          entityMeta(id, bucket, dim, labels),
			"api_key_breakdown": keysJSON(bucket.keys),
		}
	}
	return out
}

// 收成实体的展示信息。能找到名称时用名称，否则用已有别名。
// 参数 id（string）：实体元数据使用的主键。空串表示调用方没有指定记录；bucket（*entityBucket）：实体元数据使用的entityBucket；dim（entityDim）：聚合维度，例如团队、组织、用户或密钥；labels（entityLabels）：实体元数据使用的id 到展示名。
// 返回 map[string]any（map[string]any）：实体元数据的字段表。缺键表示上游或库里没有这个字段。
// 调用：仅在 entity_activity.go 内使用
// 测试：无直接单测
func entityMeta(id string, bucket *entityBucket, dim entityDim, labels entityLabels) map[string]any {
	meta := map[string]any{"id": id}
	switch dim {
	case entityTeam:
		alias := bucket.teamAlias
		if team, ok := labels.teams[id]; ok && team.Name != "" {
			alias = team.Name
		}
		meta["team_alias"] = nilIfEmpty(alias)
	case entityOrg:
		name := ""
		if org, ok := labels.orgs[id]; ok {
			name = org.Name
		}
		meta["organization_alias"] = nilIfEmpty(name)
	case entityUser:
		if user, ok := labels.users[id]; ok {
			meta["user_email"] = nilIfEmpty(user.Email)
			meta["user_alias"] = nilIfEmpty(user.Name)
			break
		}
		meta["user_alias"] = nilIfEmpty(bucket.keyAlias)
	}
	return meta
}

// 按维度批量加载团队、组织或用户的展示名。
// 参数 db（*iam.DB）：身份和用量库；ctx（context.Context）：上下文，取消时停止；rows（[]activityRow）：从用量或目录读出的用量行；dim（entityDim）：聚合维度，例如团队、组织、用户或密钥。
// 返回 entityLabels（entityLabels）：按维度批量加载团队、组织或用户的展示名；error（error）：失败原因。nil 表示这一步成功。
// 调用：仅在 entity_activity.go 内使用
// 测试：无直接单测
func loadLabels(db *iam.DB, ctx context.Context, rows []activityRow, dim entityDim) (entityLabels, error) {
	var labels entityLabels
	if db == nil || dim == entityNone {
		return labels, nil
	}
	ids := idsFor(rows, dim)
	var err error
	switch dim {
	case entityTeam:
		labels.teams, err = db.TeamsByIDs(ctx, ids)
	case entityOrg:
		labels.orgs, err = db.OrgsByIDs(ctx, ids)
	case entityUser:
		labels.users, err = db.UsersByIDs(ctx, ids)
	}
	return labels, err
}

type userSpendRow struct {
	teamID    string
	userID    string
	teamAlias string
	email     string
	alias     string
	metric
}

// 把用量按团队和用户收成花费报表。
// 参数 r（*http.Request）：入站 HTTP 请求；rows（[]activityRow）：从用量或目录读出的用量行；labels（entityLabels）：团队花费按用户正文使用的id 到展示名。
// 返回 map[string]any（map[string]any）：给响应或报表用的 JSON 对象。键是前端已经约定的字段，缺键表示这项没有数据。
// 调用：仅在 entity_activity.go 内使用
// 测试：无直接单测
func teamSpendByUserBody(r *http.Request, rows []activityRow, labels entityLabels) map[string]any {
	grouped := map[string]*userSpendRow{}
	order := make([]string, 0)
	for _, row := range rows {
		key := row.teamID + "\x00" + row.userID
		bucket := grouped[key]
		if bucket == nil {
			bucket = &userSpendRow{teamID: row.teamID, userID: row.userID, teamAlias: row.teamAlias}
			if team, ok := labels.teams[row.teamID]; ok && team.Name != "" {
				bucket.teamAlias = team.Name
			}
			if user, ok := labels.users[row.userID]; ok {
				bucket.email = user.Email
				bucket.alias = user.Name
			} else if row.userID == "" {
				bucket.alias = row.keyAlias
			}
			grouped[key] = bucket
			order = append(order, key)
		}
		bucket.add(row.prompt, row.completion, row.cacheRead, row.spend, row.success)
	}
	sort.Slice(order, func(i, j int) bool {
		a, b := grouped[order[i]], grouped[order[j]]
		if a.spend != b.spend {
			return a.spend > b.spend
		}
		if a.teamID != b.teamID {
			return a.teamID < b.teamID
		}
		return a.userID < b.userID
	})
	results := make([]any, 0, len(order))
	for _, key := range order {
		row := grouped[key]
		results = append(results, map[string]any{
			"team_id":             row.teamID,
			"team_alias":          nilIfEmpty(row.teamAlias),
			"user_id":             row.userID,
			"user_email":          nilIfEmpty(row.email),
			"user_alias":          nilIfEmpty(row.alias),
			"spend":               row.spend,
			"prompt_tokens":       row.prompt,
			"completion_tokens":   row.completion,
			"total_tokens":        row.prompt + row.completion,
			"api_requests":        row.requests,
			"successful_requests": row.success,
			"failed_requests":     row.failed,
		})
	}
	return map[string]any{
		"start_date": r.URL.Query().Get("start_date"),
		"end_date":   r.URL.Query().Get("end_date"),
		"results":    results,
	}
}

// 收集这个维度上出现过的 id，跳过空值和密钥占位。
// 参数 rows（[]activityRow）：从用量或目录读出的用量行；dim（entityDim）：聚合维度，例如团队、组织、用户或密钥。
// 返回 []string（[]string）：标识列表为。没有匹配时为空切片。
// 调用：仅在 entity_activity.go 内使用
// 测试：无直接单测
func idsFor(rows []activityRow, dim entityDim) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0)
	for _, row := range rows {
		id := entityKey(row, dim)
		if id == "" || strings.HasPrefix(id, "key:") {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}
