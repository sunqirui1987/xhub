// Package models refreshes the market price catalog on a timer when the console
// has armed one. A scheduled reload is the same call as the manual one: fetch
// the feed, and only swap the prices in if the fetch produced a usable catalog.
package models

import (
	"context"
	"time"

	"github.com/sunqirui1987/xhub/internal/catalog"
	"github.com/sunqirui1987/xhub/internal/logx"
)

// StartScheduledReload runs the armed reload plan. It returns immediately when
// no plan is armed, and otherwise re-reads the plan each cycle so a console
// change takes effect without a restart.
// 参数 s（Host）：能读框架记录的宿主。
// 返回：无。子 goroutine 在进程退出时随之结束。
// 调用：gateway.New。
// 测试：无直接单测
func StartScheduledReload(s Host) {
	plan := loadCostReload(s)
	if !plan.Scheduled || plan.IntervalHours == nil {
		return
	}
	go scheduledReloadLoop(s)
}

// reloadNow fetches the market feed and swaps it in. Both the manual reload and
// the scheduled one go through here, so they cannot drift apart.
// 参数 ctx（context.Context）：取消和超时。
// 返回 int（int）：换上的模型条数；error（error）：抓取或应用失败时不为 nil，此时价格表不动。
// 调用：ReloadCostMap、scheduledReloadLoop。
// 测试：无直接单测
func reloadNow(ctx context.Context) (int, error) {
	return catalog.ReloadFromMarket(ctx, catalog.MarketURL)
}

// scheduledReloadLoop 每分钟重读一次重载计划。计划里的下次运行时间到了就拉市场目录并换上。按分钟轮询，取消或改间隔不用等整段间隔结束。
// 参数 s（Host）：能读和保存重载计划的宿主。
// 返回：无。这个循环一直跑到进程退出。抓取失败时不改正在使用的价格，也不把下次运行时间往后推。
// 调用：StartScheduledReload 在计划已启用时。
// 测试：无直接单测
func scheduledReloadLoop(s Host) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for range t.C {
		plan := loadCostReload(s)
		if !plan.Scheduled || plan.NextRun == nil {
			continue
		}
		due, err := time.Parse(time.RFC3339, *plan.NextRun)
		if err != nil || time.Now().UTC().Before(due) {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		n, err := reloadNow(ctx)
		cancel()
		if err != nil {
			// A failed reload leaves the prices in use and the plan armed, so the
			// next cycle tries again. Pushing next_run out would hide the failure.
			logx.Error("scheduled price reload failed: %v", err)
			continue
		}
		stamp := time.Now().UTC().Format(time.RFC3339)
		plan.LastRun = &stamp
		if plan.IntervalHours != nil {
			next := time.Now().UTC().Add(time.Duration(*plan.IntervalHours) * time.Hour).Format(time.RFC3339)
			plan.NextRun = &next
		}
		if err := saveCostReload(s, plan); err != nil {
			logx.Error("scheduled price reload could not save its plan: %v", err)
			continue
		}
		logx.Info("scheduled price reload applied %d models", n)
	}
}
