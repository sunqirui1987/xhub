package catalog

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/sunqirui1987/xhub/internal/logx"
)

// The embedded catalog is the baseline. An operator's hand-entered rows are
// stored in the database and laid over it, so editing a model or adding a
// supplier never rewrites the generated file. Clearing an override restores the
// baseline row, which is why the baseline is kept separately from the live map.
var baselineModels map[string]map[string]any

// baselineProviders is the embedded supplier list, before any hand-added entry.
var baselineProviders []map[string]any

// takeBaseline snapshots the freshly loaded document. loadPriceDocument calls it
// once; everything after that is an override on top.
// 参数：无。
// 返回：无。快照留在 baselineModels 和 baselineProviders，之后的修改盖在它上面。
// 调用：loadPriceDocument，只在目录载入之后调用一次。
// 测试：无直接单测
func takeBaseline() {
	if modelCostMapValue == nil {
		return
	}
	raw, ok := modelCostMapValue.(map[string]any)
	if !ok {
		return
	}
	baselineModels = make(map[string]map[string]any, len(raw))
	for id, v := range raw {
		if row, ok := v.(map[string]any); ok {
			baselineModels[id] = row
		}
	}
	baselineProviders = make([]map[string]any, len(baseProviders))
	copy(baselineProviders, baseProviders)
}

// BaselineModel returns the embedded row for an id, or ok false when the
// generated catalog does not contain that model.
// 参数 id（string）：价格表里的模型键。
// 返回 map[string]any（map[string]any）：内嵌价格表里这一行；bool（bool）：内嵌表里有这个模型时为真。
// 调用：清除覆盖时恢复内嵌行。
// 测试：无直接单测
func BaselineModel(id string) (map[string]any, bool) {
	row, ok := baselineModels[strings.TrimSpace(id)]
	return row, ok
}

// ModelRow returns the live row for an id, which is the baseline row or the
// override that replaced it. A missing id returns ok false.
// 参数 id（string）：价格表里的模型键。
// 返回 map[string]any（map[string]any）：当前生效的这一行；bool（bool）：价格表里有这个模型时为真。
// 调用：控制台保存前先把原有字段读出来。
// 测试：无直接单测
func ModelRow(id string) (map[string]any, bool) {
	modelCostMu.RLock()
	defer modelCostMu.RUnlock()
	raw, ok := modelCostMapValue.(map[string]any)
	if !ok {
		return nil, false
	}
	row, ok := raw[strings.TrimSpace(id)].(map[string]any)
	return row, ok
}

// IsBaseline reports whether this id comes from the generated catalog rather
// than from an operator's entry.
// 参数 id（string）：价格表里的模型键。
// 返回 bool（bool）：这一行来自内嵌价格表时为真。
// 调用：判断删除覆盖后是否还能回到内嵌行。
// 测试：无直接单测
func IsBaseline(id string) bool {
	_, ok := BaselineModel(id)
	return ok
}

// SetModel writes one row into the live price map and indexes it under its
// provider. An existing row is replaced, so this is also how an edit lands.
// 参数 id（string）：价格表里的模型键；row（map[string]any）：这一行的字段。
// 返回：无。只改内存里的价格表。
// 调用：控制台保存或修改一条模型价格时。
// 测试：无直接单测
func SetModel(id string, row map[string]any) {
	id = strings.TrimSpace(id)
	if id == "" || row == nil {
		return
	}
	modelCostMu.Lock()
	defer modelCostMu.Unlock()

	raw, _ := modelCostMapValue.(map[string]any)
	if raw == nil {
		raw = map[string]any{}
		modelCostMapValue = raw
	}
	raw[id] = row
	indexLocked(id, stringField(row, "litellm_provider"))
	for _, alias := range stringListField(row, "model_alias") {
		if alias != "" && alias != id {
			officialAlias[alias] = id
		}
	}
}

// RemoveModel drops one row from the live map and its provider index. The
// embedded baseline is untouched, so the row returns after a restart unless the
// caller also deleted the stored override.
// 参数 id（string）：价格表里的模型键。
// 返回 bool（bool）：确实删掉了这一行时为真。
// 调用：控制台删除一条模型价格时。内嵌基线不动。
// 测试：无直接单测
func RemoveModel(id string) bool {
	id = strings.TrimSpace(id)
	if id == "" {
		return false
	}
	modelCostMu.Lock()
	defer modelCostMu.Unlock()

	raw, _ := modelCostMapValue.(map[string]any)
	if raw == nil {
		return false
	}
	if _, ok := raw[id]; !ok {
		return false
	}
	delete(raw, id)
	for provider, ids := range modelsByProvider {
		if filtered := without(ids, id); len(filtered) != len(ids) {
			modelsByProvider[provider] = filtered
		}
	}
	for alias, target := range officialAlias {
		if target == id || alias == id {
			delete(officialAlias, alias)
		}
	}
	return true
}

// ApplyDocument replaces the live catalog with a freshly generated one. The
// embedded baseline is untouched, so an operator's overrides and deletions still
// apply on top of it and a later restart falls back to the same rows.
//
// A document with no models is refused: a failed fetch must not empty the
// catalog the gateway is billing from.
// 参数 doc（PriceDocument）：刚生成的价格目录。
// 返回 int（int）：换上的模型条数；error（error）：目录为空或不可用时不为 nil，此时价格表不动。
// 调用：ReloadFromMarket。
// 测试：pricedata_test.go
func ApplyDocument(doc PriceDocument) (int, error) {
	if len(doc.Models) == 0 {
		logx.Error("refusing to apply an empty price catalog")
		return 0, fmt.Errorf("refusing to apply an empty price catalog")
	}
	rows := make(map[string]any, len(doc.Models))
	sets := map[string][]string{}
	for id, row := range doc.Models {
		if row == nil {
			continue
		}
		copied := map[string]any(row)
		rows[id] = copied
		if provider := stringField(copied, "litellm_provider"); provider != "" {
			sets[provider] = append(sets[provider], id)
		}
	}
	for provider := range sets {
		sort.Strings(sets[provider])
	}
	if len(rows) == 0 {
		return 0, fmt.Errorf("refusing to apply a price catalog with no usable rows")
	}

	modelCostMu.Lock()
	modelCostMapValue = rows
	modelsByProvider = sets
	refreshContributionsLocked()
	if doc.Source != "" {
		priceSource = doc.Source
	}
	if doc.GeneratedAt != "" {
		priceGeneratedAt = doc.GeneratedAt
	}
	modelCostMapLoadedAt = time.Now().UTC().Format(time.RFC3339)
	modelCostMu.Unlock()

	// The baseline is what the console compares against, so it moves with the
	// refreshed feed rather than staying at the version compiled into the binary.
	adoptBaseline()
	return len(rows), nil
}

// adoptBaseline 在刷新之后重新给当前价格行拍照。「恢复内置价格」因此恢复的是网关正在使用的价格，而不是编译进二进制的旧快照。
// 参数：无。
// 返回：无。baselineModels 换成当前价格表里的对象行。当前表不是对象时不改快照。
// 调用：ApplyDocument 在换上新目录之后。
// 测试：无直接单测
func adoptBaseline() {
	modelCostMu.Lock()
	defer modelCostMu.Unlock()
	raw, ok := modelCostMapValue.(map[string]any)
	if !ok {
		return
	}
	next := make(map[string]map[string]any, len(raw))
	for id, v := range raw {
		if row, ok := v.(map[string]any); ok {
			next[id] = row
		}
	}
	baselineModels = next
}

// SetProvider writes one supplier into the add-model dropdown, replacing an
// entry that already uses the same slug.
// 参数 slug（string）：供应商标识；row（map[string]any）：这一项的字段。
// 返回：无。只改内存里的下拉列表。
// 调用：控制台新增或修改供应商时。
// 测试：无直接单测
func SetProvider(slug string, row map[string]any) {
	slug = strings.TrimSpace(slug)
	if slug == "" || row == nil {
		return
	}
	modelCostMu.Lock()
	defer modelCostMu.Unlock()
	for i, existing := range baseProviders {
		if stringField(existing, "litellm_provider") == slug {
			baseProviders[i] = row
			knownLLMProviders[slug] = struct{}{}
			return
		}
	}
	baseProviders = append(baseProviders, row)
	knownLLMProviders[slug] = struct{}{}
}

// RemoveProvider drops one supplier from the dropdown.
// 参数 slug（string）：供应商标识。
// 返回 bool（bool）：确实删掉了这一项时为真。
// 调用：控制台从添加模型的下拉列表去掉一个供应商时。
// 测试：无直接单测
func RemoveProvider(slug string) bool {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return false
	}
	modelCostMu.Lock()
	defer modelCostMu.Unlock()
	out := make([]map[string]any, 0, len(baseProviders))
	removed := false
	for _, row := range baseProviders {
		if stringField(row, "litellm_provider") == slug {
			removed = true
			continue
		}
		out = append(out, row)
	}
	baseProviders = out
	return removed
}

// IsBaselineProvider reports whether this supplier comes from the generated
// catalog rather than from an operator's entry.
// 参数 slug（string）：供应商标识。
// 返回 bool（bool）：这一项来自内嵌价格表时为真。
// 调用：判断删掉覆盖后供应商是否还会从内嵌目录回来。
// 测试：无直接单测
func IsBaselineProvider(slug string) bool {
	slug = strings.TrimSpace(slug)
	for _, row := range baselineProviders {
		if stringField(row, "litellm_provider") == slug {
			return true
		}
	}
	return false
}

// indexLocked adds an id to its provider's set. The caller holds the write lock.
// 参数 id（string）：价格表里的模型键；provider（string）：供应商 slug。空串时不建索引。
// 返回：无。只改 modelsByProvider 和 knownLLMProviders。
// 调用：SetModel，调用方已经持有写锁。
// 测试：无直接单测
func indexLocked(id, provider string) {
	if provider == "" {
		return
	}
	if modelsByProvider == nil {
		modelsByProvider = map[string][]string{}
	}
	modelsByProvider[provider] = appendUnique(modelsByProvider[provider], id)
	sort.Strings(modelsByProvider[provider])
	knownLLMProviders[provider] = struct{}{}
}

// without returns ids with one value removed.
// 参数 ids（[]string）：原列表，不修改它；drop（string）：要去掉的那个 id。
// 返回 []string（[]string）：去掉 drop 之后的新切片。没有匹配时长度与入参相同。
// 调用：RemoveModel 从供应商索引里摘掉模型。
// 测试：无直接单测
func without(ids []string, drop string) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if id != drop {
			out = append(out, id)
		}
	}
	return out
}
