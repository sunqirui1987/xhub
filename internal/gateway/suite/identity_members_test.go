package suite

import (
	"strings"
	"testing"
)

func TestUserNewAutoCreateKeyAndRoles(t *testing.T) {
	s, master := testEnv(t)
	h := s.Handler()
	created := doJSON(t, h, "POST", "/user/new", master, map[string]any{
		"user_email": "k@x.com", "user_role": "internal_user", "max_budget": 9.0,
		"tpm_limit": 100, "metadata": map[string]any{"app": "pay"},
	})
	if created.Code != 200 {
		t.Fatal(created.Body.String())
	}
	m := decodeBody(t, created.Body.Bytes())
	mustKeys(t, m, "user_id", "user_email", "user_role", "spend", "max_budget", "teams", "created_at", "key", "token", "tpm_limit", "metadata", "blocked")
	if m["tpm_limit"] != float64(100) {
		t.Fatalf("tpm %v", m["tpm_limit"])
	}
	if _, ok := m["key"].(string); !ok || m["key"] == "" {
		t.Fatalf("missing auto key %v", m)
	}

	noKey := doJSON(t, h, "POST", "/user/new", master, map[string]any{
		"user_email": "n@x.com", "auto_create_key": false,
	})
	nm := decodeBody(t, noKey.Body.Bytes())
	if _, ok := nm["key"]; ok {
		t.Fatalf("auto_create_key false leaked key %v", nm)
	}

	roles := doJSON(t, h, "GET", "/user/available_roles", master, nil)
	if roles.Code != 200 {
		t.Fatal(roles.Body.String())
	}
	rm := decodeBody(t, roles.Body.Bytes())
	for _, role := range []string{"proxy_admin", "proxy_admin_viewer", "internal_user", "internal_user_viewer"} {
		row, _ := rm[role].(map[string]any)
		if row["ui_label"] == nil || row["description"] == nil {
			t.Fatalf("role %s %v", role, rm)
		}
	}

	bulk := doJSON(t, h, "POST", "/user/bulk_update", master, map[string]any{
		"users": []any{map[string]any{"user_id": m["user_id"], "user_alias": "renamed", "max_budget": 3.0}},
	})
	if bulk.Code != 200 {
		t.Fatal(bulk.Body.String())
	}
	bm := decodeBody(t, bulk.Body.Bytes())
	if bm["successful_updates"] != float64(1) {
		t.Fatalf("bulk %v", bm)
	}
}

func TestTeamMembersAndBlock(t *testing.T) {
	s, master := testEnv(t)
	h := s.Handler()
	tr := doJSON(t, h, "POST", "/team/new", master, map[string]any{
		"team_alias": "eng", "tpm_limit": 50, "metadata": map[string]any{"dept": "plat"},
	})
	tm := decodeBody(t, tr.Body.Bytes())
	mustKeys(t, tm, "team_id", "team_alias", "members_with_roles", "tpm_limit", "metadata", "blocked", "created_at")
	tid := tm["team_id"].(string)

	ur := doJSON(t, h, "POST", "/user/new", master, map[string]any{
		"user_id": "u_mem", "user_email": "m@x.com", "auto_create_key": false,
	})
	if ur.Code != 200 {
		t.Fatal(ur.Body.String())
	}
	add := doJSON(t, h, "POST", "/team/member_add", master, map[string]any{
		"team_id": tid, "member": map[string]any{"user_id": "u_mem", "role": "admin"},
	})
	if add.Code != 200 {
		t.Fatal(add.Body.String())
	}
	am := decodeBody(t, add.Body.Bytes())
	mustKeys(t, am, "team_id", "members_with_roles", "updated_users", "updated_team_memberships")
	members, _ := am["members_with_roles"].([]any)
	if len(members) != 1 {
		t.Fatalf("members %v", members)
	}

	upd := doJSON(t, h, "POST", "/team/member_update", master, map[string]any{
		"team_id": tid, "user_id": "u_mem", "role": "user", "max_budget_in_team": 12.0,
	})
	if upd.Code != 200 {
		t.Fatal(upd.Body.String())
	}
	um := decodeBody(t, upd.Body.Bytes())
	if um["role"] != "user" {
		t.Fatalf("role %v", um)
	}

	info := doJSON(t, h, "GET", "/team/info?team_id="+tid, master, nil)
	if !strings.Contains(info.Body.String(), "u_mem") {
		t.Fatalf("info missing member %s", info.Body.String())
	}

	del := doJSON(t, h, "POST", "/team/member_delete", master, map[string]any{"team_id": tid, "user_id": "u_mem"})
	if del.Code != 200 {
		t.Fatal(del.Body.String())
	}
	dm := decodeBody(t, del.Body.Bytes())
	left, _ := dm["members_with_roles"].([]any)
	if len(left) != 0 {
		t.Fatalf("still members %v", left)
	}

	blk := doJSON(t, h, "POST", "/team/block", master, map[string]any{"team_id": tid})
	if blk.Code != 200 {
		t.Fatal(blk.Body.String())
	}
	if decodeBody(t, blk.Body.Bytes())["blocked"] != true {
		t.Fatalf("blocked %s", blk.Body.String())
	}
	gen := doJSON(t, h, "POST", "/key/generate", master, map[string]any{"key_type": "llm_api", "team_id": tid})
	g := decodeBody(t, gen.Body.Bytes())
	chat := doJSON(t, h, "POST", "/v1/chat/completions", g["key"].(string), map[string]any{
		"model": "gpt-4o-mini", "messages": []any{map[string]any{"role": "user", "content": "hi"}},
	})
	if chat.Code != 401 {
		t.Fatalf("blocked team want 401 got %d %s", chat.Code, chat.Body.String())
	}
	un := doJSON(t, h, "POST", "/team/unblock", master, map[string]any{"team_id": tid})
	if decodeBody(t, un.Body.Bytes())["blocked"] != false {
		t.Fatalf("unblock %s", un.Body.String())
	}
}

func TestOrgMembers(t *testing.T) {
	s, master := testEnv(t)
	h := s.Handler()
	or := doJSON(t, h, "POST", "/organization/new", master, map[string]any{"organization_alias": "acme"})
	om := decodeBody(t, or.Body.Bytes())
	mustKeys(t, om, "organization_id", "organization_alias", "members", "created_at", "metadata")
	oid := om["organization_id"].(string)
	add := doJSON(t, h, "POST", "/organization/member_add", master, map[string]any{
		"organization_id": oid, "member": map[string]any{"user_id": "u1", "role": "org_admin"},
	})
	if add.Code != 200 {
		t.Fatal(add.Body.String())
	}
	am := decodeBody(t, add.Body.Bytes())
	mems, _ := am["members"].([]any)
	if len(mems) != 1 {
		t.Fatalf("org members %v", mems)
	}
	del := doJSON(t, h, "DELETE", "/organization/member_delete", master, map[string]any{
		"organization_id": oid, "user_id": "u1",
	})
	if del.Code != 200 {
		t.Fatal(del.Body.String())
	}
	if len(decodeBody(t, del.Body.Bytes())["members"].([]any)) != 0 {
		t.Fatalf("org member leftover %s", del.Body.String())
	}
}
