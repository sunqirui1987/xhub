// Package usage registers its routes here. Spend reports and health probes are mounted from this file, not from the process route table.
package usage

import (
	"net/http"

	"github.com/sunqirui1987/xhub/internal/gateway/identity"
	"github.com/sunqirui1987/xhub/internal/httpx"
)

// mountHost serves both usage queries and paged spend logs. Paging is implemented in identity. This alias only requires those methods.
type mountHost interface {
	Host
	identity.Gate
}

// Module is spend reports, activity summaries, and health probes.
func Module(h mountHost) httpx.Module {
	return httpx.Bind("usage", func(reg httpx.Registrar) {
		reg.Handle("GET /global/spend/teams", func(w http.ResponseWriter, r *http.Request) { SpendTeams(h, w, r) })
		reg.Handle("GET /spend/logs/v2", func(w http.ResponseWriter, r *http.Request) { LogsV2(h, w, r) })
		reg.Handle("GET /spend/logs/ui/{request_id}", func(w http.ResponseWriter, r *http.Request) { LogByID(h, w, r) })
		reg.Handle("GET /global/spend/logs", func(w http.ResponseWriter, r *http.Request) { SpendLogs(h, w, r) })
		reg.Handle("GET /global/spend/keys", func(w http.ResponseWriter, r *http.Request) { SpendKeys(h, w, r) })
		reg.Handle("GET /global/spend/models", func(w http.ResponseWriter, r *http.Request) { SpendModels(h, w, r) })
		reg.Handle("GET /global/spend/provider", func(w http.ResponseWriter, r *http.Request) { SpendProvider(h, w, r) })
		reg.Handle("POST /global/spend/end_users", func(w http.ResponseWriter, r *http.Request) { SpendEndUsers(h, w, r) })
		reg.Handle("GET /user/daily/activity", func(w http.ResponseWriter, r *http.Request) { UserDailyActivity(h, w, r) })
		reg.Handle("GET /user/daily/activity/aggregated", func(w http.ResponseWriter, r *http.Request) { UserDailyActivityAggregated(h, w, r) })
		reg.Handle("GET /gateway/daily/activity", func(w http.ResponseWriter, r *http.Request) { GatewayDailyActivity(h, w, r) })
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
