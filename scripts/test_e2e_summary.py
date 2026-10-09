"""Report integrity checks; no gateway or paid provider calls."""
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

SCRIPT = Path(__file__).with_name("e2e-summary.py")
SCENARIO = {"name": "identical", "deployments": [{"id": "a"}, {"id": "b"}]}


class ReportTests(unittest.TestCase):
    def report(self, *, browser=True, errors=None, weighted=None, configured=False, backend=True, preflight=None, backend_failure=False, browser_failure=False, browser_log=None, backend_log=None):
        with tempfile.TemporaryDirectory() as directory:
            out = Path(directory)
            (out / "browser").mkdir()
            (out / "evidence").mkdir()
            (out / ".e2e-report").mkdir()
            if preflight:
                (out / "preflight.log").write_text(preflight)
            if browser_log:
                (out / "browser.log").write_text(browser_log)
            if browser:
                (out / "browser/results.json").write_text(json.dumps({
                    "stats": {"expected": 0 if browser_failure else 1, "unexpected": int(browser_failure), "skipped": 0},
                    "errors": [{"message": error} for error in errors or []],
                    "suites": [{"title": "browser", "specs": [{"title": "click and reload",
                        "tests": [{"status": "unexpected" if browser_failure else "expected", "results": [{"duration": 1,
                            "errors": [{"message": "\u001b[31mError: saved value missing\u001b[0m\nExpected: 7\nReceived: 3"}] if browser_failure else []}]}]}]}],
                }))
            if backend:
                (out / "backend.log").write_text(
                    backend_log if backend_log is not None else
                    "=== RUN   TestBusiness\n--- PASS: TestBusiness (1.00s)\n"
                    "    --- PASS: TestBusiness/child (0.50s)\n"
                    "    --- SKIP: TestOptional/media (0.00s)\n"
                    "ok github.com/sunqirui1987/xhub/internal/regression 1.00s\n")
                if backend_failure:
                    with (out / "backend.log").open("a") as log:
                        log.write("=== RUN   TestLive\n    live_test.go:40: upstream DNS timeout\n--- FAIL: TestLive (30.00s)\n")
            if weighted is not None:
                (out / "evidence/weighted-identical.json").write_text(json.dumps(weighted))
            (out / ".e2e-report/routes.txt").write_text("route unsupported POST /unsupported 501\n")
            env = dict(os.environ, E2E_ACCEPTANCE_DIR=str(out), E2E_RUN_DIR=str(out / "browser"),
                       E2E_BROWSER_STATUS="0", E2E_BACKEND_STATUS="0", E2E_FULL_COVERAGE="1",
                       E2E_PROVIDER_METADATA=json.dumps({"providers": [], "weighted_scenarios": [SCENARIO] if configured else []}))
            result = subprocess.run(["python3", str(SCRIPT)], env=env, capture_output=True, text=True)
            self.last_stdout = result.stdout
            summary = json.loads((out / "summary.json").read_text())
            self.assertTrue((out / "report.md").is_file())
            self.assertTrue((out / "report.html").is_file())
            return result.returncode, summary, (out / "report.html").read_text()

    def test_missing_results_cannot_pass_even_with_zero_exit_codes(self):
        code, summary, _ = self.report(browser=False, backend=False)
        self.assertEqual((code, summary["status"]), (1, "failed"))

    def test_global_browser_errors_fail_and_are_html_escaped(self):
        code, summary, html = self.report(errors=["<script>startup failed</script>"])
        self.assertEqual(code, 1)
        self.assertTrue(summary["browser_errors"])
        self.assertIn("&lt;script&gt;", html)
        self.assertNotIn("<script>", html)

    def test_configured_weighted_evidence_is_required(self):
        code, summary, _ = self.report(configured=True)
        self.assertEqual(code, 1)
        self.assertEqual(summary["missing_weighted_evidence"], ["identical"])

    def test_partial_paid_success_does_not_pass_the_scenario(self):
        code, summary, _ = self.report(configured=True, weighted={
            "scenario": "identical", "status": "failed", "requests": 10,
            "calls": [{"deployment": "a", "cost_usd": 0.1}]})
        self.assertEqual(code, 1)
        self.assertEqual(summary["weighted"][0]["status"], "failed")

    def test_pass_does_not_double_count_children_or_unsupported_routes(self):
        code, summary, _ = self.report(configured=True, weighted={
            "scenario": "identical", "status": "passed", "requests": 10,
            "expected": {"a": 7, "b": 3}, "calls": [
                {"call_id": str(i), "deployment": "a" if i < 7 else "b", "cost_usd": 0.1}
                for i in range(10)]})
        self.assertEqual(code, 0)
        self.assertEqual(summary["backend_pass"], 1)
        self.assertEqual(summary["backend_skip"], ["TestOptional/media"])
        self.assertEqual(len(summary["unsupported_routes"]), 1)

    def test_incomplete_evidence_marked_passed_is_still_rejected(self):
        code, summary, _ = self.report(configured=True, weighted={
            "scenario": "identical", "status": "passed", "requests": 10,
            "expected": {"a": 7, "b": 3}, "calls": []})
        self.assertEqual(code, 1)
        self.assertEqual(summary["invalid_weighted_evidence"], ["identical"])

    def test_successful_fallback_cannot_pass_a_healthy_weight_cycle(self):
        code, summary, html = self.report(configured=True, weighted={
            "scenario": "identical", "status": "passed", "requests": 2,
            "expected": {"a": 1, "b": 1}, "healthy_cycle": False,
            "failure_reason": "upstream failure invalidated healthy cycle",
            "attempts": [{"request": 1, "status": 500}, {"request": 1, "status": 200},
                         {"request": 2, "status": 200}],
            "calls": [{"call_id": "1", "deployment": "a", "cost_usd": 0.1},
                      {"call_id": "2", "deployment": "b", "cost_usd": 0.1}]})
        self.assertEqual(code, 1)
        self.assertEqual(summary["invalid_weighted_evidence"], ["identical"])
        self.assertIn("upstream failure invalidated healthy cycle", self.last_stdout)
        self.assertIn("upstream failure invalidated healthy cycle", html)

    def test_preflight_failure_reason_is_preserved(self):
        code, summary, html = self.report(browser=False, backend=False, preflight="Invalid provider YAML\n")
        self.assertEqual(code, 1)
        self.assertEqual(summary["preflight_errors"], ["Invalid provider YAML"])
        self.assertIn("Invalid provider YAML", html)

    def test_backend_failure_reason_is_preserved(self):
        code, summary, html = self.report(backend_failure=True)
        self.assertEqual(code, 1)
        self.assertEqual(summary["backend_fail"], 1)
        self.assertIn("upstream DNS timeout", summary["backend_diagnostics"][0])
        self.assertIn("upstream DNS timeout", html)
        self.assertIn("[backend] TestLive", self.last_stdout)
        self.assertIn("upstream DNS timeout", self.last_stdout)
        failed = next(c for c in summary["backend_cases"] if c["name"] == "TestLive")
        self.assertEqual(failed["errors"], ["live_test.go:40: upstream DNS timeout"])
        passed = next(c for c in summary["backend_cases"] if c["name"] == "TestBusiness")
        self.assertEqual(passed["errors"], [])

    def test_backend_setup_failure_preserves_missing_module_diagnostic(self):
        code, summary, html = self.report(backend_log=
            "# github.com/sunqirui1987/xhub/internal/regression\n"
            "internal/gateway/guard/custom.go:9:2: no required module provides package go.starlark.net/starlark\n"
            "FAIL github.com/sunqirui1987/xhub/internal/regression [setup failed]\nFAIL\n")
        self.assertEqual(code, 1)
        self.assertEqual(summary["backend_pass"], 0)
        self.assertIn("no required module provides package", self.last_stdout)
        self.assertIn("go.starlark.net/starlark", html)

    def test_completed_failure_reports_assertion_without_success_or_skip_noise(self):
        code, summary, html = self.report(backend_log=
            "=== RUN   TestPassed\n    passed_test.go:10: informational usage fields\n"
            "--- PASS: TestPassed (1.00s)\n"
            "=== RUN   TestOptional\n    mode_test.go:19: simulated variant only\n"
            "--- SKIP: TestOptional (0.00s)\n"
            "=== RUN   TestWeighted\n    weighted_test.go:20: unhealthy upstream attempt\n"
            "--- FAIL: TestWeighted (2.00s)\n"
            "FAIL github.com/sunqirui1987/xhub/internal/regression 3.00s\n")
        self.assertEqual(code, 1)
        self.assertTrue(summary["backend_completed"])
        self.assertEqual(summary["incomplete_stages"], [])
        self.assertEqual(summary["backend_diagnostics"], ["weighted_test.go:20: unhealthy upstream attempt"])
        self.assertIn("unhealthy upstream attempt", self.last_stdout)
        self.assertNotIn("Suite did not finish successfully", self.last_stdout)
        self.assertNotIn("informational usage fields", html)
        self.assertNotIn("simulated variant only", html)

    def test_terminal_shows_browser_assertion_and_report_paths(self):
        code, _, _ = self.report(browser_failure=True)
        self.assertEqual(code, 1)
        self.assertIn("[browser] browser / click and reload", self.last_stdout)
        self.assertIn("Error: saved value missing; Expected: 7; Received: 3", self.last_stdout)
        self.assertNotIn("\u001b[", self.last_stdout)
        for label in ("Report HTML:", "Report Markdown:", "Results JSON:", "Logs:"):
            self.assertIn(label, self.last_stdout)

    def test_terminal_shows_preflight_and_missing_evidence_reasons(self):
        code, _, _ = self.report(browser=False, backend=False, configured=True, preflight="Invalid provider YAML\n")
        self.assertEqual(code, 1)
        for message in ("Test results missing", "Suite did not finish successfully",
                        "[preflight] Invalid provider YAML", "Evidence missing: identical"):
            self.assertIn(message, self.last_stdout)

    def test_build_failure_has_source_path_and_reason_without_browser_results(self):
        code, summary, html = self.report(browser=False, browser_log=
            "Failed to type check.\n\n./src/Panel.tsx:112:5\nType error: unknown is not an array\n")
        self.assertEqual(code, 1)
        self.assertIn("./src/Panel.tsx:112:5", self.last_stdout)
        self.assertIn("Type error: unknown is not an array", self.last_stdout)
        self.assertIn("Type error: unknown is not an array", html)
        self.assertTrue(summary["browser_diagnostics"])

    def test_interrupted_browser_is_not_misreported_as_preflight_failure(self):
        code, summary, html = self.report(browser=False, backend=False,
            preflight="Running 51 tests using 1 worker\n  ✓ 45 saved successfully\n",
            browser_log="Running 51 tests using 1 worker\n  ✓ 45 saved successfully\n")
        self.assertEqual(code, 1)
        self.assertEqual(summary["preflight_errors"], [])
        self.assertIsNone(summary["browser"])
        self.assertEqual(len(summary["incomplete_stages"]), 1)
        self.assertIn("执行未完成", html)
        self.assertIn("[incomplete]", self.last_stdout)
        self.assertIn("45 saved successfully", html)

    def test_backend_partial_passes_do_not_replace_suite_completion(self):
        code, summary, html = self.report(backend_log=
            "=== RUN   TestBusiness\n--- PASS: TestBusiness (1.00s)\n=== RUN   TestNext\n")
        self.assertEqual(code, 1)
        self.assertEqual(summary["backend_pass"], 1)
        self.assertEqual(summary["backend_fail"], 0)
        self.assertFalse(summary["backend_ok"])
        self.assertEqual(len(summary["incomplete_stages"]), 1)
        self.assertIn("后端已开始执行", html)


if __name__ == "__main__":
    unittest.main()
