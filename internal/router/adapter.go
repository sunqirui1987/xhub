// Package router orders deployments that share one public model name. After a deployment is chosen, encoding and decoding are delegated to the llm package.
package router

import (
	"github.com/sunqirui1987/xhub/internal/llm"
	"github.com/sunqirui1987/xhub/internal/logx"
	"sync"
)

var logTraceOnceAdapter sync.Once

// EncodeRequest turns the public JSON body into the upstream request body. The protocol details live in internal/llm. This wrapper keeps the old name so callers do not each import that package.
// 参数 op（string）：操作名，例如 chat；provider（string）：供应商标识，例如 openai 或 volcengine；body（map[string]any）：已解析或原始的 JSON；realModel（string）：发给上游或对外展示的模型名。
// 返回 []byte（[]byte）：编码请求的原始字节。没有内容时长度为 0；error（error）：失败原因。nil 表示这一步成功。
// 调用：仅在 adapter.go 内使用
// 测试：无直接单测
func EncodeRequest(op, provider string, body map[string]any, realModel string) ([]byte, error) {
	logTraceOnceAdapter.Do(func() { logx.Trace("enter router.EncodeRequest") })

	return llm.Encode(op, provider, body, realModel)
}

// DecodeResponse turns an upstream response into the public shape and puts the caller's model alias back into the model field.
// 参数 op（string）：操作名，例如 chat；provider（string）：供应商标识，例如 openai 或 volcengine；alias（string）：对外模型名；raw（[]byte）：原始文本或 JSON 字节。
// 返回 []byte（[]byte）：解码响应的原始字节。没有内容时长度为 0。
// 调用：gateway/spend.go
// 测试：无直接单测
func DecodeResponse(op, provider, alias string, raw []byte) []byte {
	return llm.Decode(op, provider, alias, raw)
}
