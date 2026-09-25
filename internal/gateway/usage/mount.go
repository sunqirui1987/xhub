// 用量模块的路由。花费报表和健康探测从这里挂上，不写进进程的路由总表。
package usage

import (
	"net/http"

	"github.com/sunqirui1987/xhub/internal/gateway/identity"
	"github.com/sunqirui1987/xhub/internal/gateway/module"
)

// mountHost 同时满足用量查询和花费日志分页。分页实现在 identity，这里只要求那几个方法。
type mountHost interface {
	Host
	identity.Gate
}

// Module 是花费报表、活动汇总和健康探测。
func Module(h mountHost) module.Module {
	return module.Bind("usage", func(reg module.Registrar) {
		reg.Handle("GET /global/spend/teams", func(w http.ResponseWriter, r *http.Request) { SpendTeams(h, w, r) })
		reg.Handle("GET /spend/logs/v2", func(w http.ResponseWriter, r *http.Request) { LogsV2(h, w, r) })
		reg.Handle("GET /spend/logs/ui/{request_id}", func(w http.ResponseWriter, r *http.Request) { LogByID(h, w, r) })
		reg.Handle("GET /global/spend/logs", func(w http.ResponseWriter, r *http.Request) { SpendLogs(h, w, r) })
		reg.Handle("GET /global/spend/keys", func(w http.ResponseWriter, r *http.Request) { SpendKeys(h, w, r) })
		reg.Handle("GET /global/spend/models", func(w http.ResponseWriter, r *http.Request) { SpendModels(h, w, r) })
		reg.Handle("GET /global/spend/provider", func(w http.ResponseWriter, r *http.Request) { SpendProvider(h, w, r) })
		reg.Handle("POST /global/spend/end_users", func(w http.ResponseWriter, r *http.Request) { SpendEndUsers(h, w, r) })
		reg.Handle("GET /global/activity", func(w http.ResponseWriter, r *http.Request) { Activity(h, w, r) })
		reg.Handle("GET /global/activity/model", func(w http.ResponseWriter, r *http.Request) { ActivityModel(h, w, r) })
		reg.Handle("GET /global/activity/cache_hits", func(w http.ResponseWriter, r *http.Request) { ActivityCacheHits(h, w, r) })
		reg.Handle("POST /spend/calculate", func(w http.ResponseWriter, r *http.Request) { Calculate(h, w, r) })
		reg.Handle("GET /spend/keys", func(w http.ResponseWriter, r *http.Request) { Keys(h, w, r) })
		reg.Handle("GET /spend/users", func(w http.ResponseWriter, r *http.Request) { Users(h, w, r) })
		reg.Handle("GET /spend/tags", func(w http.ResponseWriter, r *http.Request) { Tags(h, w, r) })
		reg.Handle("GET /global/spend/tags", func(w http.ResponseWriter, r *http.Request) { SpendTags(h, w, r) })
		reg.Handle("GET /global/spend/all_tag_names", func(w http.ResponseWriter, r *http.Request) { SpendTagNames(h, w, r) })
		reg.Handle("POST /health/test_connection", func(w http.ResponseWriter, r *http.Request) { HealthTestConnection(h, w, r) })
		reg.Handle("GET /health/services", func(w http.ResponseWriter, r *http.Request) { HealthServices(h, w, r) })
		reg.Handle("GET /test", func(w http.ResponseWriter, r *http.Request) { HealthTest(h, w, r) })
		reg.Handle("GET /auto_router/benchmarks", func(w http.ResponseWriter, r *http.Request) { Benchmarks(h, w, r) })
	})
}
