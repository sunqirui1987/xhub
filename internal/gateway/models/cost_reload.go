// Package models reloads the price map immediately or on a timer. The dashboard reads status, models_count, and scheduled.
package models

import (
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

// ReloadCostMap reloads the price map now. On success it returns status and the model count.
func ReloadCostMap(s Host, w http.ResponseWriter, r *http.Request) {
	logTraceOnceCostReload.Do(func() { logx.Trace("enter models.ReloadCostMap") })

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

// ScheduleCostMapReload arms a reload every given number of hours. The hour count must be an integer from 1 to 168.
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

// CancelCostMapReload turns the timer off. The last-run time is kept.
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
func CostMapReloadStatus(s Host, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	httpx.WriteJSON(w, 200, loadCostReload(s).public())
}

// loadCostReload reads the saved timer. A missing record is treated as off.
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

// saveCostReload stores the timer. The next status read uses this record.
func saveCostReload(s Host, plan costReloadPlan) error {
	if s.DB() == nil {
		return nil
	}
	return s.DB().PutConfig(costReloadNS, costReloadKey, plan.public())
}

// public is the JSON the dashboard reads for a cost reload plan. An unset time is null.
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
