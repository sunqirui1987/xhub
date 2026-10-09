"""专项入口契约测试：模拟阶段命令，运行真实入口及报告器，不调用付费服务。"""
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest

SCRIPTS = Path(__file__).resolve().parent


class ModelEndpointsRunnerTests(unittest.TestCase):
    """在临时工作区验证编排、退出码和清理；产品业务由 Playwright/Go 验证。"""

    def run_entry(self, *, browser_code=0, backend_code=0, missing=False, redis_code=0, locked=False):
        """参数注入阶段故障，返回退出码、报告、清理证据；临时工作区由上下文清理。

        调用方为入口契约测试；真实报告器读取本次产物，Docker/浏览器/后端命令不访问服务。
        """
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            scripts = root / "scripts"
            scripts.mkdir()
            (root / ".e2e/current").mkdir(parents=True)
            for name in ("e2e-model-endpoints.sh", "e2e-summary.py"):
                shutil.copyfile(SCRIPTS / name, scripts / name)
            binaries = root / "bin"
            binaries.mkdir()
            commands = {
                "docker": '''#!/usr/bin/env python3
import os, sys
from pathlib import Path
with Path("docker-calls.txt").open("a") as log: log.write(" ".join(sys.argv[1:]) + "\\n")
if sys.argv[1] == "run": raise SystemExit(int(os.environ["TEST_REDIS_CODE"]))
if sys.argv[1] == "port": print("127.0.0.1:16379")
''',
                "git": "#!/usr/bin/env bash\necho fixture-revision\n",
            }
            for name, source in commands.items():
                binary = binaries / name
                binary.write_text(source)
                binary.chmod(0o755)
            (scripts / "e2e.sh").write_text('''#!/usr/bin/env bash
echo 'Running 1 test using 1 worker'
python3 - <<'PY'
import json, os
from pathlib import Path
if os.environ["TEST_MISSING"] != "1":
    failed = int(os.environ["TEST_BROWSER_CODE"]) != 0
    Path(".e2e/current/results.json").write_text(json.dumps({
        "stats": {"expected": int(not failed), "unexpected": int(failed), "skipped": 0},
        "suites": [{"title": "fixture", "specs": [{"title": "persist",
            "tests": [{"status": "unexpected" if failed else "expected", "results": []}]}]}],
    }))
PY
exit "$TEST_BROWSER_CODE"
''')
            (scripts / "regression.sh").write_text('''#!/usr/bin/env bash
touch backend-started
echo '=== RUN   TestBusiness'
if [[ "$TEST_BACKEND_CODE" == 0 ]]; then
  echo '--- PASS: TestBusiness (0.01s)'
  echo 'ok github.com/sunqirui1987/xhub/internal/regression 0.01s'
else
  echo '--- FAIL: TestBusiness (0.01s)'
  echo 'FAIL github.com/sunqirui1987/xhub/internal/regression 0.01s'
fi
exit "$TEST_BACKEND_CODE"
''')
            if locked:
                (root / ".e2e/acceptance.lock").mkdir()
            env = dict(os.environ, PATH=str(binaries) + os.pathsep + os.environ["PATH"],
                       TEST_BROWSER_CODE=str(browser_code), TEST_BACKEND_CODE=str(backend_code),
                       TEST_MISSING=str(int(missing)), TEST_REDIS_CODE=str(redis_code))
            result = subprocess.run(["bash", str(scripts / "e2e-model-endpoints.sh")],
                                    cwd=root, env=env, capture_output=True, text=True, timeout=20)
            reports = list((root / ".e2e/runs").glob("*/summary.json"))
            summary = json.loads(reports[0].read_text()) if reports else None
            calls = root / "docker-calls.txt"
            evidence = {
                "backend_started": (root / "backend-started").exists(),
                "lock_exists": (root / ".e2e/acceptance.lock").exists(),
                "docker_calls": calls.read_text() if calls.exists() else "",
            }
            return result, summary, evidence

    def assert_cleaned(self, evidence):
        """检查本次资源删除；参数为编排证据，无返回值，未清理时断言失败。"""
        self.assertFalse(evidence["lock_exists"])
        self.assertIn("rm -f xhub-model-e2e-redis-", evidence["docker_calls"])

    def test_success_reports_pass_and_exits_zero(self):
        """正常证据生成通过报告且退出 0；临时 Redis 与锁清理。"""
        result, summary, evidence = self.run_entry()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(summary["status"], "passed")
        self.assertTrue(evidence["backend_started"])
        self.assert_cleaned(evidence)

    def test_browser_failure_still_runs_backend_and_preserves_code(self):
        """浏览器退出 7 后仍运行后端；报告保留 7、入口失败、资源清理。"""
        result, summary, evidence = self.run_entry(browser_code=7)
        self.assertEqual(result.returncode, 1)
        self.assertEqual((summary["browser_exit"], summary["backend_exit"]), (7, 0))
        self.assertEqual(summary["status"], "failed")
        self.assertTrue(evidence["backend_started"])
        self.assert_cleaned(evidence)

    def test_backend_failure_preserves_code_and_fails_entry(self):
        """后端退出 9 不被汇总掩盖；失败写报告、临时资源清理。"""
        result, summary, evidence = self.run_entry(backend_code=9)
        self.assertEqual(result.returncode, 1)
        self.assertEqual((summary["browser_exit"], summary["backend_exit"]), (0, 9))
        self.assertEqual(summary["backend_fail"], 1)
        self.assert_cleaned(evidence)

    def test_missing_browser_evidence_fails_despite_stage_success(self):
        """阶段退出 0 但报告缺失仍失败；后端执行且临时资源清理。"""
        result, summary, evidence = self.run_entry(missing=True)
        self.assertEqual(result.returncode, 1)
        self.assertEqual(summary["status"], "failed")
        self.assertIsNone(summary["browser"])
        self.assertTrue(summary["incomplete_stages"])
        self.assert_cleaned(evidence)

    def test_redis_start_failure_writes_report_and_preserves_exit(self):
        """Redis 退出 8 后不执行阶段；EXIT 写失败报告、释放锁、尝试清理 Redis。"""
        result, summary, evidence = self.run_entry(redis_code=8)
        self.assertEqual(result.returncode, 8)
        self.assertEqual(summary["status"], "failed")
        self.assertFalse(evidence["backend_started"])
        self.assert_cleaned(evidence)

    def test_existing_acceptance_lock_blocks_without_removing_owner_lock(self):
        """已有锁拒绝启动；不创建容器/报告、不删除他人锁，临时目录由测试清理。"""
        result, summary, evidence = self.run_entry(locked=True)
        self.assertEqual(result.returncode, 1)
        self.assertIn("Another full regression", result.stderr)
        self.assertIsNone(summary)
        self.assertTrue(evidence["lock_exists"])
        self.assertEqual(evidence["docker_calls"], "")


if __name__ == "__main__":
    unittest.main()
