package volcengine_test

import (
	"testing"

	"github.com/sunqirui1987/xhub/internal/catalog"
	"github.com/sunqirui1987/xhub/internal/provider"
	_ "github.com/sunqirui1987/xhub/internal/provider/volcengine"
)

func TestSeedanceModelsAndPaths(t *testing.T) {
	typ := find(t, "ark_contents_generation")
	if typ.Kind != provider.KindBypass || typ.StripPrefix != "volcengine" || typ.TaskID != "id" || typ.ModelField != "model" {
		t.Fatalf("type %+v", typ)
	}
	if len(typ.Providers) != 1 || typ.Providers[0] != "volcengine" {
		t.Fatalf("providers %v", typ.Providers)
	}
	if typ.APIBase != "https://ark.cn-beijing.volces.com" {
		t.Fatalf("base %s", typ.APIBase)
	}
	names := map[string]string{}
	for _, action := range typ.Actions {
		names[action.Name] = action.Method + " " + action.PublicPath
	}
	if names["create"] != "POST /api/v3/contents/generations/tasks" ||
		names["get"] != "GET /api/v3/contents/generations/tasks/{id}" ||
		names["list"] != "GET /api/v3/contents/generations/tasks" {
		t.Fatalf("actions %v", names)
	}
	create, ok := provider.Match("POST", "/api/v3/contents/generations/tasks", nil)
	get, getOK := provider.Match("GET", "/api/v3/contents/generations/tasks/cgt-1", nil)
	list, listOK := provider.Match("GET", "/api/v3/contents/generations/tasks", nil)
	if !ok || !getOK || !listOK || create.Action.Name == get.Action.Name || get.Names["id"] != "cgt-1" || list.Action.Name != "list" {
		t.Fatalf("match create=%s get=%s list=%s", create.Action.Name, get.Action.Name, list.Action.Name)
	}
	if _, ok := provider.Match("POST", "/v3/contents/generations/tasks", nil); ok {
		t.Fatal("volcengine path matched the qiniu path")
	}
	if provider.ModelEndpoints()["volcengine/doubao-seedance-2-0-260128"] != "ark_contents_generation" ||
		provider.ModelEndpoints()["volcengine/doubao-seedance-2-0-fast-260128"] != "ark_contents_generation" {
		t.Fatal("seedance models did not default to the ark endpoint")
	}
	if got := provider.OfficialID("volcengine", "volcengine/doubao-seedance-2-0-260128"); got != "doubao-seedance-2-0-260128" {
		t.Fatalf("official id %s", got)
	}
	in, out, ok := catalog.TokenRates("volcengine/doubao-seedance-2-0-260128")
	if !ok || in != 7.0/1_000_000 || out != in {
		t.Fatalf("price %v %v %v", in, out, ok)
	}
	if _, _, ok := catalog.TokenRates("volcengine/doubao-seedance-2-0-fast-260128"); ok {
		t.Fatal("fast has no list price")
	}
}

func find(t *testing.T, id string) provider.Transport {
	t.Helper()
	for _, typ := range provider.Transports() {
		if typ.ID == id {
			return typ
		}
	}
	t.Fatalf("missing type %s", id)
	return provider.Transport{}
}
