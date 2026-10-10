"""验收清单、代表选择与浏览器编排测试；无外部调用，临时目录自动清理。"""
import copy
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest import mock
from checklist import Checklist
from real_dataset import Dataset, load_manifest

ROOT = Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location('checklist_runner', ROOT / 'scripts/e2e-real-dataset.py')
runner = importlib.util.module_from_spec(spec)
spec.loader.exec_module(runner)


class ChecklistTests(unittest.TestCase):
    """验证成功、失败、待执行和去重的可观察结果。"""

    def test_runner_import_without_pythonpath(self):
        """目的：用户原始入口无需 PYTHONPATH 即可加载；前置干净环境，验证帮助成功且无导入错误，无外部数据清理。"""
        env = dict(os.environ)
        env.pop('PYTHONPATH', None)
        result = subprocess.run([sys.executable, str(ROOT / 'scripts/e2e-real-dataset.py'), '--help'],
                                env=env, capture_output=True, text=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn('--shared-target', result.stdout)

    def test_results_failure_pending_and_duplicate(self):
        """目的：通过、复验、异常和未执行分别报告；前置四项清单，验证编号/耗时/失败不泄漏正文及重复拒绝，目录自动清理。"""
        with tempfile.TemporaryDirectory() as directory:
            lines = []
            checklist = Checklist(directory, lines.append, [('a', '普通'), ('b', '复验'), ('c', '失败'), ('d', '未执行')])
            with checklist.case('a'):
                pass
            with checklist.case('b') as row:
                row['status'] = 'revalidated'
            with self.assertRaisesRegex(RuntimeError, 'secret'):
                with checklist.case('c'):
                    raise RuntimeError('secret')
            checklist.summary()
            result = json.loads((Path(directory) / 'cases.json').read_text())
            self.assertEqual([row['status'] for row in result['cases']], ['passed', 'revalidated', 'failed', 'pending'])
            self.assertEqual(result['counts']['passed'], 1)
            self.assertNotIn('secret', (Path(directory) / 'cases.json').read_text())
            self.assertIn('[开始 001/004', '\n'.join(lines))
            self.assertTrue((Path(directory) / 'cases.md').exists())
            with self.assertRaises(ValueError):
                with checklist.case('a'):
                    pass
            with self.assertRaises(ValueError):
                Checklist(directory, lines.append, [('same', '一'), ('same', '二')])

    def test_representatives_and_missing_category(self):
        """目的：81钥匙只选4种路由各一把且不改基线；前置重复的内存身份，验证稳定选择、全量兼容及缺分类拒绝，目录自动清理。"""
        with tempfile.TemporaryDirectory() as directory:
            dataset = Dataset('http://127.0.0.1:1', directory, load_manifest(), logger=lambda _: None)
            dataset.state['keys'] = [dict(token_id=str(i), profile=profile, team_index=team)
                for i, (profile, team) in enumerate([('inherit', 0), ('fenno-only', 0), ('qiniu-only', 0),
                                                     ('inherit', 1), ('inherit', 2), ('fenno-only', 1)] * 14)][:81]
            baseline = copy.deepcopy(dataset.state)
            self.assertEqual(len(dataset.acceptance_keys()), 81)
            dataset.representative = True
            self.assertEqual([key['token_id'] for key in dataset.acceptance_keys()], ['0', '1', '2', '3'])
            self.assertEqual(dataset.state, baseline)
            dataset.start_checklist(False)
            self.assertEqual(sum(row['id'].startswith('gpt-') for row in dataset.checklist.rows), 4)
            self.assertFalse(any(row['id'] == 'regression' for row in dataset.checklist.rows))
            dataset.report['checks'] = [dict(name='old', passed=True)]
            self.assertFalse(dataset.checked('old'))
            dataset.state['keys'] = []
            with self.assertRaisesRegex(AssertionError, '代表密钥'):
                dataset.acceptance_keys()

    def test_business_browser_selection_and_failure(self):
        """目的：去重流程必须完整执行且不能沿用旧报告；前置模拟报告，验证19项精确筛选、禁用live、跳过失败和异常传播，临时目录及补丁自动清理。"""
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            result = root / '.e2e/current/results.json'
            result.parent.mkdir(parents=True)
            report = root / 'report'
            report.mkdir()
            stats = dict(expected=19, unexpected=0, flaky=0, skipped=0)
            def execute(args, logger, **kwargs):
                """用途：模拟本轮浏览器产物；参数为命令及环境，返回无；只写临时报告，供完整性断言使用。"""
                self.assertIn('--grep', args)
                self.assertEqual(args[-1].count('$'), 19)
                self.assertNotIn('E2E_LIVE', kwargs['env'])
                current = Path(kwargs['env']['E2E_BROWSER_REPORT_DIR']) / 'results.json'
                current.parent.mkdir(parents=True, exist_ok=True)
                current.write_text(json.dumps(dict(stats=stats)))
            with mock.patch.object(runner, 'ROOT', root), mock.patch.object(runner, 'command_stream', side_effect=execute):
                self.assertEqual(runner.run_business_browser(report, mock.Mock())['cases'], 19)
                stats['skipped'] = 1
                with self.assertRaisesRegex(AssertionError, '未全部执行'):
                    runner.run_business_browser(report, mock.Mock())
            with mock.patch.object(runner, 'ROOT', root), mock.patch.object(runner, 'command_stream'):
                with self.assertRaisesRegex(AssertionError, '缺少本轮'):
                    runner.run_business_browser(report, mock.Mock())
            with mock.patch.object(runner, 'ROOT', root), mock.patch.object(runner, 'command_stream', side_effect=RuntimeError('browser failed')):
                with self.assertRaisesRegex(RuntimeError, 'browser failed'):
                    runner.run_business_browser(report, mock.Mock())

    def test_gpt_unique_requests_and_revalidation(self):
        """目的：每类只执行一个三轮会话，复验与新执行分别计数；前置4把代表和内存回执，验证12个独立请求及重复ID拒绝，目录自动清理。"""
        with tempfile.TemporaryDirectory() as directory:
            dataset = Dataset('http://127.0.0.1:1', directory, load_manifest(), logger=lambda _: None)
            dataset.representative = True
            dataset.state['keys'] = [dict(token_id=str(i), profile=profile, team_index=team)
                for i, (profile, team) in enumerate([('inherit', 0), ('inherit', 1), ('fenno-only', 0), ('qiniu-only', 0)])]
            dataset.start_checklist(False)
            def conversation(key, revalidate=False):
                """用途：返回代表会话证据；参数为身份和复验开关，返回三轮回执；第一把复验，其他新执行，无外部副作用。"""
                if revalidate and key['token_id'] != '0':
                    return None
                return dict(turns=[dict(call_id=key['token_id'] + '-' + str(i)) for i in range(3)])
            dataset.codex_conversation = mock.Mock(side_effect=conversation)
            dataset.verify_codex_agents()
            check = dataset.report['checks'][-1]
            self.assertEqual((check['sessions'], check['requests'], check['executed_this_run']), (4, 12, 3))
            self.assertEqual(dataset.checklist.rows[1]['status'], 'revalidated')
            dataset.checklist = None
            dataset.codex_conversation.return_value = dict(turns=[dict(call_id='same')] * 3)
            dataset.codex_conversation.side_effect = None
            with self.assertRaisesRegex(AssertionError, '复用了请求 ID'):
                dataset.verify_codex_agents()

    def test_verify_uses_only_representative_chat_and_routing(self):
        """目的：完整编排的Chat和继承只检查4个代表且保留81基线；前置内存接口/全部旧账单，验证每类型一次、复验状态和错误层级拒绝，目录自动清理。"""
        with tempfile.TemporaryDirectory() as directory:
            dataset = Dataset('http://127.0.0.1:1', directory, load_manifest(), logger=lambda _: None)
            dataset.representative = True
            categories = [('inherit', 0), ('inherit', 1), ('fenno-only', 0), ('qiniu-only', 0)]
            dataset.state['keys'] = [dict(token_id=str(i), profile=categories[i % 4][0],
                                          team_index=categories[i % 4][1]) for i in range(81)]
            for name, count in [('organizations', 3), ('teams', 9), ('users', 27), ('projects', 9)]:
                dataset.state[name] = [dict(id=str(i)) for i in range(count)]
            dataset.report['calls'] = [dict(key_id=str(i)) for i in range(81)]
            def api(route, *args, **kwargs):
                """用途：模拟成员和有效路由接口；参数为路径，返回业务响应；仅验证编排调用，无网络副作用。"""
                if route.startswith('/team/member_list'):
                    return dict(members=[{}, {}, {}]), {}
                index = int(route.rsplit('=', 1)[-1])
                return dict(effective=dict(scope_type=['organization', 'team', 'key', 'key'][index % 4])), {}
            dataset.api = mock.Mock(side_effect=api)
            dataset.call_checkpoint = mock.Mock(return_value=dict(call_id='saved'))
            dataset.call = mock.Mock(return_value=dict(error='guardrail_failed'))
            helpers = ['verify_acceptance_models', 'verify_permissions', 'verify_xgo', 'verify_limits',
                       'verify_model_lifecycle', 'verify_guardrail_lifecycle']
            for name in helpers:
                setattr(dataset, name, mock.Mock())
            dataset.start_checklist(False)
            dataset.verify()
            self.assertEqual(dataset.call_checkpoint.call_count, 4)
            self.assertEqual(sum(call.args[0].startswith('/route_template') for call in dataset.api.call_args_list), 4)
            self.assertEqual(len(dataset.state['keys']), 81)
            self.assertEqual(len(dataset.report['calls']), 81)
            check = next(row for row in dataset.report['checks'] if row['name'] == 'representative-keys-real-calls-and-billing')
            self.assertEqual((check['keys'], check['executed_this_run'], check['resumed']), (4, 0, 4))
            for name in helpers:
                getattr(dataset, name).assert_called_once()
            dataset.checklist = None
            dataset.api = mock.Mock(side_effect=[(dict(members=[{}, {}, {}]), {})] * 9 +
                                    [(dict(effective=dict(scope_type='wrong')), {})])
            with self.assertRaisesRegex(AssertionError, '路由继承层级错误'):
                dataset.verify()
