// Package router orders deployments that share one public model name. After a deployment is chosen, encoding and decoding are delegated to the llm package.
package router

import "github.com/sunqirui1987/xhub/internal/llm"

// EncodeRequest turns the public JSON body into the upstream request body. The protocol details live in internal/llm. This wrapper keeps the old name so callers do not each import that package.
func EncodeRequest(op, provider string, body map[string]any, realModel string) ([]byte, error) {
	return llm.Encode(op, provider, body, realModel)
}

// DecodeResponse turns an upstream response into the public shape and puts the caller's model alias back into the model field.
func DecodeResponse(op, provider, alias string, raw []byte) []byte {
	return llm.Decode(op, provider, alias, raw)
}
