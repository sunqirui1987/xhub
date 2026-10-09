// Package catalog ships the route list, public field definitions, and model price map inside the binary so the process does not read those files from disk.
package catalog

import (
	_ "embed"
	"encoding/json"
	"sync"

	"github.com/sunqirui1987/xhub/internal/logx"
)

var embedOnce sync.Once

// Embedded returns one embedded JSON document. The first valid document is logged once. A document that is not JSON is logged as an error.
// 参数 name（string）：Embedded要查找或展示的名称。空串表示还没有命名；raw（[]byte）：原始文本或 JSON 字节。
// 返回 []byte（[]byte）：Embedded的原始字节。没有内容时长度为 0。
// 调用：catalog/classify.go、catalog/model_cost.go
// 测试：无直接单测
func Embedded(name string, raw []byte) []byte {
	if !json.Valid(raw) {
		logx.Error("catalog embed %s is not json bytes=%d", name, len(raw))
		return raw
	}
	embedOnce.Do(func() { logx.Debug("catalog embed %s bytes=%d", name, len(raw)) })
	return raw
}

//go:embed routes.json
var routesJSON []byte

//go:embed publicdata/autorouter_presets.json
var autoRouterPresetsJSON []byte

//go:embed publicdata/pricedata.json
var pricedataJSON []byte

// providerFieldsJSON 保存 LiteLLM 的供应商认证字段快照，独立于模型价格发行方。
//
//go:embed publicdata/provider_fields.json
var providerFieldsJSON []byte
