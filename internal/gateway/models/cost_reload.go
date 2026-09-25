// 价格表的立即重载和定时重载。控制台只认 status、models_count 和 scheduled 这几个字段。
package models

import (
	"net/http"
	"strconv"
	"time"

	"github.com/sunqirui1987/xhub/internal/catalog"
	"github.com/sunqirui1987/xhub/internal/httpx"
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

// 立即重新加载价格表。成功时返回 status 和模型条数。
func ReloadCostMap(s Host, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	n := catalog.MarkReloaded()
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
	})
}

// 按小时设置定时重载。小时必须是 1 到 168 的整数。
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
	httpx.WriteJSON(w, 200, map[string]any{
		"status":         "success",
		"interval_hours": hours,
		"next_run":       next,
	})
}

// 取消定时重载。上次运行时间保留。
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

// 返回定时重载是否开启、间隔、上次和下次时间。
func CostMapReloadStatus(s Host, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	httpx.WriteJSON(w, 200, loadCostReload(s).public())
}

// 读出已保存的定时计划。没有记录时视为未开启。
func loadCostReload(s Host) costReloadPlan {
	if s.DB() == nil {
		return costReloadPlan{}
	}
	cfg, err := s.DB().ListConfig(costReloadNS)
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

// 保存定时计划。控制台下次读取状态时用这份记录。
func saveCostReload(s Host, plan costReloadPlan) error {
	if s.DB() == nil {
		return nil
	}
	return s.DB().PutConfig(costReloadNS, costReloadKey, plan.public())
}

// 转成控制台读取的 JSON。未设置的时间是 null。
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
