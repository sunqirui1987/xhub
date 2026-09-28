// Package catalog ships the route list, public field definitions, and model price map inside the binary so the process does not read those files from disk.
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
