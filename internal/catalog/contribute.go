package catalog

import (
	"strings"

	"github.com/sunqirui1987/xhub/internal/logx"
)

// Row is one model a provider package adds to the price map the console already
// serves. An ID that the embedded map already has is left unchanged.
type Row struct {
	ID           string
	Provider     string
	Mode         string
	EndpointType string
	Source       string
	Input        float64
	Output       float64
	Priced       bool
	Official     string
}

// ProviderField is one credential input appended for a provider the embedded
// field list does not already name.
type ProviderField struct {
	Key      string
	Label    string
	Type     string
	Required bool
	Default  string
}

// ProviderRow is one add-model dropdown entry.
type ProviderRow struct {
	Name        string
	Slug        string
	Display     string
	Placeholder string
	Fields      []ProviderField
}

var (
	extraProviders []ProviderRow
	officialAlias  = map[string]string{}
)

// Contribute inserts a model into the price map and the per-provider index. The official id is remembered so a bill for that id finds the same rates.
// 参数 row（Row）：从用量或目录读出的Row。
// 返回：无。这条模型已写入价格表和供应商索引，官方 id 也记了下来。id 或供应商为空时什么都不写。
// 调用：provider/registry.go
// 测试：无直接单测
func Contribute(row Row) {
	row.ID = strings.TrimSpace(row.ID)
	row.Provider = strings.TrimSpace(row.Provider)
	if row.ID == "" || row.Provider == "" {
		logx.Debug("catalog contribution skipped reason=missing id or provider")
		return
	}
	modelCostMu.Lock()
	defer modelCostMu.Unlock()
	raw, _ := modelCostMapValue.(map[string]any)
	if raw == nil {
		raw = map[string]any{}
		modelCostMapValue = raw
	}
	if _, exists := raw[row.ID]; !exists {
		entry := map[string]any{
			"litellm_provider": row.Provider,
			"mode":             row.Mode,
			"source":           row.Source,
		}
		if row.Priced {
			entry["input_cost_per_token"] = row.Input
			entry["output_cost_per_token"] = row.Output
		}
		if row.EndpointType != "" {
			entry["endpoint_type"] = row.EndpointType
		}
		raw[row.ID] = entry
		if modelsByProvider == nil {
			modelsByProvider = map[string][]string{}
		}
		modelsByProvider[row.Provider] = appendUnique(modelsByProvider[row.Provider], row.ID)
		knownLLMProviders[row.Provider] = struct{}{}
	}
	if official := strings.TrimSpace(row.Official); official != "" && official != row.ID {
		officialAlias[official] = row.ID
	}
}

// ContributeProvider remembers a dropdown entry. PublicBody appends it when the embedded list does not already use that provider name.
// 参数 p（ProviderRow）：Contribute供应商使用的ProviderRow。
// 返回：无。下拉列表多了一条供应商。名称或 slug 为空时不记。
// 调用：provider/registry.go
// 测试：无直接单测
func ContributeProvider(p ProviderRow) {
	if strings.TrimSpace(p.Name) == "" || strings.TrimSpace(p.Slug) == "" {
		return
	}
	modelCostMu.Lock()
	extraProviders = append(extraProviders, p)
	knownLLMProviders[p.Slug] = struct{}{}
	modelCostMu.Unlock()
}

// aliasKey resolves an official model id to the price-map key, when one was registered.
// 参数 model（string）：对外模型名，用来选部署和记用量。
// 返回 string（string）：价格表使用的模型键。没有登记别名时退回原始 id；bool（bool）：这个官方模型 id 在价格表里登记过别名时返回真。
// 调用：仅在 contribute.go 内使用。
// 测试：无直接单测
func aliasKey(model string) (string, bool) {
	modelCostMu.RLock()
	defer modelCostMu.RUnlock()
	return aliasKeyLocked(model)
}

// aliasKeyLocked is aliasKey for a caller that already holds the read lock.
// 参数 model（string）：对外模型名，用来选部署和记用量。
// 返回 string（string）：价格表使用的模型键；bool（bool）：登记过别名时为真。
// 调用：catalog/model_cost.go
// 测试：无直接单测
func aliasKeyLocked(model string) (string, bool) {
	id, ok := officialAlias[model]
	return id, ok
}

// 把 id 加进列表。已经存在时返回原切片，不重复添加。
// 参数 ids（[]string）：要保留或查询的一组 id。空切片表示没有可处理的记录；id（string）：追加去重使用的主键。空串表示调用方没有指定记录。
// 返回 []string（[]string）：追加去重。没有匹配时为空切片。
// 调用：仅在 contribute.go 内使用
// 测试：无直接单测
func appendUnique(ids []string, id string) []string {
	for _, have := range ids {
		if have == id {
			return ids
		}
	}
	return append(ids, id)
}

// mergeProviders appends provider packages that the embedded dropdown does not already name.
// 参数 base（any）：合并Providers接到的动态值。类型在函数体内收窄。
// 返回 any（any）：合并Providers的结果。具体类型由调用方断言。
// 调用：catalog/classify.go
// 测试：无直接单测
func mergeProviders(base any) any {
	list, ok := base.([]any)
	if !ok {
		return base
	}
	seen := map[string]bool{}
	for _, item := range list {
		row, _ := item.(map[string]any)
		name, _ := row["provider"].(string)
		if name != "" {
			seen[name] = true
		}
	}
	for _, item := range extraProviderMaps() {
		row, _ := item.(map[string]any)
		name, _ := row["provider"].(string)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		list = append(list, item)
	}
	return list
}

// 把额外登记的供应商字段收成价目表要的结构。
// 参数：无。
// 返回 []any（[]any）：把额外登记的供应商字段收成价目表要的结构。没有行时为空切片。
// 调用：仅在 contribute.go 内使用
// 测试：无直接单测
func extraProviderMaps() []any {
	modelCostMu.RLock()
	defer modelCostMu.RUnlock()
	out := make([]any, 0, len(extraProviders))
	for _, p := range extraProviders {
		fields := make([]any, 0, len(p.Fields))
		for _, f := range p.Fields {
			kind := f.Type
			if kind == "" {
				kind = "text"
			}
			fields = append(fields, map[string]any{
				"key":           f.Key,
				"label":         f.Label,
				"placeholder":   nil,
				"tooltip":       nil,
				"required":      f.Required,
				"field_type":    kind,
				"options":       nil,
				"default_value": emptyAsNil(f.Default),
			})
		}
		out = append(out, map[string]any{
			"provider":                  p.Name,
			"provider_display_name":     p.Display,
			"litellm_provider":          p.Slug,
			"credential_fields":         fields,
			"default_model_placeholder": p.Placeholder,
		})
	}
	return out
}

// 空字符串改成 nil，非空则原样返回，避免写出空的 JSON 字符串。
// 参数 s（string）：可能为空的文本。空串要变成 nil，避免把空值写成 JSON 字符串。
// 返回 any（any）：非空时是原字符串。空串返回 nil，这样 JSON 里是 null 而不是空字符串。
// 调用：仅在 contribute.go 内使用
// 测试：无直接单测
func emptyAsNil(s string) any {
	if s == "" {
		return nil
	}
	return s
}
