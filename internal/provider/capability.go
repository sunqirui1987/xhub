package provider

import "strings"

// 这个文件是这次拆分的事实来源：能力表、存量 id 的映射，以及"一条部署到底
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
		// messages 和 responses 说的是同一件事的另外两种拼法，所以它们和
		// chat 同属一条能力。gemini 是 generateContent 这个入口，
		// 不是给运维勾的选项，所以它只在 Ops 里，没有展示路径。
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
		// 空 op 和 chat 是一回事：老的部署和 /chat 这种老路径都走这一支。
		return "chat", true
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

// CustomTransportID 是部署自带文档那一档的 id。
const CustomTransportID = "custom"

// legacyCapabilities 是存量端点类型 id 到能力 id 的映射。
//
// 键是库里真实存在的值，来自添加模型表单写进 model_info.mode 的那些字符串。
// 注意这些是**类型 id**，不是数据面 op：表单写的是 image_generation 而不是
// images。三套词并存（类型 id、inferenceOp 的 op、Playground 的 EndpointType），
// 这张表只认存量 id。
//
// 认不出的 id（realtime、batch、ocr）刻意不在这里：它们要么是假会话，
// 要么根本没登记成端点，放进能力表会让运维以为可以勾选。
var legacyCapabilities = map[string]string{
	"chat":                "chat",
	"responses":           "chat",
	"anthropic_messages":  "chat",
	"completion":          "completion",
	"embedding":           "embedding",
	"image_generation":    "image",
	"image_edit":          "image",
	"video_generation":    "video",
	"audio_speech":        "audio_speech",
	"audio_transcription": "audio_transcription",
	"rerank":              "rerank",
}

// legacyTransports 是存量端点类型 id 到内置转发方式 id 的映射。
// 只有内置 Bypass 在这里；协议适配不进表。
var legacyTransports = map[string]string{
	"ark_contents_generation":   "ark_contents_generation",
	"qiniu_contents_generation": "qiniu_contents_generation",
}

// capabilityByLegacyID 把存量 id 收成能力 id。认不出时返回假。
// 参数 id（string）：存量端点类型 id，例如 image_generation。
// 返回 string（string）：对应的能力 id；bool（bool）：这个 id 是一条能力时为真。
// 调用：SelectedCapabilities。
// 测试：registry_test.go
func capabilityByLegacyID(id string) (string, bool) {
	capability, ok := legacyCapabilities[strings.TrimSpace(id)]
	return capability, ok
}

// transportByLegacyID 把存量 id 收成内置转发方式 id。不是内置 Bypass 时返回假。
// 参数 id（string）：存量端点类型 id，例如 qiniu_contents_generation。
// 返回 string（string）：对应的转发方式 id；bool（bool）：这个 id 是内置转发时为真。
// 调用：SelectedTransport。
// 测试：registry_test.go
func transportByLegacyID(id string) (string, bool) {
	transport, ok := legacyTransports[strings.TrimSpace(id)]
	return transport, ok
}

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
