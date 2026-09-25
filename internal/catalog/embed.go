// 内置的路由清单、公开字段和模型价格表。这些 JSON 随二进制发布，不读磁盘。
package catalog

import _ "embed"

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
