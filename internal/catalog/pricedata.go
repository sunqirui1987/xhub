package catalog

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/sunqirui1987/xhub/internal/logx"
)

// PriceDocument is the embedded price catalog. It is generated from the
// Modelink market feed by ./cmd/pricedata and embedded in the binary.
//
// Models and Providers are the two halves the console needs: a price row per
// model, and the supplier list an operator picks from when adding a model.
type PriceDocument struct {
	Version     int                       `json:"version"`
	Source      string                    `json:"source"`
	GeneratedAt string                    `json:"generated_at"`
	Providers   []map[string]any          `json:"providers"`
	Models      map[string]map[string]any `json:"models"`
}

// loadPriceDocument 解析内嵌价格目录并建好查找索引。解析失败时留下空目录，进程仍然启动。
// 参数：无。
// 返回：无。结果写进本文件的价格表、别名和供应商集合。
// 调用：catalog 在进程启动时。
// 测试：无直接单测
func loadPriceDocument() {
	var doc PriceDocument
	if err := json.Unmarshal(Embedded("pricedata", pricedataJSON), &doc); err != nil {
		logx.Error("catalog pricedata is not readable: %v", err)
		doc = PriceDocument{}
	}
	if doc.Models == nil {
		doc.Models = map[string]map[string]any{}
	}
	priceSource = doc.Source
	priceGeneratedAt = doc.GeneratedAt

	sets := map[string][]string{}
	rows := make(map[string]any, len(doc.Models))
	for id, row := range doc.Models {
		if row == nil {
			continue
		}
		rows[id] = map[string]any(row)
		provider := strings.TrimSpace(stringField(row, "litellm_provider"))
		if provider != "" {
			sets[provider] = append(sets[provider], id)
		}
		// The feed lists the provider's other names for the same model. A bill
		// for an alias must find the same rates as the canonical id.
		for _, alias := range stringListField(row, "model_alias") {
			if alias != id {
				officialAlias[alias] = id
			}
		}
	}
	for provider := range sets {
		sort.Strings(sets[provider])
		knownLLMProviders[provider] = struct{}{}
	}
	modelCostMapValue = rows
	modelsByProvider = sets
	baseProviders = doc.Providers
	takeBaseline()
}

// Providers 返回内嵌目录里的供应商列表，供添加模型时的下拉框使用，含凭据字段。
// 参数：无。
// 返回 []map[string]any（[]map[string]any）：供应商定义。目录还没载入时为空切片。
// 调用：添加模型的供应商列表。
// 测试：无直接单测
func Providers() []map[string]any {
	return baseProviders
}

// PriceSource 返回生成这份内嵌目录时用的来源地址。
// 参数：无。
// 返回 string（string）：来源 URL。目录没有写来源时为空串。
// 调用：价格目录的调试信息。
// 测试：无直接单测
func PriceSource() string { return priceSource }

// PriceGeneratedAt 返回这份内嵌目录的生成时间。
// 参数：无。
// 返回 string（string）：生成时间字符串。没有写时为空串。
// 调用：价格目录的调试信息。
// 测试：无直接单测
func PriceGeneratedAt() string { return priceGeneratedAt }

// stringField 从价格行里读一个去掉空白的字符串。
// 参数 row（map[string]any）：一行价格字段；key（string）：字段名，例如 litellm_provider。
// 返回 string（string）：去掉空白后的文本。缺键或不是字符串时为空串。
// 调用：loadPriceDocument。
// 测试：无直接单测
func stringField(row map[string]any, key string) string {
	s, _ := row[key].(string)
	return strings.TrimSpace(s)
}

// stringListField 从价格行里读一组字符串，跳过空白项。
// 参数 row（map[string]any）：一行价格字段；key（string）：字段名，例如 model_alias。
// 返回 []string（[]string）：去掉空白后的字符串。缺键或类型不对时为 nil。
// 调用：loadPriceDocument，用来登记模型别名。
// 测试：无直接单测
func stringListField(row map[string]any, key string) []string {
	list, ok := row[key].([]any)
	if !ok {
		if typed, ok := row[key].([]string); ok {
			return typed
		}
		return nil
	}
	out := make([]string, 0, len(list))
	for _, item := range list {
		if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
			out = append(out, strings.TrimSpace(s))
		}
	}
	return out
}

var (
	// baseProviders is the add-model dropdown the embedded catalog ships.
	baseProviders []map[string]any
	// priceSource and priceGeneratedAt say where the embedded catalog came from.
	priceSource      string
	priceGeneratedAt string
)
