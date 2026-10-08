package gateway

import (
	"context"
	"encoding/json"
	"github.com/sunqirui1987/xhub/internal/iam"
	"net/http"
	"testing"
)

func TestTemplateVisibilityOrdinaryWrites(t *testing.T) {
	f := newPermFixture(t)
	call := func(a actor, path string, body map[string]any) (int, []byte) {
		raw, _ := json.Marshal(body)
		return f.call(a, http.MethodPost, path, raw)
	}
	code, raw := call(f.admin, "/route_template/new", map[string]any{"name": "foreign private", "organization_id": f.teamB.OrganizationID, "body": map[string]any{}})
	if code != 200 {
		t.Fatalf("setup: %d %s", code, raw)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	id := doc["id"].(string)
	for _, path := range []string{"/route_template/binding", "/team/update"} {
		code, raw = call(f.teamAdmin, path, map[string]any{"scope": "team", "scope_id": f.teamA.ID, "team_id": f.teamA.ID, "team_alias": "must not change", "route_template_id": id})
		if code != 404 {
			t.Fatalf("%s accepted invisible template: %d %s", path, code, raw)
		}
	}
	team, err := f.db.GetTeam(context.Background(), f.teamA.ID)
	if err != nil {
		t.Fatal(err)
	}
	if team.Name == "must not change" || team.RouteTemplateID != nil {
		t.Fatal("rejected save mutated team")
	}
	code, raw = call(f.teamMember, "/key/generate", map[string]any{"team_id": f.teamA.ID, "route_template_id": id})
	if code != 404 {
		t.Fatalf("generate accepted invisible template: %d %s", code, raw)
	}
	k, _, err := f.db.CreateKey(context.Background(), iam.Actor{Kind: "system"}, iam.KeyInput{TeamID: f.teamA.ID, UserID: f.teamMember.user.ID, OwnerType: iam.OwnerPersonal})
	if err != nil {
		t.Fatal(err)
	}
	code, raw = call(f.teamMember, "/key/update", map[string]any{"key": k.ID, "route_template_id": id})
	if code != 404 {
		t.Fatalf("update accepted invisible template: %d %s", code, raw)
	}
	selected, err := f.db.ScopeRouteTemplate(context.Background(), "key", k.ID)
	if err != nil || selected != "" {
		t.Fatalf("rejected save mutated key: %q %v", selected, err)
	}
	code, raw = call(f.teamMember, "/key/bulk_update", map[string]any{"keys": []string{k.ID}, "route_template_id": id})
	if code != 404 {
		t.Fatalf("bulk update accepted invisible template: %d %s", code, raw)
	}
	code, raw = call(f.teamAdmin, "/key/service-account/generate", map[string]any{"team_id": f.teamA.ID, "route_template_id": id})
	if code != 404 {
		t.Fatalf("service account accepted invisible template: %d %s", code, raw)
	}
	code, raw = call(f.admin, "/route_template/new", map[string]any{"name": "platform selectable", "body": map[string]any{}})
	if code != 200 {
		t.Fatalf("platform setup: %d %s", code, raw)
	}
	json.Unmarshal(raw, &doc)
	for _, selection := range []any{doc["id"], "", nil} {
		code, raw = call(f.teamMember, "/key/update", map[string]any{"key": k.ID, "route_template_id": selection})
		if code != 200 {
			t.Fatalf("allowed select/clear: %d %s", code, raw)
		}
	}
}
