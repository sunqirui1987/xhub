// Package models reloads the price map immediately or on a timer. The dashboard reads status, models_count, and scheduled.
package models

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/sunqirui1987/xhub/internal/catalog"
	"github.com/sunqirui1987/xhub/internal/httpx"
	"github.com/sunqirui1987/xhub/internal/logx"
	"sync"
)

const (
	costReloadNS  = "model_cost_map"
	costReloadKey = "reload"
)

type costReloadPlan struct {
	Scheduled     bool
	IntervalHours *int
	LastRun       *string
	NextRun       *string
}

var logTraceOnceCostReload sync.Once

// ReloadCostMap refetches the price catalog from the market feed now. On success
// it returns status and the model count. A failed fetch leaves the prices in use
// untouched and answers 502, so the console can say the reload did not happen
// instead of implying fresh prices arrived.
// 参数 s（Host）：Reload费用表使用的数据面宿主；w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；r（*http.Request）：入站 HTTP 请求。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：gateway/models/mount.go
// 测试：无直接单测
func ReloadCostMap(s Host, w http.ResponseWriter, r *http.Request) {
	logTraceOnceCostReload.Do(func() { logx.Trace("enter models.ReloadCostMap") })

	if s.RequireManage(w, r) == nil {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	n, err := refreshLocalCatalog(ctx, s)
	if err != nil {
		logx.Error("price catalog reload failed: %v", err)
		httpx.WriteError(w, 502, "upstream_error", err.Error())
		return
	}
	plan := loadCostReload(s)
	stamp := time.Now().UTC().Format(time.RFC3339)
	plan.LastRun = &stamp
	if plan.Scheduled && plan.IntervalHours != nil {
		next := time.Now().UTC().Add(time.Duration(*plan.IntervalHours) * time.Hour).Format(time.RFC3339)
		plan.NextRun = &next
	}
	if err := saveCostReload(s, plan); err != nil {
		httpx.WriteError(w, 500, "internal", err.Error())
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{
		"status":       "success",
		"models_count": n,
		"source":       catalog.PriceSource(),
	})
}

// ScheduleCostMapReload arms a reload every given number of hours. The hour count must be an integer from 1 to 168.
// 参数 s（Host）：Schedule费用表Reload使用的数据面宿主；w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；r（*http.Request）：入站 HTTP 请求。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：gateway/models/mount.go
// 测试：无直接单测
func ScheduleCostMapReload(s Host, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	hours, err := strconv.Atoi(r.URL.Query().Get("hours"))
	if err != nil || hours < 1 || hours > 168 {
		httpx.WriteError(w, 400, "invalid_request", "hours must be a whole number between 1 and 168")
		return
	}
	plan := loadCostReload(s)
	plan.Scheduled = true
	plan.IntervalHours = &hours
	next := time.Now().UTC().Add(time.Duration(hours) * time.Hour).Format(time.RFC3339)
	plan.NextRun = &next
	if err := saveCostReload(s, plan); err != nil {
		httpx.WriteError(w, 500, "internal", err.Error())
		return
	}
	StartScheduledReload(s)
	httpx.WriteJSON(w, 200, map[string]any{
		"status":         "success",
		"interval_hours": hours,
		"next_run":       next,
	})
}

// CancelCostMapReload turns the timer off. The last-run time is kept.
// 参数 s（Host）：取消费用表Reload使用的数据面宿主；w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；r（*http.Request）：入站 HTTP 请求。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：gateway/models/mount.go
// 测试：无直接单测
func CancelCostMapReload(s Host, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	plan := loadCostReload(s)
	plan.Scheduled = false
	plan.IntervalHours = nil
	plan.NextRun = nil
	if err := saveCostReload(s, plan); err != nil {
		httpx.WriteError(w, 500, "internal", err.Error())
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"status": "success"})
}

// CostMapReloadStatus reports whether the timer is on, its interval, and the last and next run times.
// 参数 s（Host）：费用表Reload状态使用的数据面宿主；w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；r（*http.Request）：入站 HTTP 请求。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：gateway/models/mount.go
// 测试：无直接单测
func CostMapReloadStatus(s Host, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	httpx.WriteJSON(w, 200, loadCostReload(s).public())
}

// loadCostReload reads the saved timer. A missing record is treated as off.
// 参数 s（Host）：载入费用Reload使用的数据面宿主。
// 返回 costReloadPlan（costReloadPlan）：已保存的价格重载计时。没有记录或没有库时是关闭的零值。
// 调用：仅在 cost_reload.go 内使用
// 测试：无直接单测
func loadCostReload(s Host) costReloadPlan {
	if s.RecordStore() == nil {
		return costReloadPlan{}
	}
	cfg, err := s.RecordStore().ListConfig(costReloadNS)
	if err != nil {
		return costReloadPlan{}
	}
	raw, _ := cfg[costReloadKey].(map[string]any)
	if raw == nil {
		return costReloadPlan{}
	}
	plan := costReloadPlan{}
	if v, ok := raw["scheduled"].(bool); ok {
		plan.Scheduled = v
	}
	if v, ok := raw["interval_hours"].(float64); ok {
		n := int(v)
		plan.IntervalHours = &n
	}
	if v, ok := raw["last_run"].(string); ok && v != "" {
		plan.LastRun = &v
	}
	if v, ok := raw["next_run"].(string); ok && v != "" {
		plan.NextRun = &v
	}
	return plan
}

// saveCostReload stores the timer. The next status read uses this record.
// 参数 s（Host）：保存费用Reload使用的数据面宿主；plan（costReloadPlan）：保存费用Reload使用的costReloadPlan。
// 返回 error（error）：失败原因，nil 表示这一步成功。
// 调用：仅在 cost_reload.go 内使用
// 测试：无直接单测
func saveCostReload(s Host, plan costReloadPlan) error {
	if s.RecordStore() == nil {
		return nil
	}
	return s.RecordStore().PutConfig(costReloadNS, costReloadKey, plan.public())
}

// public is the JSON the dashboard reads for a cost reload plan. An unset time is null.
// 参数：无。
// 返回 map[string]any（map[string]any）：公开的字段表。缺键表示上游或库里没有这个字段。
// 调用：仅在 cost_reload.go 内使用
// 测试：无直接单测
func (p costReloadPlan) public() map[string]any {
	var hours, last, next any
	if p.IntervalHours != nil {
		hours = *p.IntervalHours
	}
	if p.LastRun != nil {
		last = *p.LastRun
	}
	if p.NextRun != nil {
		next = *p.NextRun
	}
	return map[string]any{
		"scheduled":      p.Scheduled,
		"interval_hours": hours,
		"last_run":       last,
		"next_run":       next,
	}
}
