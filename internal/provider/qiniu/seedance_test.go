package qiniu_test

import (
	"testing"

	"github.com/sunqirui1987/xhub/internal/provider"
	_ "github.com/sunqirui1987/xhub/internal/provider/qiniu"
)

func TestQiniuSeedanceKeepsTheBytedancePrefix(t *testing.T) {
	typ := provider.Transport{}
	for _, item := range provider.Transports() {
		if item.ID == "qiniu_contents_generation" {
			typ = item
		}
	}
	if typ.ID == "" || typ.StripPrefix != "qiniu" || typ.APIBase != "https://api.qnaigc.com" {
		t.Fatalf("type %+v", typ)
	}
	if len(typ.Providers) != 1 || typ.Providers[0] != "qiniu" {
		t.Fatalf("providers %v", typ.Providers)
	}
	create, ok := provider.Match("POST", "/v3/contents/generations/tasks", nil)
	if !ok || create.Transport.ID != "qiniu_contents_generation" || create.Action.Name != "create" {
		t.Fatalf("create %+v %v", create.Transport.ID, ok)
	}
	if _, ok := provider.Match("POST", "/api/v3/contents/generations/tasks", nil); ok {
		t.Fatal("qiniu registration matched the volcengine path")
	}
	models := []string{
		"bytedance/doubao-seedance-2-0-260128",
		"bytedance/doubao-seedance-2-0-fast-260128",
		"bytedance/doubao-seedance-2-0-mini-260615",
		"bytedance/doubao-seedance-2-5-260628",
	}
	ends := provider.ModelEndpoints()
	for _, id := range models {
		key := "qiniu/" + id
		if ends[key] != "qiniu_contents_generation" {
			t.Fatalf("%s default %s", key, ends[key])
		}
		if got := provider.OfficialID("qiniu", key); got != id {
			t.Fatalf("official %s -> %s", key, got)
		}
	}
}
