package regression

import (
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestTestdataSeedWithoutModelCalls 验证建数器和四角色验收脚本通过真实管理路由执行且不调用任何供应商。
// 前置条件为独立 PostgreSQL schema、管理员及无模型网关；验证 81 把成员密钥和管理员个人密钥、持久化模型和零上游调用，
// 同时验证组织管理员可写三项团队额度、跨组织/模型/状态越权被拒绝，原配置不变；已有组织时再次建数必须失败。
// schema 由 harness 清理，报告由临时目录清理，不依赖供应商凭据。
func TestTestdataSeedWithoutModelCalls(t *testing.T) {
	h := newHarness(t)
	admin := h.adminSession()
	directory := t.TempDir()
	command := exec.Command("python3", "../../e2e/real_dataset.py", "--gateway", h.server.URL, "--directory", directory, "--seed-only")
	command.Env = append(os.Environ(), "E2E_DATASET_ADMIN=regression-admin@example.com", "E2E_DATASET_PASSWORD=regression-admin-password", "FENNO_AI_API_KEY=", "QINIU_API_KEY=")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("无供应商凭据建数失败: %v\n%s", err, output)
	}
	raw, err := os.ReadFile(filepath.Join(directory, "access.json"))
	if err != nil {
		t.Fatal(err)
	}
	var access map[string]json.RawMessage
	if err := json.Unmarshal(raw, &access); err != nil {
		t.Fatal(err)
	}
	for name, expected := range map[string]int{"organizations": 3, "teams": 9, "users": 27, "projects": 9, "keys": 81, "deployments": 7} {
		var rows []any
		if err := json.Unmarshal(access[name], &rows); err != nil || len(rows) != expected {
			t.Fatalf("建数资源 %s 数量应为 %d，实际 %d，错误 %v", name, expected, len(rows), err)
		}
	}
	var adminKey map[string]any
	if err := json.Unmarshal(access["admin_key"], &adminKey); err != nil {
		t.Fatalf("缺少管理员个人密钥: %v", err)
	}
	if adminKey["owner_type"] != "personal" || adminKey["team_id"] != nil || adminKey["key_alias"] != "管理员个人密钥" {
		t.Fatal("管理员密钥应属于管理员本人且无需绑定团队")
	}
	personal := h.ok(http.MethodGet, "/key/list?scope=personal", admin, nil)
	adminRows := rowsOf(personal, "keys")
	if len(adminRows) != 1 || adminRows[0]["token_id"] != adminKey["token_id"] {
		t.Fatalf("管理员个人列表应仅有新建的管理员密钥，实际 %d 把", len(adminRows))
	}
	raw, err = os.ReadFile(filepath.Join(directory, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	var report map[string]any
	if err := json.Unmarshal(raw, &report); err != nil {
		t.Fatal(err)
	}
	if report["status"] != "seeded" || report["phase"] != "seed" || report["admin_key_count"] != float64(1) || report["total_key_count"] != float64(82) || len(report["calls"].([]any)) != 0 || len(report["probe_attempts"].([]any)) != 0 || report["agent_conversations"] != nil || report["media_tasks"] != nil {
		t.Fatalf("建数不应生成模型验收证据: %s", raw)
	}
	groups := h.ok(http.MethodGet, "/model/groups?size=200", admin, nil).json()
	if len(listField(groups, "data")) != 6 {
		t.Fatalf("清单的 6 个公开模型组未全部持久化: %#v", groups)
	}
	if len(h.upstreamCalls()) != 0 {
		t.Fatal("建数阶段发生了模型调用")
	}
	logs := rowsOf(h.ok(http.MethodGet, "/spend/logs/ui?page_size=200", admin, nil), "data")
	if len(logs) != 0 {
		t.Fatalf("建数阶段不应产生数据面调用日志，实际 %d 条", len(logs))
	}
	// 直接执行发生故障的 Python 权限验收，而非在 Go 中复制它的断言，防止脚本与后台规则再次漂移。
	command = exec.Command("python3", "-c", `
import json, sys
from pathlib import Path
sys.path.insert(0, "../../e2e")
from real_dataset import Dataset, load_manifest
directory = Path(sys.argv[2])
access = json.loads((directory / "access.json").read_text())
dataset = Dataset(sys.argv[1], directory, load_manifest())
dataset.state = access
dataset.admin = access["admin"]
fields = ("max_budget", "rpm_limit", "tpm_limit", "models", "status")
before = {}
for team in dataset.state["teams"]:
    body, _ = dataset.api("/team/info?team_id=" + team["id"])
    before[team["id"]] = {field: body["team_info"][field] for field in fields}
dataset.verify_permissions()
assert len(dataset.state["personas"]) == 4, "四类角色必须全部通过"
for team in dataset.state["teams"]:
    body, _ = dataset.api("/team/info?team_id=" + team["id"])
    assert {field: body["team_info"][field] for field in fields} == before[team["id"]], "权限验收改变了原额度、模型或状态"
`, h.server.URL, directory)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("真实四角色权限验收失败: %v\n%s", err, output)
	}
	if len(h.upstreamCalls()) != 0 {
		t.Fatal("权限验收不应产生供应商调用")
	}
	command = exec.Command("python3", "../../e2e/real_dataset.py", "--gateway", h.server.URL, "--directory", t.TempDir(), "--seed-only")
	command.Env = append(os.Environ(), "E2E_DATASET_ADMIN=regression-admin@example.com", "E2E_DATASET_PASSWORD=regression-admin-password")
	if err := command.Run(); err == nil {
		t.Fatal("已有组织时建数应拒绝覆盖")
	}
}
