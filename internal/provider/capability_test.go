package provider_test

import (
	"testing"

	"github.com/sunqirui1987/xhub/internal/provider"
)

// opOf 把一条展示路径走一遍 inferenceOp 得到的操作名抄在这里。
//
// 抄一份是刻意的：这样做测试能独立于实现回答"这条路径属于哪种能力"，
// 而实现改了 inferenceOp 时，这份表会在测试里红掉——那正是要发现的事。
// 认入口仍然用 gateway/family 的 inferenceOp，不从能力表的 Paths 反推。
var opOf = map[string]string{
	"/v1/chat/completions":     "chat",
	"/v1/messages":             "messages",
	"/v1/responses":            "responses",
	"/v1/completions":          "completions",
	"/v1/embeddings":           "embeddings",
	"/v1/images/generations":   "images",
	"/v1/images/edits":         "images_edits",
	"/v1/videos":               "videos",
	"/v1/audio/speech":         "audio_speech",
	"/v1/audio/transcriptions": "audio_transcription",
	"/v1/audio/translations":   "audio_translation",
	"/v1/rerank":               "rerank",
	"/v2/rerank":               "rerank",
	"/v1/moderations":          "moderations",
}

// TestEveryDeclaredPathBelongsToItsOwnCapability 是这次拆分最要紧的一条：
// 每条能力声明的展示路径，经过 inferenceOp 得到的 op，必须属于这条能力。
//
// 两边一旦走散，界面上会出现一个"选了却调不动"的入口——而这件事只有把
// 路径表和 op 表放在一起比才看得出来。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestEveryDeclaredPathBelongsToItsOwnCapability(t *testing.T) {
	for _, c := range provider.Capabilities() {
		for _, path := range c.Paths {
			op, known := opOf[path]
			if !known {
				t.Fatalf("capability %s declares %s, which this test does not know how to classify; add it to opOf", c.ID, path)
			}
			got, ok := provider.CapabilityForOp(op)
			if !ok {
				t.Fatalf("capability %s declares %s (op %s), but no capability answers that op", c.ID, path, op)
			}
			if got != c.ID {
				t.Fatalf("%s belongs to %s according to inferenceOp, but %s declares it", path, got, c.ID)
			}
		}
	}
}

// TestChatCoversThreeSpellings 证明 chat 覆盖三条入口路径。
//
// 这三种说的是同一件事，让运维在添加模型时二选一，会把本来能跑的组合挡掉。
// gemini 也在 chat 里，但它没有展示路径：那是 generateContent 这个入口，
// 不是给运维勾的选项。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestChatCoversThreeSpellings(t *testing.T) {
	for _, op := range []string{"chat", "messages", "responses"} {
		got, ok := provider.CapabilityForOp(op)
		if !ok || got != "chat" {
			t.Fatalf("op %s maps to %q (known=%v), want chat", op, got, ok)
		}
	}
}

// TestCompletionIsNotPartOfChat 钉住一个刻意的例外：/v1/completions 不并进 chat。
//
// 它的正文是 prompt，上游是 /completions，没有任何改写器把两者互转。
// 并进去会让一条只答对话的部署被 /v1/completions 选中，然后把 prompt 正文
// 发到 /chat/completions 上。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestCompletionIsNotPartOfChat(t *testing.T) {
	got, ok := provider.CapabilityForOp("completions")
	if !ok || got != "completion" {
		t.Fatalf("completions maps to %q (known=%v), want its own completion capability", got, ok)
	}
}

// TestUnregisteredOpsAreNotCapabilities 证明 realtime、batch、ocr 不会变成可勾选的项。
//
// realtime 在 inferenceOp 里认得出，但数据面不接它，而是造一份假会话；
// batch 和 ocr 只在前端的一份清单里。把它们做成能力，会让运维以为选了就能用。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestUnregisteredOpsAreNotCapabilities(t *testing.T) {
	for _, op := range []string{"realtime", "batch", "ocr", "count_tokens", ""} {
		if got, ok := provider.CapabilityForOp(op); ok {
			t.Fatalf("op %s was mapped to capability %q; it must not be selectable", op, got)
		}
	}
}

// TestImageCoversGenerationAndEdit 证明生图和改图是同一条能力下的两个入口。
//
// 两者共用一条模型很常见，分成两条会逼运维为同一个模型建两条部署。
// 参数 t（*testing.T）：当前测试。
// 返回：无。
func TestImageCoversGenerationAndEdit(t *testing.T) {
	for _, op := range []string{"images", "images_edits"} {
		if got, _ := provider.CapabilityForOp(op); got != "image" {
			t.Fatalf("op %s maps to %q, want image", op, got)
		}
	}
}
