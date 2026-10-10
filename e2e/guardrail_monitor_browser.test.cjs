const {test} = require('node:test');
const assert = require('node:assert/strict');
const {requireBlockedGuardrail, verifyGuardrailMonitor} = require('./guardrail_monitor_browser.cjs');
const name = 'XGo-敏感信息拦截';
const row = {id: 'guardrail-block', name, requestsEvaluated: 5, failRate: 100};
const overview = {totalRequests: 9, totalBlocked: 5, rows: [row]};

/** 目的：正常中文名称与真实非零统计返回精确行；前置固定总览输入，验证不修改输入，无业务数据需要清理。 */
test('返回具有真实拦截统计的目标护栏', () => {
  assert.equal(requireBlockedGuardrail(overview, name), row);
});

/** 目的：最小一次评估与极小非零拦截率仍有效；前置边界统计，验证无需任意次数门槛，无外部副作用。 */
test('一次评估的边界统计可以通过', () => {
  const boundary = {...row, requestsEvaluated: 1, failRate: 0.01};
  assert.equal(requireBlockedGuardrail({totalRequests: 1, totalBlocked: 1, rows: [boundary]}, name), boundary);
});

/** 目的：空窗口、缺失或重名目标不能被其他护栏替代；前置非法行集合，验证明确中文错误，无清理数据。 */
test('目标护栏必须存在且唯一', () => {
  for (const rows of [undefined, [], [{...row, name: '其他护栏'}], [row, {...row, id: 'duplicate'}]]) {
    assert.throws(() => requireBlockedGuardrail({...overview, rows}, name), /唯一目标护栏/);
  }
});

/** 目的：缺少 ID、零评估、零拦截率和不一致总计均不能通过；前置异常统计，验证失败说明，无持久化副作用。 */
test('拒绝缺失或不一致的真实拦截统计', () => {
  for (const patch of [{id: ''}, {requestsEvaluated: 0}, {requestsEvaluated: -1}, {failRate: 0}]) {
    assert.throws(() => requireBlockedGuardrail({...overview, rows: [{...row, ...patch}]}, name), /真实拦截统计/);
  }
  for (const patch of [{totalRequests: 0}, {totalBlocked: 0}]) {
    assert.throws(() => requireBlockedGuardrail({...overview, ...patch}, name), /真实拦截统计/);
  }
});

/** 目的：接口失败时在定位页面前终止，避免把后台错误伪装成元素超时；前置内存页面边界，验证监听先于导航和错误说明，无外部数据需清理。 */
test('浏览器校验先监听总览并明确报告接口错误', async () => {
  const calls = [];
  const page = {
    /** 用途：记录响应订阅并核对筛选范围；参数为筛选器和等待配置，返回失败 HTTP 响应，供错误路径单测，无外部副作用。 */
    waitForResponse(predicate, options) {
      calls.push('listen');
      assert.equal(options.timeout, 1234);
      const response = {
        url: () => 'http://localhost/guardrails/usage/overview?start_date=2026-10-10',
        request: () => ({method: () => 'GET'}),
        status: () => 503,
      };
      assert.equal(predicate(response), true);
      assert.equal(predicate({...response, request: () => ({method: () => 'POST'})}), false);
      assert.equal(predicate({...response, url: () => 'http://localhost/guardrails/list'}), false);
      return Promise.resolve(response);
    },
    /** 用途：记录待导航地址；参数为 URL，返回导航完成，供顺序校验，无浏览器或持久化副作用。 */
    async goto(url) {
      calls.push('goto');
      assert.equal(url, 'http://localhost/ui/guardrails-monitor/');
    },
  };
  await assert.rejects(verifyGuardrailMonitor(page, 'http://localhost', name, 1234), /护栏监控总览接口/);
  assert.deepEqual(calls, ['listen', 'goto']);
});

/** 目的：空时间窗与响应超时均直接传播明确失败，不继续点击其他护栏；前置内存响应边界，验证错误原因，无外部状态需清理。 */
test('浏览器校验拒绝空时间窗并传播响应超时', async () => {
  for (const result of [
    Promise.resolve({status: () => 200, json: async () => ({rows: [], totalRequests: 0, totalBlocked: 0})}),
    Promise.reject(new Error('总览响应超时')),
  ]) {
    const page = {
      /** 用途：返回指定响应或超时；无参数，供浏览器校验边界单测，无网络和数据副作用。 */
      waitForResponse() { return result; },
      /** 用途：模拟导航完成；无参数和返回数据，供失败路径单测，无外部副作用。 */
      async goto() {},
    };
    await assert.rejects(verifyGuardrailMonitor(page, 'http://localhost', name), /唯一目标护栏|总览响应超时/);
  }
});
