// Package usage registers its routes here. Spend reports and health probes are mounted from this file, not from the process route table.
package usage

import (
	"net/http"

	"github.com/sunqirui1987/xhub/internal/gateway/identity"
	"github.com/sunqirui1987/xhub/internal/httpx"
	"github.com/sunqirui1987/xhub/internal/logx"
	"sync"
)

// mountHost serves both usage queries and paged spend logs. Paging is implemented in identity. This alias only requires those methods.
type mountHost interface {
	Host
	identity.Gate
}

var logTraceOnceMount sync.Once

// Module is spend reports, activity summaries, and health probes.
// 参数 h（mountHost）：实现这一步所需能力的mountHost。聊天、直通和刷写各自只依赖自己的方法。
// 返回 httpx.Module（httpx.Module）：可挂到网关上的模块。
// 调用：gateway/family/mount.go、gateway/guard/mount.go、gateway/identity/mount.go、gateway/keys/mount.go
// 测试：无直接单测
func Module(h mountHost) httpx.Module {
	logTraceOnceMount.Do(func() { logx.Trace("enter usage.Module") })

	traceModule("usage")
	return httpx.Bind("usage", func(reg httpx.Registrar) {
		reg.Handle("GET /global/spend/teams", func(w http.ResponseWriter, r *http.Request) { SpendTeams(h, w, r) })
		reg.Handle("GET /spend/logs/v2", func(w http.ResponseWriter, r *http.Request) { LogsV2(h, w, r) })
		// The console's Logs page reads this collection route, and only the
		// per-request variant was registered. Its request fell through to the
		// catalog's generic key-value store and answered an empty list, so the
		// page rendered no rows however much traffic the deployment had. It is
		// the same read as v2, so it is served by the same handler.
		reg.Handle("GET /spend/logs/ui", func(w http.ResponseWriter, r *http.Request) { LogsV2(h, w, r) })
		reg.Handle("GET /spend/logs/ui/{request_id}", func(w http.ResponseWriter, r *http.Request) { LogByID(h, w, r) })
		// The session drawer lists every call that shares a session id. It must
		// not collapse them, or the clicked log has nothing to open.
		reg.Handle("GET /spend/logs/session/ui", func(w http.ResponseWriter, r *http.Request) { SessionLogs(h, w, r) })
		reg.Handle("GET /global/spend/logs", func(w http.ResponseWriter, r *http.Request) { SpendLogs(h, w, r) })
		reg.Handle("GET /global/spend/keys", func(w http.ResponseWriter, r *http.Request) { SpendKeys(h, w, r) })
		reg.Handle("GET /global/spend/models", func(w http.ResponseWriter, r *http.Request) { SpendModels(h, w, r) })
		reg.Handle("GET /global/spend/provider", func(w http.ResponseWriter, r *http.Request) { SpendProvider(h, w, r) })
		reg.Handle("POST /global/spend/end_users", func(w http.ResponseWriter, r *http.Request) { SpendEndUsers(h, w, r) })
		reg.Handle("GET /user/daily/activity", func(w http.ResponseWriter, r *http.Request) { UserDailyActivity(h, w, r) })
		reg.Handle("GET /user/daily/activity/aggregated", func(w http.ResponseWriter, r *http.Request) { UserDailyActivityAggregated(h, w, r) })
		reg.Handle("GET /team/daily/activity", func(w http.ResponseWriter, r *http.Request) { TeamDailyActivity(h, w, r) })
		reg.Handle("GET /team/daily/activity/aggregated", func(w http.ResponseWriter, r *http.Request) { TeamDailyActivityAggregated(h, w, r) })
		reg.Handle("GET /team/spend/by_user", func(w http.ResponseWriter, r *http.Request) { TeamSpendByUser(h, w, r) })
		reg.Handle("GET /organization/daily/activity", func(w http.ResponseWriter, r *http.Request) { OrganizationDailyActivity(h, w, r) })
		reg.Handle("GET /gateway/daily/activity", func(w http.ResponseWriter, r *http.Request) { GatewayDailyActivity(h, w, r) })
		reg.Handle("POST /usage/ai/chat", func(w http.ResponseWriter, r *http.Request) { UsageAIChat(h, w, r) })
		reg.Handle("GET /global/activity", func(w http.ResponseWriter, r *http.Request) { Activity(h, w, r) })
		reg.Handle("GET /global/activity/model", func(w http.ResponseWriter, r *http.Request) { ActivityModel(h, w, r) })
		reg.Handle("GET /global/activity/cache_hits", func(w http.ResponseWriter, r *http.Request) { ActivityCacheHits(h, w, r) })
		reg.Handle("POST /spend/calculate", func(w http.ResponseWriter, r *http.Request) { Calculate(h, w, r) })
		reg.Handle("GET /spend/keys", func(w http.ResponseWriter, r *http.Request) { Keys(h, w, r) })
		reg.Handle("GET /spend/users", func(w http.ResponseWriter, r *http.Request) { Users(h, w, r) })
		reg.Handle("GET /tag/list", func(w http.ResponseWriter, r *http.Request) { TagList(h, w, r) })
		reg.Handle("GET /spend/tags", func(w http.ResponseWriter, r *http.Request) { Tags(h, w, r) })
		reg.Handle("GET /global/spend/tags", func(w http.ResponseWriter, r *http.Request) { SpendTags(h, w, r) })
		reg.Handle("GET /global/spend/all_tag_names", func(w http.ResponseWriter, r *http.Request) { SpendTagNames(h, w, r) })
		reg.Handle("POST /health/test_connection", func(w http.ResponseWriter, r *http.Request) { HealthTestConnection(h, w, r) })
		reg.Handle("GET /health/services", func(w http.ResponseWriter, r *http.Request) { HealthServices(h, w, r) })
		reg.Handle("GET /test", func(w http.ResponseWriter, r *http.Request) { HealthTest(h, w, r) })
		reg.Handle("GET /auto_router/benchmarks", func(w http.ResponseWriter, r *http.Request) { Benchmarks(h, w, r) })
	})
}
