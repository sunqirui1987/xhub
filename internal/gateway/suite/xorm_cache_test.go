package suite

import (
	"database/sql"
	"sync"
	"testing"
	"time"

	"github.com/sunqirui1987/xhub/internal/store"
)

func TestXormCachedSecondGet(t *testing.T) {
	st, err := store.Open(testDatabaseURL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.DB.Close() })

	created := time.Now().UTC().Truncate(time.Second)
	if err := st.InsertOrg(store.Entity{
		ID: "org_cache", Alias: "缓存组织", ModelsJSON: "[]", CreatedAt: created,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetOrg("org_cache"); err != nil {
		t.Fatal(err)
	}
	st.ResetStatements()
	got, err := st.GetOrg("org_cache")
	if err != nil {
		t.Fatal(err)
	}
	if stmts := st.Statements(); stmts != 0 {
		t.Fatalf("second get used %d statements", stmts)
	}
	if got.Alias != "缓存组织" {
		t.Fatalf("cached alias %q", got.Alias)
	}

	got.Alias = "已更新"
	if err := st.UpdateOrg(*got); err != nil {
		t.Fatal(err)
	}
	again, err := st.GetOrg("org_cache")
	if err != nil {
		t.Fatal(err)
	}
	if again.Alias != "已更新" {
		t.Fatalf("write not visible: %+v", again)
	}
}

func TestXormOrgProjectCreateRead(t *testing.T) {
	s, master := testEnv(t)
	h := s.Handler()

	org := doJSON(t, h, "POST", "/organization/new", master, map[string]any{
		"organization_alias": "七牛", "models": []any{},
	})
	if org.Code != 200 {
		t.Fatalf("org %d %s", org.Code, org.Body.String())
	}
	orgID := str(decodeBody(t, org.Body.Bytes())["organization_id"])
	if orgID == "" {
		t.Fatal("missing organization_id")
	}
	got, err := s.Store.GetOrg(orgID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Alias != "七牛" || got.ID != orgID {
		t.Fatalf("org row %+v", got)
	}

	team := doJSON(t, h, "POST", "/team/new", master, map[string]any{
		"team_alias": "前端", "organization_id": orgID,
	})
	if team.Code != 200 {
		t.Fatalf("team %d %s", team.Code, team.Body.String())
	}
	teamID := str(decodeBody(t, team.Body.Bytes())["team_id"])
	proj := doJSON(t, h, "POST", "/project/new", master, map[string]any{
		"project_alias": "test", "models": []any{}, "blocked": false, "team_id": teamID,
	})
	if proj.Code != 200 {
		t.Fatalf("project %d %s", proj.Code, proj.Body.String())
	}
	projectID := str(decodeBody(t, proj.Body.Bytes())["project_id"])
	row, err := s.Store.GetProject(projectID)
	if err != nil {
		t.Fatal(err)
	}
	if row.Alias != "test" {
		t.Fatalf("project alias %q", row.Alias)
	}
	if row.Extra()["organization_id"] != orgID {
		t.Fatalf("project organization_id %v want %s extra %s", row.Extra()["organization_id"], orgID, row.ExtraJSON)
	}
}

func TestXormSpendBatchReplay(t *testing.T) {
	st, err := store.Open(testDatabaseURL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.DB.Close() })

	const hash = "abc123spend"
	if err := st.InsertKey(store.Key{TokenHash: hash, KeyAlias: "replay", ModelsJSON: "[]"}); err != nil {
		t.Fatal(err)
	}
	if err := st.InsertOrg(store.Entity{ID: "org_spend", Alias: "spend", ModelsJSON: "[]", CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	row := store.SpendLogRow{
		RequestID: "req-replay",
		APIKey:    hash,
		OrgID:     "org_spend",
		Spend:     sql.NullFloat64{Float64: 1.25, Valid: true},
		Start:     time.Now().UTC(),
		End:       time.Now().UTC(),
	}
	if err := st.ApplySpendBatch([]store.SpendLogRow{row}); err != nil {
		t.Fatal(err)
	}
	if err := st.ApplySpendBatch([]store.SpendLogRow{row}); err != nil {
		t.Fatal(err)
	}
	key, err := st.GetByHash(hash)
	if err != nil {
		t.Fatal(err)
	}
	if !nearSpend(key.Spend, 1.25) {
		t.Fatalf("key spend %v", key.Spend)
	}
	org, err := st.GetOrg("org_spend")
	if err != nil {
		t.Fatal(err)
	}
	if !nearSpend(org.Spend, 1.25) {
		t.Fatalf("org spend %v extra %s", org.Spend, org.ExtraJSON)
	}
	logs, err := st.ListSpendLogs()
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, item := range logs {
		if item["request_id"] == "req-replay" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("spend logs %d", n)
	}
}

func TestConcurrentAddOrgSpendSums(t *testing.T) {
	st, err := store.Open(testDatabaseURL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.DB.Close() })
	if err := st.InsertOrg(store.Entity{ID: "org_sum", Alias: "sum", ModelsJSON: "[]", CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	const n = 20
	var wg sync.WaitGroup
	errCh := make(chan error, 2)
	run := func(delta float64) {
		defer wg.Done()
		for i := 0; i < n; i++ {
			if err := st.AddOrgSpend("org_sum", delta); err != nil {
				errCh <- err
				return
			}
		}
	}
	wg.Add(2)
	go run(1)
	go run(2)
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatal(err)
	}
	got, err := st.GetOrg("org_sum")
	if err != nil {
		t.Fatal(err)
	}
	if !nearSpend(got.Spend, float64(n)*1+float64(n)*2) {
		t.Fatalf("spend %v want %v extra %s", got.Spend, float64(n)*3, got.ExtraJSON)
	}
}

func TestUpdateOrgKeepsFreshSpend(t *testing.T) {
	st, err := store.Open(testDatabaseURL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.DB.Close() })
	if err := st.InsertOrg(store.Entity{ID: "org_alias", Alias: "旧名", ModelsJSON: "[]", CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	stale, err := st.GetOrg("org_alias")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AddOrgSpend("org_alias", 5); err != nil {
		t.Fatal(err)
	}
	stale.Alias = "新名"
	if err := st.UpdateOrg(*stale); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetOrg("org_alias")
	if err != nil {
		t.Fatal(err)
	}
	if got.Alias != "新名" || !nearSpend(got.Spend, 5) {
		t.Fatalf("alias %q spend %v extra %s", got.Alias, got.Spend, got.ExtraJSON)
	}
}
