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
func Module(h Host) httpx.Module {
	logTraceOnceMount.Do(func() { logx.Trace("enter models.Module") })

	traceModule("models")
	return httpx.Bind("models", func(reg httpx.Registrar) {
		reg.Handle("GET /v1/models", func(w http.ResponseWriter, r *http.Request) { List(h, w, r) })
		reg.Handle("GET /models", func(w http.ResponseWriter, r *http.Request) { List(h, w, r) })
		reg.Handle("GET /model/cost_map/source", func(w http.ResponseWriter, r *http.Request) { CostMapSource(h, w, r) })
		reg.Handle("POST /reload/model_cost_map", func(w http.ResponseWriter, r *http.Request) { ReloadCostMap(h, w, r) })
		reg.Handle("POST /schedule/model_cost_map_reload", func(w http.ResponseWriter, r *http.Request) { ScheduleCostMapReload(h, w, r) })
		reg.Handle("DELETE /schedule/model_cost_map_reload", func(w http.ResponseWriter, r *http.Request) { CancelCostMapReload(h, w, r) })
		reg.Handle("GET /schedule/model_cost_map_reload/status", func(w http.ResponseWriter, r *http.Request) { CostMapReloadStatus(h, w, r) })
		reg.Handle("POST /model/new", func(w http.ResponseWriter, r *http.Request) { New(h, w, r) })
		reg.Handle("POST /model/update", func(w http.ResponseWriter, r *http.Request) { Update(h, w, r) })
		reg.Handle("PATCH /model/{model_id}/update", func(w http.ResponseWriter, r *http.Request) { Update(h, w, r) })
		reg.Handle("POST /model/delete", func(w http.ResponseWriter, r *http.Request) { Delete(h, w, r) })
		reg.Handle("POST /model/block", func(w http.ResponseWriter, r *http.Request) { Block(h, w, r) })
		reg.Handle("POST /model/unblock", func(w http.ResponseWriter, r *http.Request) { Unblock(h, w, r) })
		reg.Handle("GET /model_group/info", func(w http.ResponseWriter, r *http.Request) { GroupInfo(h, w, r) })
	})
}
