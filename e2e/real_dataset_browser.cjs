/** 用途：浏览器操作真实数据集；环境传入 UI/网关/私有目录，输出脱敏报告与截图。
 * 调用：e2e-real-dataset.py；异常保留报告，finally 清理临时护栏及浏览器，响应来自真实供应商。 */
const fs = require('fs');
const path = require('path');
const {chromium, expect} = require('../frontend/node_modules/@playwright/test');
const dir = process.env.E2E_DATASET_DIR;
const access = JSON.parse(fs.readFileSync(path.join(dir, 'access.json')));
const data = JSON.parse(fs.readFileSync(path.join(__dirname, '../docs/testdata/real-acceptance/dataset.json')));
const report = {checks: [], status: 'running'};
let browser;
let stepNumber = 0;

/** 用途：立即输出不含凭据的浏览器步骤；参数为步骤正文，无返回值；由 Python runner 同步保存到 e2e.log。 */
function log(message) {
  console.log('[浏览器 ' + String(++stepNumber).padStart(2, '0') + '] ' + message);
}

/** 用途：执行登录、租户展示、个人密钥请求、账单和护栏流程；无参数，返回 Promise；不拦截真实网络。 */
async function main() {
  log('启动无头 Chromium，视口=1440x1000');
  browser = await chromium.launch({headless: true});
  const context = await browser.newContext({viewport: {width: 1440, height: 1000}});
  // 实际页面状态关闭流式，使 JSON 用量和真实响应可以直接核对。
  await context.addInitScript(() => sessionStorage.setItem('streamingEnabled', 'false'));
  const page = await context.newPage();
  const errors = [];
  page.on('pageerror', e => errors.push(e.message));
  const ui = process.env.E2E_DATASET_UI;
  log('打开登录页并使用管理员真实登录');
  await page.goto(ui + '/ui/login/');
  await page.getByRole('textbox', {name: '用户名', exact: true}).fill('admin');
  await page.locator('input[type=password]').fill(process.env.E2E_DATASET_PASSWORD);
  await page.locator('input[type=password]').press('Enter');
  await expect(page.getByTestId('create-key-button')).toBeVisible({timeout: 60000});
  log('管理员登录通过，导航到组织页面');
  await page.goto(ui + '/ui/organizations/');
  for (const org of data.organizations) await expect(page.getByText(org.name, {exact: true}).first()).toBeVisible({timeout: 30000});
  report.checks.push({name: 'browser-real-organizations', passed: true});
  await page.screenshot({path: path.join(dir, 'organizations.png'), fullPage: true});
  log('3 个真实组织可见，截图=organizations.png');
  log('打开 Playground，选择企业智能助手和数据集个人密钥');
  await page.goto(ui + '/ui/playground/');
  const picker = page.getByPlaceholder('选择模型', {exact: true});
  await picker.click();
  await picker.fill(data.routing.alias);
  await page.getByRole('option', {name: data.routing.alias, exact: true}).click();
  await page.getByText('连接设置', {exact: true}).click();
  await page.getByRole('combobox', {name: '虚拟密钥源', exact: true}).click();
  await page.getByRole('option', {name: '虚拟密钥', exact: true}).click();
  const key = access.keys[1];
  await page.getByPlaceholder('输入自定义虚拟密钥', {exact: true}).fill(key.key);
  await page.getByTestId('chat-composer-input').fill('Reply only OK. browser-real-' + Date.now());
  const pending = page.waitForResponse(r => new URL(r.url()).pathname.endsWith('/chat/completions') && r.request().method() === 'POST', {timeout: 110000});
  await page.getByTestId('chat-send-button').click();
  const response = await pending;
  log('Playground 真实请求返回，HTTP=' + response.status());
  expect(response.status(), '个人密钥真实请求').toBe(200);
  const answer = await response.json();
  expect(answer.choices[0].message.content.length).toBeGreaterThan(0);
  await expect(page.getByTestId('message-surface').last()).toContainText(answer.choices[0].message.content, {timeout: 30000});
  const callId = response.headers()['x-litellm-call-id'];
  expect(callId).toBeTruthy();
  let bill;
  await expect.poll(async () => {
    const res = await page.request.get(process.env.E2E_DATASET_GATEWAY + '/spend/logs/ui/' + callId, {headers: {Authorization: 'Bearer ' + access.admin}});
    if (!res.ok()) return 0;
    bill = await res.json();
    return Number(bill.spend);
  }, {timeout: 90000}).toBeGreaterThan(0);
  log('Playground 请求账单已落库，开始核对五级归属、token 和金额');
  for (const [field, expected] of [['api_key', key.token_id], ['user', key.user_id], ['team_id', key.team_id], ['project_id', key.project_id], ['organization_id', key.organization_id]]) expect(bill[field], field).toBe(expected);
  for (const field of ['prompt_tokens', 'completion_tokens']) expect(bill[field], '真实响应计量 ' + field).toBe(answer.usage[field]);
  expect(Number(bill.spend)).toBeCloseTo(bill.prompt_tokens * data.billing.input_cost_per_token + bill.completion_tokens * data.billing.output_cost_per_token, 9);
  report.checks.push({name: 'browser-real-request-five-owner-bill', call_id: callId, cost: bill.spend, passed: true});
  await page.screenshot({path: path.join(dir, 'playground.png'), fullPage: true});
  log('Playground 五级账单通过，输入=' + bill.prompt_tokens + ' 输出=' + bill.completion_tokens + ' 金额=' + bill.spend + '，截图=playground.png');
  log('打开请求日志页面，核对真实模型记录');
  await page.goto(ui + '/ui/logs/');
  await expect(page.getByText(data.routing.alias, {exact: true}).first()).toBeVisible({timeout: 30000});
  report.checks.push({name: 'browser-request-log', passed: true});
  log('请求日志页面通过');
  log('打开模型广场，核对模型广场和本地模型页签');
  await page.goto(ui + '/ui/price-data/');
  await expect(page.getByRole('heading', {name: 'AI 大模型广场'})).toBeVisible({timeout: 30000});
  await expect(page.getByRole('tab', {name: '模型广场', exact: true})).toBeVisible();
  await expect(page.getByRole('tab', {name: '本地模型列表', exact: true})).toBeVisible();
  report.checks.push({name: 'browser-model-marketplace', passed: true});
  await page.screenshot({path: path.join(dir, 'model-marketplace.png'), fullPage: true});
  log('模型广场通过，截图=model-marketplace.png');
  log('打开护栏监控，核对总评估、拦截数量、详情和日志');
  await page.goto(ui + '/ui/guardrails-monitor/');
  await expect(page.getByRole('heading', {name: '护栏监控', exact: true})).toBeVisible({timeout: 30000});
  await expect(page.getByText('XGo-敏感信息拦截', {exact: true})).toBeVisible({timeout: 30000});
  const monitor = await page.request.get(process.env.E2E_DATASET_GATEWAY + '/guardrails/usage/overview', {headers: {Authorization: 'Bearer ' + access.admin}});
  expect(monitor.status()).toBe(200);
  const overview = await monitor.json();
  expect(overview.totalRequests).toBeGreaterThan(0);
  expect(overview.totalBlocked).toBeGreaterThan(0);
  await page.getByText('XGo-敏感信息拦截', {exact: true}).click();
  await expect(page.getByRole('tab', {name: /日志|Logs/})).toBeVisible();
  await page.getByRole('tab', {name: /日志|Logs/}).click();
  await expect(page.getByText(/拦截|Blocked/).first()).toBeVisible();
  report.checks.push({name: 'browser-persisted-guardrail-monitor-overview-detail-logs', evaluations: overview.totalRequests, passed: true});
  await page.screenshot({path: path.join(dir, 'guardrails-monitor.png'), fullPage: true});
  log('护栏监控通过，评估数=' + overview.totalRequests + ' 拦截数=' + overview.totalBlocked + '，截图=guardrails-monitor.png');
  const guardName = '浏览器验收临时护栏';
  try {
    log('通过页面创建临时关键词护栏并执行调试');
    await page.goto(ui + '/ui/guardrails/');
    await page.getByRole('tab', {name: '护栏', exact: true}).click();
    await page.getByRole('button', {name: '添加护栏', exact: true}).click();
    await page.getByRole('menuitem', {name: '关键词 / 正则护栏', exact: true}).click();
    const dialog = page.getByRole('dialog');
    await dialog.getByLabel('护栏名称', {exact: true}).fill(guardName);
    await dialog.getByLabel('关键词（每行一个）', {exact: true}).fill('浏览器禁止外发');
    await dialog.getByLabel('测试文本', {exact: true}).fill('浏览器禁止外发');
    const trial = page.waitForResponse(r => new URL(r.url()).pathname.endsWith('/guardrails/apply_guardrail'));
    await dialog.getByRole('button', {name: '测试当前规则', exact: true}).click();
    expect((await (await trial).json()).action).toBe('block');
    const saved = page.waitForResponse(r => new URL(r.url()).pathname.endsWith('/guardrails') && r.request().method() === 'POST');
    await dialog.getByRole('button', {name: '保存护栏', exact: true}).click();
    expect((await saved).status()).toBe(200);
    await expect(dialog).not.toBeVisible();
    report.checks.push({name: 'browser-guardrail-create-debug-save', passed: true});
    log('临时护栏调试、保存通过');
  } finally {
    const headers = {Authorization: 'Bearer ' + access.admin};
    const res = await page.request.get(process.env.E2E_DATASET_GATEWAY + '/guardrails/list', {headers});
    if (res.ok()) for (const row of (await res.json()).guardrails || []) if (row.guardrail_name === guardName) await page.request.delete(process.env.E2E_DATASET_GATEWAY + '/guardrails/' + row.guardrail_id, {headers});
    log('临时浏览器护栏清理完成');
  }
  // 每种身份使用独立浏览器上下文，确保没有沿用管理员会话或本地状态。
  for (const persona of access.personas) {
    log('角色=' + persona.role + '：创建独立上下文，验证登录、团队范围和编辑权限');
    const roleContext = await browser.newContext();
    try {
      const rolePage = await roleContext.newPage();
      await rolePage.goto(ui + '/ui/login/');
      await rolePage.getByRole('textbox', {name: '用户名', exact: true}).fill(persona.email);
      await rolePage.locator('input[type=password]').fill(persona.role === 'platform_admin' ? process.env.E2E_DATASET_PASSWORD : access.member_password);
      const loginResponse = rolePage.waitForResponse(r => new URL(r.url()).pathname.endsWith('/v2/login') && r.request().method() === 'POST');
      await rolePage.locator('input[type=password]').press('Enter');
      const login = await loginResponse;
      expect(login.status(), persona.role + '真实登录').toBe(200);
      const token = (await login.json()).key;
      await expect(rolePage.getByTestId('create-key-button')).toBeVisible({timeout: 60000});
      await rolePage.goto(ui + '/ui/teams/');
      await expect(rolePage.getByText('后端', {exact: true}).first()).toBeVisible({timeout: 30000});
      const headers = {Authorization: 'Bearer ' + token};
      const list = await rolePage.request.get(process.env.E2E_DATASET_GATEWAY + '/team/list', {headers});
      expect((await list.json()).length, persona.role + '团队隔离').toBe(persona.visible_teams);
      const update = await rolePage.request.post(process.env.E2E_DATASET_GATEWAY + '/team/update', {
        headers, data: {team_id: persona.team_id, team_description: '浏览器权限验收'}});
      expect(update.status(), persona.role + '团队编辑权限').toBe(persona.role === 'member' ? 403 : 200);
      await rolePage.screenshot({path: path.join(dir, 'role-' + persona.role + '.png'), fullPage: true});
      report.checks.push({name: 'browser-role-' + persona.role, passed: true});
      log('角色=' + persona.role + '：通过，可见团队=' + persona.visible_teams + '，截图=role-' + persona.role + '.png');
    } finally { await roleContext.close(); }
  }
  expect(errors, '浏览器运行异常').toEqual([]);
  log('页面异常数=0，全部浏览器场景通过');
  report.status = 'passed';
}

main().catch(e => {report.status = 'failed'; report.error = e.message; console.error('[浏览器失败] ' + e.stack); process.exitCode = 1;}).finally(async () => {
  if (report.status === 'failed' && browser) for (const context of browser.contexts()) for (const page of context.pages()) {
    await page.screenshot({path: path.join(dir, 'browser-failure.png'), fullPage: true}).catch(() => {});
    fs.writeFileSync(path.join(dir, 'browser-failure.txt'), await page.locator('body').innerText().catch(() => 'page closed'));
  }
  if (browser) await browser.close();
  fs.writeFileSync(path.join(dir, 'browser-report.json'), JSON.stringify(report, null, 2));
  console.log('[浏览器结果] 状态=' + report.status + '，检查数=' + report.checks.length + '，报告=' + path.join(dir, 'browser-report.json'));
});
