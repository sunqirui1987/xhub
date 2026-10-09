"""续接测试供应商的确定性单元测试，无外部数据。"""
import unittest
from fake_upstream import continuation_answer


class ContinuationTests(unittest.TestCase):
    def test_complete_history(self):
        """前置完整三轮历史，验证回复反映实际轮数；仅内存，无需清理。"""
        messages = []
        for index in range(1, 4):
            messages.append({"role": "user", "content": f"continuation-{index}"})
            self.assertEqual(continuation_answer({"messages": messages}), f"history-ok-{index}")
            messages.append({"role": "assistant", "content": f"history-ok-{index}"})

    def test_missing_history(self):
        """前置仅本轮消息或空历史，验证不会伪装成成功；仅内存，无需清理。"""
        for messages in ([], [{"role": "user", "content": "continuation-2"}]):
            with self.assertRaises(ValueError):
                continuation_answer({"messages": messages})

    def test_proxy_fields(self):
        """前置合法消息但携带代理续接字段，验证转换隔离；仅内存，无需清理。"""
        for field in ("previous_response_id", "store"):
            with self.assertRaises(ValueError):
                continuation_answer({"messages": [{"role": "user", "content": "continuation-1"}], field: False})
