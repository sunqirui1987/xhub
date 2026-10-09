"""验证覆盖审计不会把单元覆盖、重复覆盖块或缺失证据误算成后台覆盖。"""
import importlib.util
import tempfile
import unittest
from pathlib import Path

spec = importlib.util.spec_from_file_location("internal_coverage", Path(__file__).with_name("internal-coverage.py"))
coverage = importlib.util.module_from_spec(spec)
spec.loader.exec_module(coverage)


class InternalCoverageTests(unittest.TestCase):
    def test_layer_separation_and_duplicates(self):
        """前置重复覆盖块和单测独有命中；验证去重且两层分开；临时报告在退出时清理。"""
        filename = coverage.MODULE + "internal/example/logic.go"
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / "profile"
            path.write_text(f"mode: set\n{filename}:1.1,2.2 2 0\n{filename}:1.1,2.2 2 1\n{filename}:3.1,4.2 3 0\n")
            regression = coverage.read_profile(path)
            unit = {(filename, "3.1,4.2"): (3, True)}
            rows = coverage.summarize(regression, unit, [filename.rsplit("/", 1)[0]])
            row = next(iter(rows.values()))
            self.assertEqual((row["statements"], row["regression"], row["combined"]), (5, 2, 5))
            self.assertEqual(row["uncovered_blocks"][0]["unit_covered"], True)

    def test_revision_mismatch_is_rejected(self):
        """前置同源码块语句数不同；验证拒绝混合版本证据；仅内存数据，无需清理。"""
        key = (coverage.MODULE + "internal/example/logic.go", "1.1,2.2")
        with self.assertRaisesRegex(ValueError, "different source revisions"):
            coverage.summarize({key: (2, True)}, {key: (3, True)}, [])

    def test_missing_package_is_visible(self):
        """前置生产包没有覆盖块；验证仍列出该包且不算通过；内存数据，无需清理。"""
        name = coverage.MODULE + "internal/empty"
        row = coverage.summarize({}, {}, [name])[name]
        self.assertEqual(row["statements"], 0)
        self.assertEqual(row["regression"], 0)

    def test_invalid_evidence_fails(self):
        """前置坏模式和重复块冲突；验证报错而非输出绿色报告；临时文件自动清理。"""
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / "profile"
            path.write_text("mode: wrong\n")
            with self.assertRaisesRegex(ValueError, "coverage mode"):
                coverage.read_profile(path)
            filename = coverage.MODULE + "internal/example/logic.go"
            path.write_text(f"mode: set\n{filename}:1.1,2.2 2 0\n{filename}:1.1,2.2 3 1\n")
            with self.assertRaisesRegex(ValueError, "inconsistent statement count"):
                coverage.read_profile(path)


if __name__ == "__main__":
    unittest.main()
