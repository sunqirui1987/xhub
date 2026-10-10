package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sunqirui1987/xhub/internal/iam"
)

// TestDailyActivityHTTPReadsRecordedUsage 验证真实用量写入后控制台聚合接口返回一致的请求数与费用。
// 参数 t 为测试上下文；前置隔离配置库和身份库、关闭共享 Redis，使用真实登录和 HTTP 查询。
// 返回：无；用户、密钥、用量和配置随私有 schema 清理，服务器与连接池在结束时关闭。
func TestDailyActivityHTTPReadsRecordedUsage(t *testing.T) {
	cfg, st, db := testGatewayStores(t)
	ctx := context.Background()

	admin, err := db.CreateUser(ctx, testActor, iam.UserInput{
		Email: "activity-admin@example.com", Name: "Activity Admin", Password: "password123", Role: iam.RoleAdmin,
	})
	if err != nil {
		t.Fatalf("create admin: %v", err)
	}
	key, plain, err := db.CreateKey(ctx, iam.Actor{ID: admin.ID, Kind: "session"}, iam.KeyInput{
		Name: "activity-probe", TeamID: testTeam(t, db, admin.ID), OwnerType: iam.OwnerService,
	})
	if err != nil {
		t.Fatalf("create key: %v", err)
	}
	_ = key

	id := "activity-probe-" + time.Now().UTC().Format("20060102150405.000000000")
	start := time.Date(2026, 9, 27, 8, 0, 0, 0, time.UTC)
	if err := db.RecordUsage(ctx, []iam.UsageRecord{{
		RequestID: id, TS: start, KeyID: key.ID, OwnerType: iam.OwnerService,
		UserID: admin.ID, TeamID: key.TeamID, Model: "activity-probe-model",
		CallType: "chat", Status: "success",
		PromptTokens: 11, CompletionTokens: 7, Cost: 3.5, DurationMS: 1000,
	}}); err != nil {
		t.Fatalf("record usage: %v", err)
	}

	gw := New(cfg, st, db)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)

	// The master key reaches the health and bootstrap routes only, so the
	// session is created through the login path the console uses.
	sess := loginAs(t, srv.URL, admin.Email, "password123")

	status, body := authed(t, srv.URL, sess, http.MethodGet,
		"/user/daily/activity/aggregated?start_date=2026-09-27&end_date=2026-09-27", nil)
	if status != http.StatusOK {
		t.Fatalf("status %d %s", status, trim(body))
	}
	var parsed struct {
		Results []struct {
			Date      string `json:"date"`
			Breakdown struct {
				Models map[string]struct {
					Metrics struct {
						Spend       float64 `json:"spend"`
						APIRequests int     `json:"api_requests"`
					} `json:"metrics"`
				} `json:"models"`
				ModelGroups map[string]json.RawMessage `json:"model_groups"`
			} `json:"breakdown"`
		} `json:"results"`
		Metadata struct {
			TotalSpend      float64 `json:"total_spend"`
			TotalAPI        int     `json:"total_api_requests"`
			TotalSuccessful int     `json:"total_successful_requests"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatal(err)
	}
	var probeSpend float64
	var probeRequests int
	var foundProbe bool
	for _, day := range parsed.Results {
		model, ok := day.Breakdown.Models["activity-probe-model"]
		if !ok {
			continue
		}
		foundProbe = true
		probeSpend = model.Metrics.Spend
		probeRequests = model.Metrics.APIRequests
		if _, grouped := day.Breakdown.ModelGroups["activity-probe-model"]; !grouped {
			t.Fatal("probe model missing from model_groups")
		}
	}
	if !foundProbe {
		t.Fatalf("probe model missing from %s", trim(body))
	}
	if probeRequests < 1 || probeSpend < 3.5 {
		t.Fatalf("probe metrics spend=%v requests=%d", probeSpend, probeRequests)
	}
	if parsed.Metadata.TotalAPI < 1 || parsed.Metadata.TotalSpend < 3.5 || parsed.Metadata.TotalSuccessful < 1 {
		t.Fatalf("metadata: %+v", parsed.Metadata)
	}

	gwStatus, gwBody := authed(t, srv.URL, sess, http.MethodGet,
		"/gateway/daily/activity?start_date=2026-09-27&end_date=2026-09-27", nil)
	if gwStatus != http.StatusOK {
		t.Fatalf("gateway status %d %s", gwStatus, trim(gwBody))
	}
	var counts struct {
		TotalSuccessful int `json:"total_successful_requests"`
		ByRoute         []struct {
			Route      string `json:"route"`
			Successful int    `json:"successful_requests"`
		} `json:"by_route"`
	}
	if err := json.Unmarshal(gwBody, &counts); err != nil {
		t.Fatal(err)
	}
	if counts.TotalSuccessful < 1 {
		t.Fatalf("gateway counts: %s", trim(gwBody))
	}
	foundRoute := false
	for _, route := range counts.ByRoute {
		if route.Route == "/chat/completions" && route.Successful >= 1 {
			foundRoute = true
		}
	}
	if !foundRoute {
		t.Fatalf("chat route missing: %s", trim(gwBody))
	}

	_ = plain
}

// TestUsageViewsFollowTheCallerScope checks the four usage views the console
// offers. A person sees their own calls. A team administrator also sees the
// team they run. An organization administrator sees that organization. None of
// them see the other organization's calls, and asking only for that other
// organization is not found rather than answered with their own rows.
func TestUsageViewsFollowTheCallerScope(t *testing.T) {
	c := newChain(t)
	when := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	record := func(id string, user *iam.User, team *iam.Team, org *iam.Organization, cost float64) {
		t.Helper()
		err := c.f.db.RecordUsage(c.ctx, []iam.UsageRecord{{
			RequestID: id, TS: when, OwnerType: iam.OwnerPersonal,
			UserID: user.ID, TeamID: team.ID, OrganizationID: org.ID,
			TeamAlias: team.Name, Model: "usage-view-model", CallType: "chat",
			Status: "success", PromptTokens: 10, CompletionTokens: 4, Cost: cost,
		}})
		if err != nil {
			t.Fatalf("record %s: %v", id, err)
		}
	}
	record("view-a1-member", c.memberA1.user, c.teamA1, c.orgA, 2)
	record("view-a1-admin", c.teamAdminA1.user, c.teamA1, c.orgA, 3)
	record("view-b1-member", c.memberB1.user, c.teamB1, c.orgB, 8)

	window := "start_date=2026-09-27&end_date=2026-09-27"
	get := func(a actor, path string) (int, float64, map[string]activityEntity) {
		t.Helper()
		status, body := c.f.call(a, http.MethodGet, path, nil)
		if status != http.StatusOK {
			t.Fatalf("%s %s: %d %s", a.label(), path, status, trim(body))
		}
		total, entities := activityEntities(t, body)
		return status, total, entities
	}

	_, total, entities := get(c.teamAdminA1, "/team/daily/activity/aggregated?"+window)
	if total != 5 || entities[c.teamA1.ID].spend != 5 || entities[c.teamB1.ID].spend != 0 {
		t.Fatalf("team admin team view: total %v entities %#v", total, entities)
	}
	if entities[c.teamA1.ID].alias != c.teamA1.Name {
		t.Fatalf("team alias: %#v", entities[c.teamA1.ID])
	}

	_, total, entities = get(c.teamAdminA1, "/team/daily/activity/aggregated?"+window+"&team_ids="+c.teamA1.ID+","+c.teamB1.ID)
	if total != 5 || entities[c.teamB1.ID].spend != 0 {
		t.Fatalf("mixed team filter widened: total %v %#v", total, entities)
	}

	status, body := c.f.call(c.teamAdminA1, http.MethodGet, "/team/daily/activity/aggregated?"+window+"&team_ids="+c.teamB1.ID, nil)
	if status != http.StatusNotFound {
		t.Fatalf("team admin read another team's usage: %d %s", status, trim(body))
	}

	_, total, entities = get(c.memberA1, "/team/daily/activity/aggregated?"+window)
	if total != 2 || entities[c.teamA1.ID].spend != 2 {
		t.Fatalf("member team view: total %v %#v", total, entities)
	}

	_, total, entities = get(c.teamAdminA1, "/user/daily/activity/aggregated?"+window+"&user_id="+c.teamAdminA1.user.ID)
	if total != 3 || len(entities) != 1 || entities[c.teamAdminA1.user.ID].spend != 3 {
		t.Fatalf("own usage: total %v %#v", total, entities)
	}

	_, total, entities = get(c.teamAdminA1, "/user/daily/activity/aggregated?"+window)
	if total != 5 || entities[c.memberA1.user.ID].spend != 2 || entities[c.teamAdminA1.user.ID].spend != 3 || entities[c.memberB1.user.ID].spend != 0 {
		t.Fatalf("team user view: total %v %#v", total, entities)
	}
	if entities[c.memberA1.user.ID].email != c.memberA1.user.Email {
		t.Fatalf("member email: %#v", entities[c.memberA1.user.ID])
	}

	_, total, _ = get(c.memberA1, "/user/daily/activity/aggregated?"+window+"&user_id="+c.teamAdminA1.user.ID)
	if total != 0 {
		t.Fatalf("member read another user's usage: %v", total)
	}

	_, total, entities = get(c.orgAdminA, "/organization/daily/activity?"+window)
	if total != 5 || entities[c.orgA.ID].spend != 5 || entities[c.orgB.ID].spend != 0 {
		t.Fatalf("org view: total %v %#v", total, entities)
	}
	if entities[c.orgA.ID].alias != c.orgA.Name {
		t.Fatalf("org alias: %#v", entities[c.orgA.ID])
	}

	status, body = c.f.call(c.orgAdminA, http.MethodGet, "/organization/daily/activity?"+window+"&organization_ids="+c.orgB.ID, nil)
	if status != http.StatusNotFound {
		t.Fatalf("org admin read another organization: %d %s", status, trim(body))
	}

	_, total, entities = get(c.f.admin, "/team/daily/activity/aggregated?"+window)
	if total != 13 || entities[c.teamA1.ID].spend != 5 || entities[c.teamB1.ID].spend != 8 {
		t.Fatalf("platform team view: total %v %#v", total, entities)
	}

	status, body = c.f.call(c.teamAdminA1, http.MethodGet, "/team/spend/by_user?"+window+"&team_ids="+c.teamA1.ID, nil)
	if status != http.StatusOK {
		t.Fatalf("spend by user: %d %s", status, trim(body))
	}
	var byUser struct {
		Results []struct {
			UserID string  `json:"user_id"`
			Email  string  `json:"user_email"`
			Spend  float64 `json:"spend"`
			TeamID string  `json:"team_id"`
		} `json:"results"`
	}
	if err := json.Unmarshal(body, &byUser); err != nil {
		t.Fatalf("spend by user json: %v %s", err, trim(body))
	}
	if len(byUser.Results) != 2 {
		t.Fatalf("spend by user rows: %s", trim(body))
	}
	seen := map[string]float64{}
	for _, row := range byUser.Results {
		if row.TeamID != c.teamA1.ID {
			t.Fatalf("spend by user team: %+v", row)
		}
		seen[row.UserID] = row.Spend
		if row.UserID == c.memberA1.user.ID && row.Email != c.memberA1.user.Email {
			t.Fatalf("spend by user email: %+v", row)
		}
	}
	if seen[c.memberA1.user.ID] != 2 || seen[c.teamAdminA1.user.ID] != 3 {
		t.Fatalf("spend by user: %#v", seen)
	}
}

type activityEntity struct {
	spend float64
	alias string
	email string
}

func activityEntities(t *testing.T, body []byte) (float64, map[string]activityEntity) {
	t.Helper()
	var parsed struct {
		Results []struct {
			Breakdown struct {
				Entities map[string]struct {
					Metrics struct {
						Spend float64 `json:"spend"`
					} `json:"metrics"`
					Metadata struct {
						TeamAlias string `json:"team_alias"`
						OrgAlias  string `json:"organization_alias"`
						Email     string `json:"user_email"`
					} `json:"metadata"`
				} `json:"entities"`
			} `json:"breakdown"`
		} `json:"results"`
		Metadata struct {
			TotalSpend float64 `json:"total_spend"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("activity json: %v %s", err, trim(body))
	}
	out := map[string]activityEntity{}
	for _, day := range parsed.Results {
		for id, entity := range day.Breakdown.Entities {
			cur := out[id]
			cur.spend += entity.Metrics.Spend
			if entity.Metadata.TeamAlias != "" {
				cur.alias = entity.Metadata.TeamAlias
			}
			if entity.Metadata.OrgAlias != "" {
				cur.alias = entity.Metadata.OrgAlias
			}
			if entity.Metadata.Email != "" {
				cur.email = entity.Metadata.Email
			}
			out[id] = cur
		}
	}
	return parsed.Metadata.TotalSpend, out
}

// testActor is the acting identity for fixture writes the test itself performs.
// api_keys.created_by is a foreign key to users, so it must name a real account.
var testActor = iam.Actor{Kind: "system"}

// testTeam creates an organization and a team administered by the fixture, so a
// service key has somewhere to belong. CreateTeam requires a first team_admin,
// which is the account the test later signs in as.
func testTeam(t *testing.T, db *iam.DB, adminID string) string {
	t.Helper()
	ctx := context.Background()
	actor := iam.Actor{ID: adminID, Kind: "session"}
	org, err := db.CreateOrg(ctx, actor, "Activity Org", nil)
	if err != nil {
		t.Fatalf("create org: %v", err)
	}
	team, err := db.CreateTeam(ctx, actor, iam.TeamInput{
		Name: "Activity Team", OrganizationID: org.ID, AdminUserID: adminID,
	})
	if err != nil {
		t.Fatalf("create team: %v", err)
	}
	return team.ID
}

// loginAs signs in through the console's login route and returns the session
// credential the browser would keep.
func loginAs(t *testing.T, base, email, password string) string {
	t.Helper()
	payload, _ := json.Marshal(map[string]string{"username": email, "password": password})
	status, body := authed(t, base, "", http.MethodPost, "/v2/login", payload)
	if status != http.StatusOK {
		t.Fatalf("login %d %s", status, trim(body))
	}
	var parsed struct {
		Key string `json:"key"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil || parsed.Key == "" {
		t.Fatalf("login key: %s", trim(body))
	}
	return parsed.Key
}
