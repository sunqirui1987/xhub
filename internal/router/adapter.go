// 选中部署之后，把操作和供应商交给 llm 编码或解码。本文件不挑选部署。
package router

import "github.com/sunqirui1987/xhub/internal/llm"

// EncodeRequest 把对外 JSON 收成上游请求体。
// 具体协议在 internal/llm。这里只保留原来的函数名，避免数据面和测试各写一份。
func EncodeRequest(op, provider string, body map[string]any, realModel string) ([]byte, error) {
	return llm.Encode(op, provider, body, realModel)
}

// DecodeResponse 把上游响应收成对外形状，并把 model 改回调用别名。
func DecodeResponse(op, provider, alias string, raw []byte) []byte {
	return llm.Decode(op, provider, alias, raw)
}
