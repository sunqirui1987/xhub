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

//go:embed publicdata/agent_create_fields.json
var agentFieldsJSON []byte

//go:embed publicdata/provider_create_fields.json
var providerFieldsJSON []byte

//go:embed publicdata/autorouter_presets.json
var autoRouterPresetsJSON []byte

//go:embed publicdata/model_cost_map.json
var modelCostMapJSON []byte
