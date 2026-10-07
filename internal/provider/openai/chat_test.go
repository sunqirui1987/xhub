package openai_test

import (
	"testing"

	"github.com/sunqirui1987/xhub/internal/provider"
	_ "github.com/sunqirui1987/xhub/internal/provider/openai"
)

// TestAdaptedTypesAreNotRegistered 钉住这次拆分的一个结果：协议适配那一档不再
// 登记成端点类型。
//
// 它原来登记了七条（chat、completion、embedding、image_generation、
// audio_speech、rerank、video_generation），每条带上游路径。那些路径不参与
// 路由——真正决定上游地址的是 llm.Endpoint 里的 (op, 供应商)。留着它们等于
// 把同一件事写两遍，而两遍里的那份是假的。
//
// 现在入口由能力表描述（capability.go），转发由转发方式描述。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestAdaptedTypesAreNotRegistered(t *testing.T) {
	for _, transport := range provider.Transports() {
		if transport.Kind == provider.KindAdapted {
			t.Fatalf("an adapted transport was registered: %+v; adaptations are described by the capability table", transport)
		}
	}
	// 适配的入口不进 bypass 匹配：chat 由 family 循环接走。
	if _, ok := provider.Match("POST", "/v1/chat/completions", nil); ok {
		t.Fatal("chat is adapted and must stay out of bypass matching")
	}
}

// TestCapabilityTableDescribesTheAdaptedEntrypoints 证明七条适配类型变成了能力表。
//
// 对应关系：chat、completion、embedding、image、video、audio_speech、rerank
// 这几条能力都在，而且 image 把生图和改图收成了一条。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestCapabilityTableDescribesTheAdaptedEntrypoints(t *testing.T) {
	want := []string{"chat", "completion", "embedding", "image", "video", "audio_speech", "audio_transcription", "rerank", "moderation"}
	got := map[string]bool{}
	for _, c := range provider.Capabilities() {
		got[c.ID] = true
	}
	for _, id := range want {
		if !got[id] {
			t.Fatalf("capability %s is missing from the table", id)
		}
		delete(got, id)
	}
	if len(got) != 0 {
		t.Fatalf("unexpected capabilities %v", got)
	}
}

// TestRegisterTransportRejectsAnEntryWithNoActions 证明一个没有动作的转发方式不会被登记。
//
// 它无处可转：Match 靠动作的路径命中，没有动作就永远匹配不上。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestRegisterTransportRejectsAnEntryWithNoActions(t *testing.T) {
	before := len(provider.Transports())
	provider.RegisterTransport(provider.Transport{ID: "no-actions", Kind: provider.KindBypass})
	provider.RegisterTransport(provider.Transport{Kind: provider.KindBypass, Actions: []provider.Action{{Name: "x", Method: "POST", PublicPath: "/x"}}})
	if after := len(provider.Transports()); after != before {
		t.Fatalf("a transport with no id or no actions was registered: %d -> %d", before, after)
	}
}
