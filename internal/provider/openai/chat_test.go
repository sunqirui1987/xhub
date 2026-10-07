package openai_test

import (
	"testing"

	"github.com/sunqirui1987/xhub/internal/provider"
	_ "github.com/sunqirui1987/xhub/internal/provider/openai"
)

func TestAdaptedTypesMatchTheCatalog(t *testing.T) {
	want := map[string]struct{ method, public, upstream string }{
		"chat":             {"POST", "/v1/chat/completions", "/chat/completions"},
		"completion":       {"POST", "/v1/completions", "/completions"},
		"embedding":        {"POST", "/v1/embeddings", "/embeddings"},
		"image_generation": {"POST", "/v1/images/generations", "/images/generations"},
		"audio_speech":     {"POST", "/v1/audio/speech", "/audio/speech"},
		"rerank":           {"POST", "/v1/rerank", "/rerank"},
		"video_generation": {"POST", "/v1/videos", "/videos"},
	}
	for _, typ := range provider.Types() {
		row, ok := want[typ.ID]
		if !ok {
			continue
		}
		if typ.Kind != provider.KindAdapted || typ.ModelField != "model" || len(typ.Actions) != 1 {
			t.Fatalf("%s shape %+v", typ.ID, typ)
		}
		action := typ.Actions[0]
		if action.Method != row.method || action.PublicPath != row.public || action.UpstreamPath != row.upstream {
			t.Fatalf("%s action %+v", typ.ID, action)
		}
		delete(want, typ.ID)
	}
	if len(want) != 0 {
		t.Fatalf("missing types %v", want)
	}
	if _, ok := provider.Match("POST", "/v1/chat/completions", nil); ok {
		t.Fatal("chat is adapted and must stay out of bypass matching")
	}
}
