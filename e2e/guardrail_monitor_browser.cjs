const {expect} = require('../frontend/node_modules/@playwright/test');

/** 用途：校验监控接口中目标护栏的真实拦截证据；参数为总览 JSON 和精确名称，返回目标行。
 * 调用：浏览器验收及单元测试；空窗口、重名、缺失目标或只有其他护栏拦截时抛错，无持久化副作用。 */
function requireBlockedGuardrail(overview, name) {
  const rows = (overview.rows || []).filter(row => row.name === name);
  if (rows.length !== 1) throw new Error('护栏监控必须包含唯一目标护栏：' + name);
  const row = rows[0];
  if (!row.id || !(row.requestsEvaluated > 0) || !(row.failRate > 0) ||
      !(overview.totalRequests >= row.requestsEvaluated) || !(overview.totalBlocked > 0)) {
    throw new Error('护栏监控缺少目标护栏的真实拦截统计：' + name);
  }
  return row;
}

/** 用途：从真实监控页面打开指定护栏并核对同一时间窗的总览、详情和拦截日志。
 * 参数：page 为已登录页面，ui 为控制台地址，name 为精确护栏名，timeout 为接口和页面等待毫秒数。
 * 返回：包含评估数、拦截数和目标 ID 的验收证据，供真实数据集与离线 E2E 共用。
 * 边界：标题先于异步数据出现，必须在导航前监听接口，再通过可访问按钮选择；HTTP 错误或证据不符直接失败。
 * 副作用：只导航和切换标签，不调用供应商、不创建或删除业务数据。 */
async function verifyGuardrailMonitor(page, ui, name, timeout = 60000) {
  const overviewPending = page.waitForResponse(response =>
    new URL(response.url()).pathname.endsWith('/guardrails/usage/overview') &&
    response.request().method() === 'GET', {timeout});
  // 导航失败时仍消费监听器的拒绝，避免悬挂 Promise 产生未处理异常。
  overviewPending.catch(() => {});
  await page.goto(ui + '/ui/guardrails-monitor/');
  const overviewResponse = await overviewPending;
  expect(overviewResponse.status(), '护栏监控总览接口').toBe(200);
  const overview = await overviewResponse.json();
  const row = requireBlockedGuardrail(overview, name);
  const button = page.getByRole('button', {name, exact: true});
  await expect(button, '监控列表中的目标护栏按钮').toBeVisible({timeout});

  const detailPending = page.waitForResponse(response =>
    new URL(response.url()).pathname.endsWith('/guardrails/usage/detail/' + row.id) &&
    response.request().method() === 'GET', {timeout});
  const logsPending = page.waitForResponse(response => {
    const url = new URL(response.url());
    return url.pathname.endsWith('/guardrails/usage/logs') &&
      url.searchParams.get('guardrail_id') === row.id && response.request().method() === 'GET';
  }, {timeout});
  detailPending.catch(() => {});
  logsPending.catch(() => {});
  await button.click();
  const detailResponse = await detailPending;
  expect(detailResponse.status(), '目标护栏详情接口').toBe(200);
  const detail = await detailResponse.json();
  expect(detail.guardrail_name, '详情应属于选中的护栏').toBe(name);
  expect(detail.requestsEvaluated, '详情与页面总览应使用同一时间窗').toBe(row.requestsEvaluated);
  await expect(page.getByRole('heading', {name, exact: true})).toBeVisible({timeout});
  const logsTab = page.getByRole('tab', {name: /^(日志|Logs)$/, exact: true});
  await logsTab.click();
  const logsResponse = await logsPending;
  expect(logsResponse.status(), '目标护栏日志接口').toBe(200);
  const logs = await logsResponse.json();
  const blocked = logs.logs.find(log => log.guardrail_name === name && log.action === 'blocked');
  expect(blocked, '目标护栏必须有真实拦截日志').toBeTruthy();
  expect(typeof blocked.reason === 'string' && blocked.reason.trim().length > 0, '真实拦截日志必须包含非空原因').toBe(true);
  await expect(page.getByRole('button').filter({hasText: blocked.reason}).first(), '真实拦截日志原因应在页面可见').toBeVisible({timeout});
  return {evaluations: overview.totalRequests, blocked: overview.totalBlocked, guardrail_id: row.id};
}

module.exports = {requireBlockedGuardrail, verifyGuardrailMonitor};
