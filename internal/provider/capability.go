package provider

import (
	"strings"

	"github.com/sunqirui1987/xhub/internal/logx"
)

// 这个文件是能力表，以及"一条部署到底
// 应答什么、怎么转发"的判定。读路径全部集中在这里，调用方只拿能力列表和
// transport id，不自己去猜字符串。

// capabilities 是全部入口能力，按登记顺序返回给表单。
//
// Ops 里的名字必须和 gateway/family 的 inferenceOp 返回值一致。有一条测试
// （capability_test.go）拿 Paths 走一遍 inferenceOp，核对得到的 op 都在这条
// 能力的 Ops 里，这样两边走散时会在测试里红掉，而不是在线上悄悄放过请求。
var capabilities = []Capability{
	{
		ID: "chat", Label: "Chat",
		// messages、responses 和 chat 是三种公开协议入口，但都由对话适配器
		// 承担，因此属于同一个“对话能力”。gemini 是内部路由识别出的
		// generateContent 入口，不作为配置页面上的独立选项，也没有展示路径。
		Ops:   []string{"chat", "messages", "responses", "gemini"},
		Paths: []string{"/v1/chat/completions", "/v1/messages", "/v1/responses"},
	},
	{
		// 单独一条，不并进 chat：正文是 prompt，上游是 /completions，
		// 没有任何改写器把两者互转。
		ID: "completion", Label: "Completion",
		Ops:   []string{"completions"},
		Paths: []string{"/v1/completions"},
	},
	{
		ID: "embedding", Label: "Embedding",
		Ops:   []string{"embeddings"},
		Paths: []string{"/v1/embeddings"},
	},
	{
		// 生图和改图共用一条模型很常见，所以它们是一条能力下的两个入口。
		ID: "image", Label: "Image",
		Ops:   []string{"images", "images_edits"},
		Paths: []string{"/v1/images/generations", "/v1/images/edits"},
	},
	{
		ID: "video", Label: "Video",
		Ops:   []string{"videos"},
		Paths: []string{"/v1/videos"},
	},
	{
		ID: "audio_speech", Label: "Audio Speech",
		Ops:   []string{"audio_speech"},
		Paths: []string{"/v1/audio/speech"},
	},
	{
		ID: "audio_transcription", Label: "Audio Transcription",
		Ops:   []string{"audio_transcription", "audio_translation"},
		Paths: []string{"/v1/audio/transcriptions", "/v1/audio/translations"},
	},
	{
		ID: "rerank", Label: "Rerank",
		Ops:   []string{"rerank"},
		Paths: []string{"/v1/rerank", "/v2/rerank"},
	},
	{
		ID: "moderation", Label: "Moderation",
		Ops:   []string{"moderations"},
		Paths: []string{"/v1/moderations"},
	},
}

// Capabilities 返回全部入口能力。
// 参数：无。
// 返回 []Capability（[]Capability）：能力表。调用方可以改返回的切片，不影响目录。
// 调用：PublicBody 组装 /public/endpoints。
// 测试：capability_test.go
func Capabilities() []Capability {
	out := make([]Capability, len(capabilities))
	copy(out, capabilities)
	for i := range out {
		out[i].Ops = append([]string(nil), capabilities[i].Ops...)
		out[i].Paths = append([]string(nil), capabilities[i].Paths...)
	}
	return out
}

// CapabilityForOp 返回应答这个数据面 op 的能力 id。认不出 op 时返回假。
//
// 适配路径用它过滤部署：一条只标了 embedding 的部署不该应答
// /v1/chat/completions。
// 参数 op（string）：数据面操作名，取值来自 inferenceOp。
// 返回 string（string）：这条 op 归属的能力 id；bool（bool）：有哪个能力认这条 op 时为真。
// 调用：dataplane/serve.go 在 router.Order 之后过滤适配池。
// 测试：capability_test.go
func CapabilityForOp(op string) (string, bool) {
	op = strings.TrimSpace(op)
	if op == "" {
		// 新系统要求数据面在进入能力过滤前明确给出操作名。空值没有足够
		// 信息判断它是对话、图片还是其他入口，不能偷偷退回 chat。
		return "", false
	}
	for _, c := range capabilities {
		for _, covered := range c.Ops {
			if covered == op {
				return c.ID, true
			}
		}
	}
	return "", false
}

// AdaptedTransportID 是协议适配那一档的 id。它不进登记表：行为由
// (op, 供应商) 决定，没有可登记的数据。
const AdaptedTransportID = "adapted"

// isKnownCapability 判断这个 id 是不是一条登记过的能力。
// 参数 id（string）：要判断的 id。
// 返回 bool（bool）：这个 id 在能力表里时为真。
// 调用：SelectedCapabilities 在读取新写入的 endpoint_types 时。
// 测试：registry_test.go
func isKnownCapability(id string) bool {
	for _, c := range capabilities {
		if c.ID == id {
			return true
		}
	}
	return false
}

// 条数变了通常意味着有人加了一条能力而没想清楚它的 op 覆盖。
// init 记一次能力表的载入，并把条数写下来。能力表是这次拆分的事实来源，
// 条数变了通常意味着有人加了一条能力而没想清楚它的 op 覆盖。
// 参数：无。
// 返回：无。只写一行进程日志，含能力条数。
// 调用：Go 在载入这个包时自动执行。
// 测试：无直接单测
func init() { logx.Debug("provider capabilities loaded count=%d", len(capabilities)) }
