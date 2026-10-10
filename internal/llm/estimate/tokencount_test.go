package estimate

import (
	"slices"
	"testing"
)

// TestCountTokensContract 验证文本、消息和未知模型回退的计数契约，不能把估算当作供应商实际 usage。
// 前置为离线 tokenizer，覆盖空输入、提示词优先、消息开销、非法内容类型和模型缺失；不写外部数据。
func TestCountTokensContract(t *testing.T) {
	plain, kind, err := CountTokens("gpt-4", "hello", nil)
	if err != nil || plain != 1 || kind != "openai_tokenizer" {
		t.Fatalf("文本计数错误: count=%d kind=%q err=%v", plain, kind, err)
	}
	unknown, kind, err := CountTokens("unknown-test-model", "hello", nil)
	if err != nil || unknown != plain || kind != "openai_tokenizer" {
		t.Fatalf("未知模型未回退到基础 tokenizer: %d/%q/%v", unknown, kind, err)
	}
	for _, tc := range []struct {
		name, prompt string
		messages     []map[string]any
		want         int
	}{
		{"空文本", "", nil, 0}, {"空消息列表", "", []map[string]any{}, 3},
		{"文本优先于消息", "hello", []map[string]any{{"role": "user", "content": "different text"}}, 1},
		{"一条消息包含协议开销", "", []map[string]any{{"role": "user", "content": "hello"}}, 8},
		{"名称增加名称和标记开销", "", []map[string]any{{"role": "user", "content": "hello", "name": "Bob"}}, 10},
		{"非法内容类型不按文本估算", "", []map[string]any{{"role": 42, "content": []any{"hello"}, "name": false}}, 6},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, kind, err := CountTokens("gpt-4", tc.prompt, tc.messages)
			if err != nil || got != tc.want || kind != "openai_tokenizer" {
				t.Fatalf("输入计数契约错误: got=%d want=%d kind=%q err=%v", got, tc.want, kind, err)
			}
		})
	}
}

// TestSupportedParamsContract 验证旧模型的 response_format 限制、目录模型的 user 参数及返回值独立性。
// 前置为模型名和目录标志，正常、未知及空模型采用默认参数集合，调用方修改列表不会污染下一次调用，无需清理。
func TestSupportedParamsContract(t *testing.T) {
	for _, model := range []string{"gpt-4", "gpt-3.5-turbo-16k", "gpt-4o", "unknown", ""} {
		for _, catalog := range []bool{false, true} {
			params := OpenAISupportedParams(model, catalog)
			format := model != "gpt-4" && model != "gpt-3.5-turbo-16k"
			if slices.Contains(params, "response_format") != format || slices.Contains(params, "user") != catalog || !slices.Contains(params, "stream") {
				t.Fatalf("模型 %q catalog=%v 支持参数错误: %v", model, catalog, params)
			}
			params[0] = "caller-mutation"
			if slices.Contains(OpenAISupportedParams(model, catalog), "caller-mutation") {
				t.Fatalf("模型 %q 参数列表在调用间共享", model)
			}
		}
	}
}

// TestModelUsedForCountContract 验证部署模型优先及首个供应商前缀剥离，保留无部署时的请求模型。
// 前置为纯字符串，覆盖空白、缺失、无前缀、多段与边界斜杠；结果供 gateway 选择 tokenizer，无副作用。
func TestModelUsedForCountContract(t *testing.T) {
	for _, tc := range []struct{ request, deployment, want string }{
		{"request", " openai/gpt-4o ", "gpt-4o"}, {"request", "gpt-4o", "gpt-4o"}, {"request", "  ", "request"},
		{"request", "provider/nested/model", "nested/model"}, {"request", "/model", "/model"}, {"request", "provider/", ""}, {"", "", ""},
	} {
		if got := ModelUsedForCount(tc.request, tc.deployment); got != tc.want {
			t.Fatalf("部署模型 %q 请求模型 %q: got=%q want=%q", tc.deployment, tc.request, got, tc.want)
		}
	}
}
