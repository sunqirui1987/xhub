package llm

import (
	"encoding/json"
	"testing"
)

// TestGoogleDialogueBoundaries 验证配置、工具关联、多候选和未知多模态；参数 t 为上下文，正常输入成功，错误输入拒绝，内存测试无需清理。
func TestGoogleDialogueBoundaries(t *testing.T) {
	cases := []struct {
		name, raw string
		valid     bool
	}{
		{"text", `{"contents":[{"parts":[{"text":"hello"}]}]}`, true},
		{"empty", `{"contents":[]}`, false},
		{"config_type", `{"contents":[{"parts":[{"text":"x"}]}],"generationConfig":[]}`, false},
		{"system_type", `{"contents":[{"parts":[{"text":"x"}]}],"systemInstruction":"x"}`, false},
		{"candidate_count", `{"contents":[{"parts":[{"text":"x"}]}],"generationConfig":{"candidateCount":2}}`, false},
		{"multimodal", `{"contents":[{"parts":[{"inlineData":{"mimeType":"image/png","data":"AA"}}]}]}`, false},
		{"tools_type", `{"contents":[{"parts":[{"text":"x"}]}],"tools":[null]}`, false},
		{"ambiguous_tool", `{"contents":[{"role":"model","parts":[{"functionCall":{"id":"a","name":"f","args":{}}},{"functionCall":{"id":"b","name":"f","args":{}}}]},{"parts":[{"functionResponse":{"name":"f","response":{}}}]}]}`, false},
		{"duplicate_tool", `{"contents":[{"role":"model","parts":[{"functionCall":{"id":"a","name":"f","args":{}}},{"functionCall":{"id":"a","name":"f","args":{}}}]}]}`, false},
		{"explicit_tool", `{"contents":[{"role":"model","parts":[{"functionCall":{"id":"a","name":"f","args":{}}},{"functionCall":{"id":"b","name":"f","args":{}}}]},{"parts":[{"functionResponse":{"id":"b","name":"f","response":{}}},{"functionResponse":{"id":"a","name":"f","response":{}}}]}]}`, true},
		{"orphan_tool", `{"contents":[{"parts":[{"functionResponse":{"id":"a","name":"f","response":{}}}]}]}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var body map[string]any
			if err := json.Unmarshal([]byte(tc.raw), &body); err != nil {
				t.Fatal(err)
			}
			_, err := ParseDialogue("gemini", body)
			if (err == nil) != tc.valid {
				t.Fatalf("Google 输入边界错误 valid=%v err=%v", tc.valid, err)
			}
		})
	}
}

// TestGoogleResultBoundaries 验证安全阻断、多候选、思考用量及缺用量；参数 t 为上下文，错误回复失败，缺用量不计价，无外部数据清理。
func TestGoogleResultBoundaries(t *testing.T) {
	for _, raw := range []string{`{"candidates":[{"finishReason":"SAFETY","content":{"parts":[{"text":"x"}]}}]}`, `{"candidates":[{},{}]}`, `{"candidates":[{"finishReason":"STOP","content":{"parts":[{"text":"x"}]}}],"usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":-1}}`} {
		if _, err := ParseDialogueResult("gemini", []byte(raw)); err == nil {
			t.Fatalf("非法上游回复被接受：%s", raw)
		}
	}
	raw := `{"candidates":[{"finishReason":"STOP","content":{"parts":[{"text":"x"}]}}],"usageMetadata":{"promptTokenCount":8,"candidatesTokenCount":2,"thoughtsTokenCount":3}}`
	result, err := ParseDialogueResult("vertex", []byte(raw))
	if err != nil || result.Usage["completion_tokens"] != float64(5) {
		t.Fatalf("思考用量丢失：%+v %v", result, err)
	}
	result, err = ParseDialogueResult("gemini", []byte(`{"candidates":[{"finishReason":"STOP","content":{"parts":[{"text":"x"}]}}]}`))
	if err != nil || result.Usage["pricing_blocked"] != "upstream_usage_missing" {
		t.Fatalf("缺用量应阻止计价：%+v %v", result, err)
	}
}
