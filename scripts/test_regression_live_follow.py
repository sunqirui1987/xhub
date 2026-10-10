"""实时回归日志的本地检查。不连接数据库，不调用付费供应商。"""
import json
import os
from pathlib import Path
import subprocess
import tempfile
import time
import unittest

SCRIPT = Path(__file__).with_name("regression_live_follow.py")


class LiveFollowTests(unittest.TestCase):
    def test_prints_progress_while_stdin_is_quiet(self):
        """验证静默等待时仍写出当前用例。

        前置：把心跳改成 1 秒，标准输入先只给一个 RUN。
        验证：1 秒内出现 RUN，随后在没有新事件时出现“仍在执行”。
        清理：关闭子进程并删除临时目录。
        """
        with tempfile.TemporaryDirectory() as directory:
            out = Path(directory)
            live = out / "live.log"
            raw = out / "test.jsonl"
            report = out / "report.md"
            env = dict(os.environ)
            env["XHUB_REGRESSION_LOG_HEARTBEAT"] = "1"
            proc = subprocess.Popen(
                ["python3", "-u", str(SCRIPT), str(live), str(raw), str(report)],
                stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                text=True, env=env,
            )
            proc.stdin.write(json.dumps({"Action": "run", "Test": "TestQuiet"}) + "\n")
            proc.stdin.write("Live regression: FENNO models=gpt-5.5\n")
            proc.stdin.write(json.dumps({
                "Action": "output", "Test": "TestQuiet",
                "Output": "token sk-abcdefghijklmnopqrstuvwxyz\n",
            }) + "\n")
            proc.stdin.flush()
            deadline = time.time() + 4
            seen = ""
            while time.time() < deadline and "仍在执行 TestQuiet" not in seen:
                time.sleep(0.2)
                seen = live.read_text() if live.exists() else ""
            proc.stdin.write(json.dumps({"Action": "pass", "Test": "TestQuiet", "Elapsed": 1.2}) + "\n")
            proc.stdin.write(json.dumps({"Action": "pass", "Elapsed": 1.2}) + "\n")
            proc.stdin.close()
            stdout = proc.stdout.read()
            stderr = proc.stderr.read()
            proc.stdout.close()
            proc.stderr.close()
            proc.wait(timeout=5)
            self.assertEqual(proc.returncode, 0, stderr + "\n" + seen)
            self.assertIn("RUN  TestQuiet", seen)
            self.assertIn("仍在执行 TestQuiet", seen)
            self.assertIn("Live regression: FENNO models=gpt-5.5", seen)
            self.assertNotIn("abcdefghijklmnopqrstuvwxyz", stdout + seen + report.read_text())
            self.assertIn("sk-***", raw.read_text())
            self.assertIn("包结果: PASS", report.read_text())
            self.assertIn("PASS TestQuiet", stdout)


if __name__ == "__main__":
    unittest.main()
