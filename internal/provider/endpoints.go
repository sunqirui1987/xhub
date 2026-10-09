package provider

// EndpointTypes 返回独立于供应商和具体模型的公开协议描述。
// 参数：无。返回 []EndpointDescriptor：OpenAI、Vertex/Gemini、Claude 与 Bypass 的静态目录。
// 调用：/public/endpoints，供模型配置表单选择协议；不代表每个模型都支持全部端点。
// 测试：validate_test.go、bindings_test.go。
func EndpointTypes() []EndpointDescriptor {
	out := make([]EndpointDescriptor, 0, len(capabilities)+10)
	for _, c := range Capabilities() {
		// 入口协议显式登记，不从能力名称拼接未实现的协议。
		protocol := map[string]string{
			"chat": "openai-chat", "completion": "openai-completions",
			"embedding": "openai-embeddings",
			"video":     "openai-videos", "audio_speech": "openai-audio-speech",
			"audio_transcription": "openai-audio-transcription",
			"rerank":              "rerank", "moderation": "openai-moderations",
		}[c.ID]
		if protocol == "" {
			continue
		}
		out = append(out, EndpointDescriptor{Category: "openai", ID: c.ID, Label: accessLabel(c), Kind: KindAdapted, Protocol: protocol, Family: c.ID, Capability: c.ID, Paths: c.Paths[:1]})
	}
	return append(out,
		EndpointDescriptor{Category: "openai", ID: "audio_translation", Label: "OpenAI · Audio Translation", Kind: KindBypass, Protocol: "openai-audio-translation", Family: "audio_transcription", Paths: []string{"/v1/audio/translations"}},
		EndpointDescriptor{Category: "openai", ID: "image_generation", Label: "OpenAI Images · 创建图片", Kind: KindBypass, Protocol: "openai-images", Family: "image", Paths: []string{"/v1/images/generations"}},
		EndpointDescriptor{Category: "openai", ID: "image_edit", Label: "OpenAI Images · 编辑图片", Kind: KindBypass, Protocol: "openai-images", Family: "image", Paths: []string{"/v1/images/edits"}},
		EndpointDescriptor{Category: "vertex", ID: "gemini", Label: "Gemini · Generate Content", Kind: KindAdapted, Protocol: "gemini", Family: "chat", Paths: []string{"/v1beta/models/{model}:generateContent", "/v1beta/models/{model}:streamGenerateContent"}},
		EndpointDescriptor{Category: "vertex", ID: "vertex", Label: "Vertex AI · Generate Content", Kind: KindAdapted, Protocol: "vertex", Family: "chat", Paths: []string{"/vertex/v1/models/{model}:generateContent", "/vertex/v1/models/{model}:streamGenerateContent"}},
		EndpointDescriptor{Category: "openai", ID: "responses", Label: "OpenAI · Responses", Kind: KindAdapted, Protocol: "openai-responses", Family: "chat", Paths: []string{"/v1/responses"}},
		EndpointDescriptor{Category: "claude", ID: "messages", Label: "Anthropic · Messages", Kind: KindAdapted, Protocol: "anthropic-messages", Family: "chat", Paths: []string{"/v1/messages"}},
		EndpointDescriptor{Category: "bypass", ID: "bypass:gemini", Label: "Bypass · Gemini", Kind: KindBypass, Protocol: "gemini", Family: "chat", Paths: []string{"/bypass/gemini/v1beta/models/{model}:generateContent", "/bypass/gemini/v1beta/models/{model}:streamGenerateContent"}},
		EndpointDescriptor{Category: "bypass", ID: "bypass:vertex", Label: "Bypass · Vertex", Kind: KindBypass, Protocol: "vertex", Family: "chat", Paths: []string{"/bypass/vertex/v1/models/{model}:generateContent", "/bypass/vertex/v1/models/{model}:streamGenerateContent"}},
		EndpointDescriptor{Category: "bypass", ID: "bypass:openai-chat", Label: "Bypass · Chat Completions", Kind: KindBypass, Protocol: "openai-chat", Family: "chat", Paths: []string{"/bypass/openai/v1/chat/completions"}},
		EndpointDescriptor{Category: "bypass", ID: "bypass:openai-responses", Label: "Bypass · OpenAI Responses", Kind: KindBypass, Protocol: "openai-responses", Family: "chat", Paths: []string{"/bypass/openai/v1/responses"}},
		EndpointDescriptor{Category: "bypass", ID: "bypass:anthropic-messages", Label: "Bypass · Anthropic Messages", Kind: KindBypass, Protocol: "anthropic-messages", Family: "chat", Paths: []string{"/bypass/anthropic/v1/messages"}},
		EndpointDescriptor{Category: "bypass", ID: "bypass:openai-images", Label: "Bypass · OpenAI Images Generation", Kind: KindBypass, Protocol: "openai-images", Family: "image"},
		EndpointDescriptor{Category: "bypass", ID: "bypass:openai-image-edit", Label: "Bypass · OpenAI Images Edit", Kind: KindBypass, Protocol: "openai-images", Family: "image"},
		EndpointDescriptor{Category: "bypass", ID: "fal:queue", Label: "Fal · Queue", Kind: KindBypass, Protocol: "fal", Family: "video"},
		EndpointDescriptor{Category: "bypass", ID: "bypass:ark-video", Label: "Bypass · Ark Video", Kind: KindBypass, Protocol: "ark", Family: "video"},
	)
}

// KnownEndpoint 校验公开目录稳定 ID，禁止使用任意路径作为模板入口条件。
// 参数 id：入口 ID；返回是否登记。调用：模板验证，不读取供应商或凭据。
func KnownEndpoint(id string) bool {
	for _, endpoint := range EndpointTypes() {
		if endpoint.ID == id {
			return true
		}
	}
	return false
}

// EndpointForOp 返回操作对应的用户入口 ID，与部署的上游协议独立。
// 参数 op：网关操作名；返回目录 ID。调用：统一选路、计数器及粘性隔离。
func EndpointForOp(op string) string {
	switch op {
	case "audio_translation":
		return "audio_translation"
	case "images":
		return "image_generation"
	case "images_edits":
		return "image_edit"
	case "chat", "responses", "messages", "gemini", "vertex":
		return op
	}
	id, _ := CapabilityForOp(op)
	return id
}

// CompatibleEndpoint 判断一个接入声明能否由指定执行协议处理。
// 参数 e 为公开目录、t 为注册上游；返回兼容状态。校验与选路共用，未知或旧声明无兜底。
// 对话接入允许已实现的五种对话协议转换；媒体按独立操作绑定，防止生图模型被误声明为改图。
func CompatibleEndpoint(e EndpointDescriptor, t Transport) bool {
	if e.Kind == KindAdapted && DialogueProtocol(e.Protocol) && DialogueProtocol(t.Protocol) {
		return true
	}
	if e.Protocol != t.Protocol {
		return false
	}
	if e.Kind == KindBypass {
		return e.ID == t.EndpointID || DialogueProtocol(t.Protocol)
	}
	return e.ID == t.EndpointID
}

// accessLabel 返回接入协议名称；参数为能力目录，返回用户可辨认的原厂名称。
// 调用：公开目录；Chat 使用明确的 Chat Completions，Rerank 不冒充 OpenAI 原厂协议。
func accessLabel(c Capability) string {
	if c.ID == "chat" {
		return "OpenAI · Chat Completions"
	}
	if c.ID == "rerank" {
		return "Rerank"
	}
	return "OpenAI · " + c.Label
}
