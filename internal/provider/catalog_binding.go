package provider

import (
	"fmt"
	"github.com/sunqirui1987/xhub/internal/config"
	"slices"
	"strings"
)

// CatalogModels 返回目录 ID 到上游模型、执行传输白名单的独立副本。
// 参数无；返回编译时供应商注册的能力表，供公开目录及表单使用；不读取任意文件路径。
func CatalogModels() map[string]map[string][]string {
	mu.Lock()
	defer mu.Unlock()
	out := map[string]map[string][]string{}
	for id, models := range catalogModels {
		out[id] = map[string][]string{}
		for model, ids := range models {
			out[id][model] = slices.Clone(ids)
		}
	}
	return out
}

// ValidateCatalogBinding 校验供应商目录与模型的实现白名单，供保存和凭据水合调用。
// 参数 catalogID 来自可信连接，m 为部署；返回错误或 nil，无副作用。
// 目录仅约束已登记专用能力；普通目录模型可以显式选择通用协议，由 ValidateDeployment 校验连接类型和端点。
func ValidateCatalogBinding(catalogID string, m config.ModelEntry) error {
	catalogID = strings.TrimSpace(catalogID)
	if catalogID == "" {
		return nil
	}
	models, ok := CatalogModels()[catalogID]
	if !ok {
		// 中转商目录 ID 是连接元数据，不要求每个商家都实现独立执行器；能力仍由注册传输验证。
		return nil
	}
	model := strings.TrimPrefix(m.ParamString("model", ""), catalogID+"/")
	ids := models[model]
	id := SelectedTransport(m)
	for _, t := range Transports() {
		if t.ID == id && (t.CatalogID == "" || (t.CatalogID == catalogID && slices.Contains(ids, id))) {
			return nil
		}
	}
	return fmt.Errorf("供应商目录 %s 的模型 %s 不支持所选上游协议", catalogID, model)
}
