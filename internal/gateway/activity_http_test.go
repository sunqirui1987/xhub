package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/iam"
)

// TestDailyActivityHTTPReadsRecordedUsage records one call through the usage
// write path and then reads the usage page's own endpoints, so the roll-up the
// console renders is checked against the rows the gateway actually stores.
func TestDailyActivityHTTPReadsRecordedUsage(t *testing.T) {
	cfg, err := config.Load(configPath(t))
	if err != nil {
		t.Fatal(err)
	}
	db := testIdentityStore(t)
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

	gw := New(cfg, nil, db)
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
