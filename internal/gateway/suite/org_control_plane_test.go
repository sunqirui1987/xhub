package suite

import (
	"testing"
	"time"

	"github.com/sunqirui1987/xhub/internal/store"
)

func TestInsertProjectOnControlPlaneTable(t *testing.T) {
	st, err := store.Open(testDatabaseURL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.DB.Close() })

	if _, err := st.DB.Exec(`INSERT INTO teams (team_id, team_alias, organization_id, models_json, spend, created_at) VALUES ('team_1', '前端', 'org_50236e0bcfbb', '[]', 0, '2026-01-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB.Exec(`DROP TABLE projects`); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB.Exec(`CREATE TABLE projects (
		id text PRIMARY KEY,
		organization_id text NOT NULL,
		name text NOT NULL,
		environment text NOT NULL DEFAULT 'production',
		created_at timestamptz NOT NULL DEFAULT now(),
		updated_at timestamptz NOT NULL DEFAULT now(),
		blocked integer NOT NULL DEFAULT 0,
		extra_json text,
		UNIQUE (organization_id, name, environment)
	)`); err != nil {
		t.Fatal(err)
	}

	err = st.InsertProject(store.Entity{
		ID: "proj_test", Alias: "test", TeamID: "team_1", ModelsJSON: "[]", CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := st.GetProject("proj_test")
	if err != nil {
		t.Fatal(err)
	}
	if got.Alias != "test" || got.TeamID != "team_1" {
		t.Fatalf("got %+v extra %s", got, got.ExtraJSON)
	}
	if got.Extra()["organization_id"] != "org_50236e0bcfbb" {
		t.Fatalf("organization_id %v", got.Extra()["organization_id"])
	}
}

func TestInsertOrgOnControlPlaneTable(t *testing.T) {
	st, err := store.Open(testDatabaseURL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.DB.Close() })

	if _, err := st.DB.Exec(`DROP TABLE organizations`); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB.Exec(`CREATE TABLE organizations (
		id text PRIMARY KEY,
		name text NOT NULL,
		status text NOT NULL DEFAULT 'active',
		created_at timestamptz NOT NULL DEFAULT now(),
		updated_at timestamptz NOT NULL DEFAULT now(),
		extra_json text
	)`); err != nil {
		t.Fatal(err)
	}

	created := time.Now().UTC().Truncate(time.Second)
	err = st.InsertOrg(store.Entity{
		ID: "org_qiniu", Alias: "七牛", ModelsJSON: "[]", CreatedAt: created,
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := st.GetOrg("org_qiniu")
	if err != nil {
		t.Fatal(err)
	}
	if got.Alias != "七牛" || got.ID != "org_qiniu" {
		t.Fatalf("got %+v", got)
	}
	list, err := st.ListOrgs()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Alias != "七牛" {
		t.Fatalf("list %+v", list)
	}

	if _, err := st.DB.Exec(`DROP TABLE IF EXISTS projects`); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB.Exec(`CREATE TABLE projects (
		id text PRIMARY KEY,
		organization_id text NOT NULL REFERENCES organizations(id),
		name text NOT NULL
	)`); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB.Exec(`INSERT INTO projects (id, organization_id, name) VALUES ('proj_1', 'org_qiniu', 'demo')`); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteOrg("org_qiniu"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetOrg("org_qiniu"); err == nil {
		t.Fatal("organization still present after delete")
	}
	var n int
	if err := st.DB.QueryRow(`SELECT COUNT(*) FROM projects`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("projects left: %d", n)
	}
}
