// Package router orders deployments that share one public model name. After a deployment is chosen, encoding and decoding are delegated to the llm package.
package router

import "github.com/sunqirui1987/xhub/internal/llm"

// DecodeResponse turns an upstream response into the public shape and puts the caller's model alias back into the model field.
// 参数 op（string）：操作名，例如 chat；provider（string）：供应商标识，例如 openai 或 volcengine；alias（string）：对外模型名；raw（[]byte）：原始文本或 JSON 字节。
// 返回 []byte（[]byte）：解码响应的原始字节。没有内容时长度为 0。
// 调用：gateway/spend.go
// 测试：无直接单测
func DecodeResponse(op, provider, alias string, raw []byte) []byte {
	return llm.Decode(op, provider, alias, raw)
}
