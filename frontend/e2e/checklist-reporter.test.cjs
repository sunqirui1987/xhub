const {test} = require('node:test');
const assert = require('node:assert/strict');
const Reporter = require('./checklist-reporter.cjs');
/** 目的：执行前列出完整业务清单且跳过/超时不能显示通过；前置内存测试树，断言编号和全部结果类型，finally 恢复输出，无外部数据。 */
test('逐项显示业务内容并区分通过、失败与未执行', () => {
  const logs = [];
  const original = console.log;
  console.log = message => logs.push(message);
  try {
    const reporter = new Reporter();
    const cases = [{title: '密钥完整生命周期'}, {title: '错误日志详情'}];
    reporter.onBegin({}, {allTests: () => cases});
    reporter.onTestBegin(cases[0]);
    for (const status of ['passed', 'failed', 'timedOut', 'skipped', 'interrupted']) {
      reporter.onTestEnd(cases[0], {status});
    }
    assert.match(logs[0], /待测 001\/002.*密钥完整生命周期/);
    assert.match(logs[1], /待测 002\/002.*错误日志详情/);
    assert.match(logs[2], /开始 001\/002/);
    for (const [index, status] of ['通过', '失败', '超时', '未执行', '中断'].entries()) {
      assert.match(logs[index + 3], new RegExp('浏览器' + status));
    }
    const empty = new Reporter();
    const count = logs.length;
    empty.onBegin({}, {allTests: () => []});
    assert.equal(logs.length, count);
  } finally { console.log = original; }
});
