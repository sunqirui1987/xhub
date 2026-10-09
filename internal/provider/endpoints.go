package provider

// EndpointTypes 返回独立于供应商和具体模型的公开协议描述。
// 参数：无。返回 []EndpointDescriptor：统一端点与原生 Bypass 端点的静态目录。
// 调用：/public/endpoints，供模型配置表单选择协议；不代表每个模型都支持全部端点。
// 测试：validate_test.go、bindings_test.go。
func EndpointTypes() []EndpointDescriptor {
	out := make([]EndpointDescriptor, 0, len(capabilities)+6)
	for _, c := range Capabilities() {
		out = append(out, EndpointDescriptor{ID: c.ID, Label: c.Label, Kind: KindAdapted, Protocol: "adapted", Family: c.ID, Capability: c.ID})
	}
	return append(out,
		EndpointDescriptor{ID: "bypass:openai-responses", Label: "Bypass · OpenAI Responses", Kind: KindBypass, Protocol: "openai-responses", Family: "chat"},
		EndpointDescriptor{ID: "bypass:anthropic-messages", Label: "Bypass · Anthropic Messages", Kind: KindBypass, Protocol: "anthropic-messages", Family: "chat"},
		EndpointDescriptor{ID: "bypass:openai-images", Label: "Bypass · OpenAI Images Generation", Kind: KindBypass, Protocol: "openai-images", Family: "image"},
		EndpointDescriptor{ID: "bypass:openai-image-edit", Label: "Bypass · OpenAI Images Edit", Kind: KindBypass, Protocol: "openai-images", Family: "image"},
		EndpointDescriptor{ID: "bypass:fal-video", Label: "Bypass · Fal Video", Kind: KindBypass, Protocol: "fal", Family: "video"},
		EndpointDescriptor{ID: "bypass:ark-video", Label: "Bypass · Ark Video", Kind: KindBypass, Protocol: "ark", Family: "video"},
	)
}
