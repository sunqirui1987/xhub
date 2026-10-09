// Package models refreshes the market price catalog on a timer when the console
// has armed one. A scheduled reload is the same call as the manual one: fetch
// the feed, and only swap the prices in if the fetch produced a usable catalog.
package models

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/sunqirui1987/xhub/internal/catalog"
	"github.com/sunqirui1987/xhub/internal/logx"
)

var reloadLoops sync.Map

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
	if s.RecordStore() == nil {
		return
	}
	if _, running := reloadLoops.LoadOrStore(s.RecordStore(), true); running {
		return
	}
	go func() { defer reloadLoops.Delete(s.RecordStore()); scheduledReloadLoop(s) }()
}

// reloadNow 从 Modelink 或运维配置的镜像读取完整价格；失败时保留当前目录。
// 参数 ctx（context.Context）：取消和超时。
// 返回 int（int）：换上的模型条数；error（error）：抓取或应用失败时不为 nil，此时价格表不动。
// 调用：ReloadCostMap、scheduledReloadLoop。
// 测试：schedule_test.go 的缺失源、失败源与目录保留断言。
func reloadNow(ctx context.Context) (int, error) {
	return catalog.ReloadFromMarket(ctx, priceFeedURL())
}

// priceFeedURL 解析运行时价格来源；参数无，返回显式镜像或 Modelink 默认地址。
// 调用：手动和定时重载；只解析配置，不发起网络请求。
func priceFeedURL() string {
	if url := strings.TrimSpace(os.Getenv("XHUB_PRICE_FEED_URL")); url != "" {
		return url
	}
	return catalog.MarketURL
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
		// 数据库关闭表示宿主已结束；短暂断连继续等待，避免丢失已保存计划。
		if err := s.RecordStore().DB.Ping(); err != nil {
			if strings.Contains(err.Error(), "database is closed") {
				return
			}
			continue
		}
		plan := loadCostReload(s)
		if !plan.Scheduled {
			continue
		}
		if plan.NextRun == nil {
			continue
		}
		due, err := time.Parse(time.RFC3339, *plan.NextRun)
		if err != nil || time.Now().UTC().Before(due) {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		n, err := refreshLocalCatalog(ctx, s)
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

// refreshLocalCatalog 拉取并保存完整本地快照；参数为上下文和宿主，返回模型数与错误。
// 手动及定时刷新调用；远程移除条目保留但标记不可售，失败不覆盖目录，上下架状态独立持久化。
func refreshLocalCatalog(ctx context.Context, s Host) (int, error) {
	doc, err := catalog.FetchMarket(ctx, priceFeedURL())
	if err != nil {
		return 0, err
	}
	if s.RecordStore() == nil {
		return 0, fmt.Errorf("local catalog store unavailable")
	}
	// 不把手工价格覆盖固化进市场基线。
	previous, err := s.RecordStore().ListConfig(priceSnapshotNS)
	if err != nil {
		return 0, err
	}
	var prior catalog.PriceDocument
	if raw, exists := previous["catalog"]; exists {
		bytes, _ := json.Marshal(raw)
		if err := json.Unmarshal(bytes, &prior); err != nil {
			return 0, err
		}
	} else {
		// 首次刷新取编译基线补齐历史记录，不把手工价格覆盖固化进市场快照。
		prior.Models = map[string]map[string]any{}
		for id := range catalog.CostMap() {
			if row, ok := catalog.BaselineModel(id); ok {
				prior.Models[id] = row
			}
		}
	}
	for id, row := range prior.Models {
		if _, exists := doc.Models[id]; !exists {
			copy := map[string]any{}
			for key, value := range row {
				copy[key] = value
			}
			copy["feed_unavailable"] = true
			doc.Models[id] = copy
		}
	}
	if err := s.RecordStore().PutConfig(priceSnapshotNS, "catalog", doc); err != nil {
		return 0, err
	}
	n, err := catalog.ApplyDocument(doc)
	if err != nil {
		return 0, err
	}
	LoadPriceOverrides(s)
	return n, nil
}
