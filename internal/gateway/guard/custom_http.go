package guard

import (
	"encoding/json"
	"github.com/sunqirui1987/xhub/internal/httpx"
	"net/http"
)

// testCustomCode 执行未保存 XGo 的管理调试，使用与真实请求相同的编译和执行器。
// 参数：w/r：HTTP 响应与 POST 请求；custom_code 源码，test_input.texts 有序数组，request_data 模型与元数据。
// 返回：无；参数错误写 400，编译/运行结果写 200 的 success/result 或 error/error_type。block 是成功执行结果。
// 调用：Manage 的 /guardrails/test_custom_code 分支。
// 测试：custom_test.go、internal/gateway/guardrail_manage_test.go。
func testCustomCode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.WriteError(w, 405, "method_not_allowed", "POST required")
		return
	}
	var body map[string]any
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<20)).Decode(&body); err != nil {
		httpx.WriteError(w, 400, "invalid_request", "JSON object required")
		return
	}
	code := str(body["custom_code"])
	if _, err := compileCustom(code); err != nil {
		httpx.WriteJSON(w, 200, map[string]any{"success": false, "error": err.Error(), "error_type": "compilation"})
		return
	}
	input, _ := body["test_input"].(map[string]any)
	request, _ := body["request_data"].(map[string]any)
	texts := extraWords(input["texts"])
	// 保留空文本的位置；Modify 必须返回与输入数量、顺序一一对应的文本。
	if rows, ok := input["texts"].([]any); ok {
		texts = make([]string, len(rows))
		for i, row := range rows {
			v, yes := row.(string)
			if !yes {
				httpx.WriteError(w, 400, "invalid_request", "texts must contain strings")
				return
			}
			texts[i] = v
		}
	} else {
		httpx.WriteError(w, 400, "invalid_request", "test_input.texts must be an array")
		return
	}
	if request == nil {
		request = map[string]any{}
	}
	if request["model"] == nil {
		request["model"] = input["model"]
	}
	inputType := str(body["input_type"])
	if inputType == "" {
		inputType = "request"
	}
	action, reason, out, metadata, err := runCustomDetailed(code, texts, request, inputType)
	if err != nil {
		httpx.WriteJSON(w, 200, map[string]any{"success": false, "error": err.Error(), "error_type": "execution"})
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"success": true, "result": map[string]any{"metadata": metadata, "action": action, "reason": reason, "texts": out}})
}
