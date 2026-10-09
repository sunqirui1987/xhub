package provider

// EndpointTypes 返回独立于供应商和具体模型的公开协议描述。
// 参数：无。返回 []EndpointDescriptor：统一端点与原生 Bypass 端点的静态目录。
// 调用：/public/endpoints，供模型配置表单选择协议；不代表每个模型都支持全部端点。
// 测试：validate_test.go、bindings_test.go。
func EndpointTypes() []EndpointDescriptor {
	out := make([]EndpointDescriptor, 0, len(capabilities)+6)
	for _, c := range Capabilities() {
		// 入口协议显式登记，不从能力名称拼接未实现的协议。
		protocol := map[string]string{
			"chat": "openai-chat", "completion": "openai-completions",
			"embedding": "openai-embeddings", "image": "openai-images",
			"video": "openai-videos", "audio_speech": "openai-audio-speech",
			"audio_transcription": "openai-audio-transcription",
			"rerank":              "rerank", "moderation": "openai-moderations",
		}[c.ID]
		if protocol == "" {
			continue
		}
		out = append(out, EndpointDescriptor{ID: c.ID, Label: c.Label, Kind: KindAdapted, Protocol: protocol, Family: c.ID, Capability: c.ID, Paths: c.Paths})
	}
	return append(out,
		EndpointDescriptor{ID: "responses", Label: "统一 · Responses", Kind: KindAdapted, Protocol: "openai-responses", Family: "chat", Paths: []string{"/v1/responses"}},
		EndpointDescriptor{ID: "messages", Label: "统一 · Messages", Kind: KindAdapted, Protocol: "anthropic-messages", Family: "chat", Paths: []string{"/v1/messages"}},
		EndpointDescriptor{ID: "bypass:openai-chat", Label: "Bypass · Chat Completions", Kind: KindBypass, Protocol: "openai-chat", Family: "chat", Paths: []string{"/bypass/openai/v1/chat/completions"}},
		EndpointDescriptor{ID: "bypass:openai-responses", Label: "Bypass · OpenAI Responses", Kind: KindBypass, Protocol: "openai-responses", Family: "chat", Paths: []string{"/bypass/openai/v1/responses"}},
		EndpointDescriptor{ID: "bypass:anthropic-messages", Label: "Bypass · Anthropic Messages", Kind: KindBypass, Protocol: "anthropic-messages", Family: "chat", Paths: []string{"/bypass/anthropic/v1/messages"}},
		EndpointDescriptor{ID: "bypass:openai-images", Label: "Bypass · OpenAI Images Generation", Kind: KindBypass, Protocol: "openai-images", Family: "image"},
		EndpointDescriptor{ID: "bypass:openai-image-edit", Label: "Bypass · OpenAI Images Edit", Kind: KindBypass, Protocol: "openai-images", Family: "image"},
		EndpointDescriptor{ID: "bypass:fal-video", Label: "Bypass · Fal Video", Kind: KindBypass, Protocol: "fal", Family: "video"},
		EndpointDescriptor{ID: "bypass:ark-video", Label: "Bypass · Ark Video", Kind: KindBypass, Protocol: "ark", Family: "video"},
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
	case "chat", "responses", "messages":
		return op
	}
	id, _ := CapabilityForOp(op)
	return id
}
