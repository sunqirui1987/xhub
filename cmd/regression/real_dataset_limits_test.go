package regression

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"testing"
	"time"
)

// TestRealDatasetLimitsAcceptance 验证实际 Python 限额验收不会再把有下级分配/已消费的组织设零。
// 前置为独立 PostgreSQL、真实网关和本地计量供应商，基线五层有固定额度且已有消费；
// 覆盖五预算、四层 RPM/TPM 拒绝零外发、恢复响应/账单、模型白名单、成员密钥生命周期，
// 以及恢复失败与部分创建失败的清理。临时资源由脚本删除，剩余夹具与报告由 harness/t.TempDir 清理，无外网凭据。
func TestRealDatasetLimitsAcceptance(t *testing.T) {
	h, admin, baseline, model := budgetFixture(t, "dataset-limits")
	h.setOrgBudget(t, admin, baseline.orgID, 100)
	h.setTeamBudget(t, admin, baseline.teamID, 30)
	h.setProjectBudget(t, admin, baseline.projectID, 20)
	h.setUserBudget(t, admin, baseline.userID, 10)
	h.setKeyBudget(t, admin, baseline.key, 3)
	h.ok(http.MethodPost, "/key/update", admin, map[string]any{"key": baseline.key, "models": []string{model}})
	templateID := routeTemplate(t, h, admin, "limits inherited route", routeTemplateBody([]any{}, 1, 30, 3, 0))
	bindTemplate(t, h, admin, "organization", baseline.orgID, templateID)
	h.assertBilled(t, baseline, admin, model, "baseline already spent", []string{model})
	denied := h.do(http.MethodPatch, "/organization/update", admin, map[string]any{
		"organization_id": baseline.orgID, "max_budget": 0})
	if denied.status != http.StatusBadRequest || denied.json()["error"].(map[string]any)["type"] != "quota_allocation_exceeded" {
		t.Fatalf("已分配且消费的组织设零应拒绝，实际 %s", denied.describe())
	}
	// 只读观察端点让 Python 自己判断每次拒绝前后的上游数量，避免在 Go 中复制验收逻辑。
	observer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(h.upstreamCalls())
	}))
	defer observer.Close()
	// Python wait_bill 读取持久化价格快照；定时推进真实结算，避免依赖生产刷新间隔。
	done := make(chan struct{})
	flushed := make(chan struct{})
	go func() {
		defer close(flushed)
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				h.gw.FlushSpend()
			}
		}
	}()
	defer func() { close(done); <-flushed }()
	command := exec.Command("python3", "-c", `
import json, sys
from pathlib import Path
from urllib.request import urlopen
sys.path.insert(0, "../../e2e")
from real_dataset import Dataset, load_manifest

class LocalObserver:
    """用途：只读本地供应商调用记录；无实例参数，rows 返回列表；供 refuse 核对零外发，不包含供应商凭据。"""
    @property
    def rows(self):
        """用途：实时获取累计上游请求；无参数，返回列表；供拒绝前后数量比较，无写入副作用。"""
        with urlopen(sys.argv[10]) as response:
            return [{"messages": json.dumps(row["Body"].get("messages", [])), "status": 200}
                    for row in json.load(response)]

dataset = Dataset(sys.argv[1], sys.argv[2], load_manifest(), observer=LocalObserver())
dataset.admin = sys.argv[3]
key = dict(zip(("organization_id", "team_id", "project_id", "user_id", "key", "token_id", "call_model"), sys.argv[4:10] + [sys.argv[11]]))
key["profile"] = "inherit"
dataset.data["billing"].update(input_cost_per_token=0.000002, output_cost_per_token=0.000008)
dataset.state["users"] = [{"id": key["user_id"], "email": sys.argv[12]}]
dataset.state["member_password"] = sys.argv[13]
fields = ("max_budget", "rpm_limit", "tpm_limit")
routes = {
    "organization": ("/organization/info?organization_id=", None),
    "team": ("/team/info?team_id=", "team_info"),
    "user": ("/user/info?user_id=", "user_info"),
    "project": ("/project/info?project_id=", "project"),
    "key": ("/key/info?key=", "info"),
}
def snapshot():
    """用途：读取基线四层额度配置；无参数，返回字段快照；供验收前后比较，只有只读管理调用。"""
    result = {}
    for scope, (route, wrapper) in routes.items():
        identity = key["token_id"] if scope == "key" else key[scope + "_id"]
        body, _ = dataset.api(route + identity)
        info = body[wrapper] if wrapper else body
        result[scope] = {field: info.get(field) for field in fields}
    return result
before = snapshot()
real_api = dataset.api
created = []
def tracking_api(route, body=None, **kwargs):
    """用途：记录真实创建的临时 ID；参数与 Dataset.api 一致，返回真实响应；供最终删除核对，无额外写入。"""
    reply = real_api(route, body, **kwargs)
    if route in ("/organization/new", "/team/new", "/user/new", "/project/new", "/key/generate"):
        created.append((route, reply[0]))
    return reply
dataset.api = tracking_api
dataset.verify_limits(key)
expected_checks = {"budget-" + scope + "-no-egress" for scope in ("organization", "team", "project", "user", "key")}
expected_checks.update(("" if scope == "key" else scope + "-") + field + "-reject-and-real-recovery"
    for scope in ("organization", "team", "user", "key") for field in ("rpm_limit", "tpm_limit"))
expected_checks.update(("model-allowlist-no-egress", "member-key-generate-block-rotate-real-calls"))
assert expected_checks <= {c["name"] for c in dataset.report["checks"] if c["passed"]}, "五预算、八限流、白名单、生命周期必须全部通过"
assert snapshot() == before, "验收不得改动保留租户额度"

for failure in ("create", "recovery"):
    def failing_api(route, body=None, **kwargs):
        """用途：在真实组织/团队/个人创建后注入项目失败；参数兼容 API，返回响应或异常；失败前创建资源仍须由上下文删除。"""
        if failure == "create" and route == "/project/new":
            raise RuntimeError("injected creation failure")
        return tracking_api(route, body, **kwargs)
    dataset.api = failing_api
    try:
        with dataset.temporary_limit_subject(key, "failure-" + failure) as subject:
            dataset.call(subject, "before-recovery-failure-" + dataset.run)
            raise RuntimeError("injected recovery failure")
    except RuntimeError as error:
        assert "injected" in str(error), "应保留原始失败"
    else:
        raise AssertionError("失败场景没有失败")
dataset.api = tracking_api
for route, row in created:
    if route == "/organization/new":
        dataset.api("/organization/info?organization_id=" + row["organization_id"], expected=404)
    elif route == "/team/new":
        dataset.api("/team/info?team_id=" + row["team_id"], expected=404)
    elif route == "/user/new":
        dataset.api("/user/info?user_id=" + row["user_id"], expected=404)
    elif route == "/project/new":
        dataset.api("/project/info?project_id=" + row["project_id"], expected=404)
    elif route == "/key/generate":
        dataset.api("/key/info?key=" + row["token_id"], expected=404)
assert snapshot() == before, "异常清理不得影响保留额度"
dataset.save()
`, h.server.URL, t.TempDir(), admin, baseline.orgID, baseline.teamID, baseline.projectID, baseline.userID, baseline.key, baseline.keyID, observer.URL, model, baseline.email, baseline.password)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("真实限额验收脚本失败: %v\n%s", err, output)
	}
	if got := len(h.upstreamCalls()); got != 17 {
		t.Fatalf("应仅有基线 1、限额恢复 13、生命周期 2、异常清理前 1 次上游，实际 %d", got)
	}
}
