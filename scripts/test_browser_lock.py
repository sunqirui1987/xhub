"""浏览器串行锁契约测试：使用真实 Bash 和临时目录，不启动外部服务。"""
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

SCRIPT = Path(__file__).with_name("browser-lock.sh")


class BrowserLockTests(unittest.TestCase):
    """验证获取、等待、超时和配置错误；临时目录由上下文清理。"""

    def acquire(self, lock, timeout):
        """用途：执行真实锁函数；参数为目录和超时，返回子进程结果；供契约测试调用，成功保留锁供调用方释放。"""
        return subprocess.run(["bash", "-c", 'source "$1"; acquire_browser_lock "$2"',
                               "lock-test", str(SCRIPT), str(lock)],
                              env={**os.environ, "E2E_BROWSER_LOCK_TIMEOUT": str(timeout)},
                              capture_output=True, text=True, timeout=8)

    def test_free_lock_acquires_immediately(self):
        """目的：空闲锁立即获取；前置临时目录，验证0秒等待也可成功且目录存在；上下文清理锁。"""
        with tempfile.TemporaryDirectory() as directory:
            lock = Path(directory) / "browser.lock"
            result = self.acquire(lock, 0)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertTrue(lock.is_dir())

    def test_busy_lock_waits_for_owner_release(self):
        """目的：竞争运行等待原持有者完成；前置真实子进程占锁至测试释放，验证排队日志和成功接管；子进程及目录自动清理。"""
        with tempfile.TemporaryDirectory() as directory:
            lock = Path(directory) / "browser.lock"
            lock.mkdir()
            with subprocess.Popen(["bash", "-c", 'source "$1"; acquire_browser_lock "$2"',
                                   "lock-test", str(SCRIPT), str(lock)],
                                  env={**os.environ, "E2E_BROWSER_LOCK_TIMEOUT": "5"},
                                  stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True) as waiter:
                self.assertIn("waiting up to 5s", waiter.stdout.readline())
                self.assertIsNone(waiter.poll())
                lock.rmdir()
                output, error = waiter.communicate(timeout=8)
                self.assertEqual(waiter.returncode, 0, error)
                self.assertIn("Browser lock acquired", output)
                self.assertTrue(lock.is_dir())

    def test_timeout_preserves_owner_lock_and_evidence(self):
        """目的：超时不删除他人的锁和证据；前置占用锁及标记，验证0与1秒失败和保留标记；临时目录清理。"""
        for timeout in [0, 1]:
            with self.subTest(timeout=timeout), tempfile.TemporaryDirectory() as directory:
                lock = Path(directory) / "browser.lock"
                lock.mkdir()
                marker = lock / "owner"
                marker.write_text("owner evidence")
                result = self.acquire(lock, timeout)
                self.assertEqual(result.returncode, 1)
                self.assertIn("wait timed out", result.stderr)
                self.assertEqual(marker.read_text(), "owner evidence")

    def test_invalid_configuration_and_uncreatable_path_fail(self):
        """目的：配置和路径错误明确失败；前置非法超时或不存在的父目录，验证不创建锁；上下文清理。"""
        with tempfile.TemporaryDirectory() as directory:
            lock = Path(directory) / "browser.lock"
            for timeout in ["-1", "abc", "1.5", "1000000"]:
                with self.subTest(timeout=timeout):
                    result = self.acquire(lock, timeout)
                    self.assertEqual(result.returncode, 1)
                    self.assertIn("non-negative integer", result.stderr)
                    self.assertFalse(lock.exists())
            result = self.acquire(Path(directory) / "missing" / "browser.lock", 5)
            self.assertEqual(result.returncode, 1)
            self.assertIn("Cannot acquire", result.stderr)


if __name__ == "__main__":
    unittest.main()
