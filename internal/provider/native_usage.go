package provider

import (
	"encoding/json"
	"strings"
)

// NativeUsage 按原生协议建立用量提取器，只记录事实，不在传输层决定价格。
// 参数 protocol：注册表中的协议标识；返回函数的 doc 是上游响应，request 是原始请求字段。
// 返回：可供目录计价的用量；缺少事实时带 pricing_blocked，明确失败时返回 nil。
// 调用：原生传输注册、NativeStreamUsage。价格由 catalog 和部署规则决定。
func NativeUsage(protocol string) func(map[string]any, map[string]any) map[string]any {
	return func(doc, request map[string]any) map[string]any {
		if doc["error"] != nil {
			return nil
		}
		if protocol == "openai-responses" {
			if status, _ := doc["status"].(string); status != "" && status != "completed" {
				return map[string]any{"pricing_blocked": "response_not_completed"}
			}
		}
		source, _ := doc["usage"].(map[string]any)
		out := map[string]any{}
		for k, v := range source {
			out[k] = v
		}
		if protocol == "openai-images" {
			if doc["type"] == "image_generation.completed" || doc["type"] == "image_edit.completed" {
				out["images"] = 1
			}
			if data, ok := doc["data"].([]any); ok {
				out["images"] = len(data)
			}
			quality, _ := doc["quality"].(string)
			size, _ := doc["size"].(string)
			if quality == "" {
				quality, _ = request["quality"].(string)
			}
			if size == "" {
				size, _ = request["size"].(string)
			}
			if quality == "auto" || size == "auto" {
				quality, size = "", ""
				out["pricing_blocked"] = "image_variant_unknown"
			}
			if size != "" {
				out["image_variant"] = size
			}
			if quality != "" && size != "" {
				out["image_variant"] = quality + "_" + size
			}
		}
		if len(out) == 0 {
			return map[string]any{"pricing_blocked": "upstream_usage_missing"}
		}
		return out
	}
}

// NativeStreamUsage 读取完整 SSE 记录中的用量，不改写发送给客户端的字节。
// 参数 protocol：原生协议；raw：包含空行分隔符的事件；request：请求中的图片规格等事实。
// 返回：本批事件报告的用量；无事实的中间事件返回 nil，终态缺失事实保留计价阻断原因。
// 调用：dataplane.forwardOfficial。图片的跨事件累加由转发器完成。
func NativeStreamUsage(protocol string, raw []byte, request map[string]any) map[string]any {
	out := map[string]any{}
	source := strings.ReplaceAll(string(raw), "\r\n", "\n")
	for _, event := range strings.Split(source, "\n\n") {
		var data []string
		var eventType string
		for _, line := range strings.Split(event, "\n") {
			if strings.HasPrefix(line, "event:") {
				eventType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			}
			if strings.HasPrefix(line, "data:") {
				data = append(data, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
			}
		}
		var doc map[string]any
		if json.Unmarshal([]byte(strings.Join(data, "\n")), &doc) != nil {
			continue
		}
		typ, _ := doc["type"].(string)
		if typ == "" {
			typ = eventType
			doc["type"] = typ // 事件头与 JSON type 都是协议事件名，统一后才能提取图片完成事实。
		}
		terminal := typ == "response.completed" || typ == "image_generation.completed" || typ == "image_edit.completed"
		if protocol == "openai-images" && !terminal {
			continue // 部分图片事件不是生成结果，不能提前计入图片或规格。
		}
		for _, key := range []string{"response", "message"} {
			if nested, ok := doc[key].(map[string]any); ok {
				doc = nested
				break
			}
		}
		for k, v := range NativeUsage(protocol)(doc, request) {
			if k == "pricing_blocked" && !terminal {
				continue // 中间事件允许没有用量；终态未知规格必须阻止结算。
			}
			out[k] = v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// NativeStreamState 保存一次原生流的完成、失败及图片完成事件数。
// 流正常 EOF 不代表业务成功；结算必须观察到协议规定的完成事件且没有失败事件。
type NativeStreamState struct {
	Completed bool
	Failed    bool
	Images    int
}

// Observe 消费一个完整 SSE 事件，更新状态但不修改事件内容。
// 参数 protocol：协议标识；raw：原始事件。返回：无，结果写入接收者。
// 调用：dataplane.forwardOfficial。错误状态只累积，不会被后续完成事件覆盖。
func (s *NativeStreamState) Observe(protocol string, raw []byte) {
	var data []string
	var event string
	for _, line := range strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n") {
		if strings.HasPrefix(line, "event:") {
			event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		}
		if strings.HasPrefix(line, "data:") {
			data = append(data, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	var doc map[string]any
	_ = json.Unmarshal([]byte(strings.Join(data, "\n")), &doc)
	if typ, _ := doc["type"].(string); typ != "" {
		event = typ
	}
	if event == "error" || event == "response.failed" || event == "response.incomplete" || doc["error"] != nil {
		s.Failed = true
	}
	switch protocol {
	case "openai-responses":
		if event == "response.completed" {
			s.Completed = true
		}
	case "anthropic-messages":
		if event == "message_stop" {
			s.Completed = true
		}
	case "openai-images":
		if event == "image_generation.completed" || event == "image_edit.completed" {
			s.Completed = true
			s.Images++
		}
	}
}
