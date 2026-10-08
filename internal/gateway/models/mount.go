// Package models registers model routes. The list, writes, and price-map source are mounted here.
package models

import (
	"net/http"

	"github.com/sunqirui1987/xhub/internal/httpx"
	"github.com/sunqirui1987/xhub/internal/logx"
	"sync"
)

var logTraceOnceMount sync.Once

// Module is the model list and the management writes. The process implements Host.
// 参数 h（Host）：实现这一步所需能力的数据面宿主。聊天、直通和刷写各自只依赖自己的方法。
// 返回 httpx.Module（httpx.Module）：可挂到网关上的模块。
// 调用：gateway/family/mount.go、gateway/guard/mount.go、gateway/identity/mount.go、gateway/keys/mount.go
// 测试：无直接单测
func Module(h Host) httpx.Module {
	logTraceOnceMount.Do(func() { logx.Trace("enter models.Module") })

	traceModule("models")
	return httpx.Bind("models", func(reg httpx.Registrar) {
		reg.Handle("GET /v1/models", func(w http.ResponseWriter, r *http.Request) { List(h, w, r) })
		reg.Handle("GET /models", func(w http.ResponseWriter, r *http.Request) { List(h, w, r) })
		reg.Handle("GET /model/available", func(w http.ResponseWriter, r *http.Request) { Available(h, w, r) })
		// The console's Models and Endpoints page reads this and nothing else.
		// Without it the page fell through to the catalog's generic store and
		// rendered an empty table.
		reg.Handle("GET /v2/model/info", func(w http.ResponseWriter, r *http.Request) { Info(h, w, r) })
		reg.Handle("GET /model/cost_map/source", func(w http.ResponseWriter, r *http.Request) { CostMapSource(h, w, r) })
		reg.Handle("POST /reload/model_cost_map", func(w http.ResponseWriter, r *http.Request) { ReloadCostMap(h, w, r) })
		reg.Handle("POST /schedule/model_cost_map_reload", func(w http.ResponseWriter, r *http.Request) { ScheduleCostMapReload(h, w, r) })
		reg.Handle("DELETE /schedule/model_cost_map_reload", func(w http.ResponseWriter, r *http.Request) { CancelCostMapReload(h, w, r) })
		reg.Handle("GET /schedule/model_cost_map_reload/status", func(w http.ResponseWriter, r *http.Request) { CostMapReloadStatus(h, w, r) })
		// The price catalog: the embedded Modelink baseline plus whatever an
		// operator adds or edits from the console.
		reg.Handle("GET /price/catalog", func(w http.ResponseWriter, r *http.Request) { PriceList(h, w, r) })
		reg.Handle("POST /price/model", func(w http.ResponseWriter, r *http.Request) { UpsertPriceModel(h, w, r) })
		reg.Handle("DELETE /price/model", func(w http.ResponseWriter, r *http.Request) { DeletePriceModel(h, w, r) })
		reg.Handle("POST /price/model/reset", func(w http.ResponseWriter, r *http.Request) { ResetPriceModel(h, w, r) })
		reg.Handle("POST /price/provider", func(w http.ResponseWriter, r *http.Request) { UpsertPriceProvider(h, w, r) })
		reg.Handle("DELETE /price/provider", func(w http.ResponseWriter, r *http.Request) { DeletePriceProvider(h, w, r) })
		reg.Handle("POST /model/new", func(w http.ResponseWriter, r *http.Request) { New(h, w, r) })
		reg.Handle("POST /model/builtin/refresh", func(w http.ResponseWriter, r *http.Request) { RefreshBuiltin(h, w, r) })
		reg.Handle("POST /model/builtin/models", func(w http.ResponseWriter, r *http.Request) { ListBuiltin(h, w, r) })
		reg.Handle("POST /model/builtin/add", func(w http.ResponseWriter, r *http.Request) { AddBuiltinModels(h, w, r) })
		reg.Handle("POST /model/update", func(w http.ResponseWriter, r *http.Request) { Update(h, w, r) })
		reg.Handle("PATCH /model/{model_id}/update", func(w http.ResponseWriter, r *http.Request) { Update(h, w, r) })
		reg.Handle("POST /model/delete", func(w http.ResponseWriter, r *http.Request) { Delete(h, w, r) })
		reg.Handle("POST /model/disable", func(w http.ResponseWriter, r *http.Request) { Disable(h, w, r) })
		reg.Handle("POST /model/enable", func(w http.ResponseWriter, r *http.Request) { Enable(h, w, r) })
		reg.Handle("GET /model_group/info", func(w http.ResponseWriter, r *http.Request) { GroupInfo(h, w, r) })
	})
}
