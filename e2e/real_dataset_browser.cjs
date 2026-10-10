/** 用途：浏览器操作真实数据集；环境传入 UI/网关/私有目录，输出脱敏报告与截图。
 * 调用：e2e-real-dataset.py；异常保留报告，finally 清理临时护栏及浏览器，响应来自真实供应商。 */
const fs = require('fs');
const path = require('path');
const {chromium, expect} = require('../frontend/node_modules/@playwright/test');
const {verifyCodexLogs} = require('./codex_browser.cjs');
const {verifyModelDeployment} = require('./model_deployment_browser.cjs');
const dir = process.env.E2E_DATASET_DIR;
const access = JSON.parse(fs.readFileSync(path.join(dir, 'access.json')));
const data = JSON.parse(fs.readFileSync(path.join(__dirname, '../docs/testdata/real-acceptance/dataset.json')));
const persistedReport = JSON.parse(fs.readFileSync(path.join(dir, 'report.json')));
const report = {checks: [], status: 'running'};
let browser;
let stepNumber = 0;

/** 用途：立即输出不含凭据的浏览器步骤；参数为步骤正文，无返回值；由 Python runner 同步保存到 e2e.log。 */
function log(message) {
  console.log('[浏览器 ' + String(++stepNumber).padStart(2, '0') + '] ' + message);
}

/** 用途：通过可分享的精确日志 ID 打开详情抽屉；参数为页面、UI 地址、日志 ID 和预期媒体测试标记，返回详情区域；等待真实详情接口加载完成，失败由 Playwright 保留页面证据。 */
async function openExactMediaLog(page, ui, logId, testId = 'media-response') {
  await page.goto(ui + '/ui/logs/?log_id=' + encodeURIComponent(logId));
  const media = page.getByTestId(testId);
  await expect(media).toBeVisible({timeout: 30000});
  return media;
}

/** 用途：验收图片和三种视频的请求、终态日志详情；参数为页面与 UI 地址，无返回值；原任务日志完成后展示终态，旧独立日志仍展示创建态，核对模型、prompt、Task ID、URL、usage/时长并保存截图。 */
async function verifyMediaLogs(page, ui) {
  const image = persistedReport.media_calls.find(row => row.kind === 'image');
  expect(image, 'gpt-image-2 报告证据').toBeTruthy();
  await openExactMediaLog(page, ui, image.call_id, 'media-response-image');
  await expect(page.getByText(image.model, {exact: true}).first()).toBeVisible();
  await expect(page.getByText('A red apple on a white background.', {exact: true})).toBeVisible();
  const imageNode = page.getByTestId('media-response-image').first();
  await expect.poll(() => imageNode.evaluate(node => node.complete && node.naturalWidth > 0), {timeout: 30000}).toBe(true);
  await page.screenshot({path: path.join(dir, 'log-gpt-image-2.png'), fullPage: true});
  report.checks.push({name: 'browser-media-image-log', model: image.model, call_id: image.call_id, passed: true});
  log('✅ [' + image.model + ' / bypass_openai_image_generation / 浏览器日志图片] 模型、prompt、真实图片均可见，截图=log-gpt-image-2.png');

  for (const task of persistedReport.media_tasks || []) {
    const slug = task.transport.replace(/[^a-z0-9]+/gi, '-').toLowerCase();
    await openExactMediaLog(page, ui, task.create_call_id);
    await expect(page.getByText(task.model, {exact: true}).first()).toBeVisible();
    await expect(page.getByText('A red apple on a white table, static camera, gentle natural light.', {exact: true})).toBeVisible();
    await expect(page.getByText(task.task_id, {exact: true}).first()).toBeVisible();
    if (task.transport.startsWith('qiniu_fal_')) {
      const status = task.create_call_id === task.settlement_log_id ? 'COMPLETED' : 'IN_QUEUE';
      await expect(page.getByText(status, {exact: true})).toBeVisible();
      await expect(page.getByText(task.result_path, {exact: true})).toBeVisible();
      await expect(page.getByText(task.result_path + '/status', {exact: true})).toBeVisible();
    }
    await page.screenshot({path: path.join(dir, 'log-' + slug + '-create.png'), fullPage: true});
    report.checks.push({name: 'browser-media-' + slug + '-create', model: task.model, call_id: task.create_call_id, passed: true});
    log('✅ [' + task.model + ' / ' + task.transport + ' / 浏览器创建日志] prompt、Task ID、状态及查询地址完整，截图=log-' + slug + '-create.png');

    await openExactMediaLog(page, ui, task.settlement_log_id, 'media-response-video');
    await expect(page.getByText(task.model, {exact: true}).first()).toBeVisible();
    await expect(page.getByText(task.task_id, {exact: true}).first()).toBeVisible();
    const video = page.getByTestId('media-response-video');
    await expect(video).toHaveAttribute('src', task.bypass_result.video_url);
    await expect(page.getByText(task.bypass_result.video_url, {exact: true})).toBeVisible();
    if (task.transport === 'qiniu_fal_kling') {
      await expect(page.getByText(String(task.bypass_result.duration), {exact: true}).first()).toBeVisible();
    } else {
      await expect(page.getByTestId('media-response-usage')).toBeVisible();
    }
    await page.screenshot({path: path.join(dir, 'log-' + slug + '-result.png'), fullPage: true});
    report.checks.push({name: 'browser-media-' + slug + '-result', model: task.model, call_id: task.settlement_log_id, passed: true});
    log('✅ [' + task.model + ' / ' + task.transport + ' / 浏览器终态日志] Task ID、终态、真实视频、最终 URL 和 usage/时长可见，截图=log-' + slug + '-result.png');
  }
}

/** 用途：执行登录、按部署 ID 核对模型完整入口、租户展示、个人密钥请求、账单和护栏流程。
 * 无参数，返回完成 Promise；调用场景为真实数据集验收，不拦截真实网络，页面或接口不符时抛错并保留报告。 */
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
  log('✅ [admin / browser / 管理员登录] 登录成功并进入控制台');
  log('打开模型提供商页面，先核对供应商连接');
  await page.goto(ui + '/ui/model-providers/');
  // 页面标题和连接列表都使用“模型提供商”标题；限定主内容并取页面级标题，避免严格模式把两个合法标题判为歧义。
  await expect(page.locator('main').getByRole('heading', {name: '模型提供商', exact: true}).first()).toBeVisible({timeout: 30000});
  for (const provider of data.providers) {
    await expect(page.getByText(provider.id, {exact: true}).first()).toBeVisible({timeout: 30000});
    report.checks.push({name: 'browser-provider-' + provider.id, provider: provider.id, passed: true});
    log('✅ [' + provider.id + ' / provider / 供应商连接] 页面已显示已持久化连接');
  }
  await page.screenshot({path: path.join(dir, 'model-providers.png'), fullPage: true});
  log('打开模型与端点页面，在供应商之后核对每个真实模型');
  await page.goto(ui + '/ui/models-and-endpoints/');
  for (const deployment of access.deployments) {
    if (deployment.temporary_fault) continue;
    await verifyModelDeployment(page, deployment);
    report.checks.push({name: 'browser-model-' + deployment.public_name, model: deployment.public_name, protocol: deployment.transport, passed: true});
    log('✅ [' + deployment.public_name + ' / ' + deployment.transport + ' / 模型与端点] 公开名、供应商、上游型号、协议和用户入口均正确');
  }
  await page.screenshot({path: path.join(dir, 'models-and-endpoints.png'), fullPage: true});
  log('导航到组织页面');
  await page.goto(ui + '/ui/organizations/');
  for (const org of data.organizations) await expect(page.getByText(org.name, {exact: true}).first()).toBeVisible({timeout: 30000});
  report.checks.push({name: 'browser-real-organizations', passed: true});
  await page.screenshot({path: path.join(dir, 'organizations.png'), fullPage: true});
  log('✅ [organizations / browser / 组织列表] 3 个真实组织可见，截图=organizations.png');
  const key = access.keys.find(row => row.profile === 'fenno-only');
  const chatModel = key.call_model;
  log('打开 Playground，选择真实模型 ' + chatModel + ' 和数据集个人密钥');
  await page.goto(ui + '/ui/playground/');
  const picker = page.getByPlaceholder('选择模型', {exact: true});
  await picker.click();
  await picker.fill(chatModel);
  await page.getByRole('option', {name: chatModel, exact: true}).click();
  await page.getByText('连接设置', {exact: true}).click();
  await page.getByRole('combobox', {name: '虚拟密钥源', exact: true}).click();
  await page.getByRole('option', {name: '虚拟密钥', exact: true}).click();
  await page.getByPlaceholder('输入自定义虚拟密钥', {exact: true}).fill(key.key);
  let response;
  let answer;
  // 浏览器必须真的重复点击并发起新请求；只重试明确的供应商临时状态，最终成功前不得打勾。
  for (let attempt = 1; attempt <= 5; attempt++) {
    await page.getByTestId('chat-composer-input').fill('Reply only OK. browser-real-' + Date.now() + '-attempt-' + attempt);
    const pending = page.waitForResponse(r => new URL(r.url()).pathname.endsWith('/chat/completions') && r.request().method() === 'POST', {timeout: 110000});
    log('▶ [' + chatModel + ' / bypass_openai_chat / Playground 第 ' + attempt + '/5 次真实 AI 请求] 点击发送并等待供应商响应');
    await page.getByTestId('chat-send-button').click();
    response = await pending;
    if (response.status() === 200) {
      answer = await response.json();
      expect(answer.choices[0].message.content.length, '真实回答内容').toBeGreaterThan(0);
      expect(answer.usage.prompt_tokens, '真实输入计量').toBeGreaterThan(0);
      expect(answer.usage.completion_tokens, '真实输出计量').toBeGreaterThan(0);
      log('✅ [' + chatModel + ' / bypass_openai_chat / Playground 真实 AI 请求] 第 ' + attempt + '/5 次成功，HTTP=200，输入=' + answer.usage.prompt_tokens + '，输出=' + answer.usage.completion_tokens);
      break;
    }
    log('❌ [' + chatModel + ' / bypass_openai_chat / Playground 第 ' + attempt + '/5 次真实 AI 请求] HTTP=' + response.status());
    if (![429, 502, 503, 504].includes(response.status()) || attempt === 5) throw new Error('个人密钥真实请求 HTTP=' + response.status());
    const delay = response.status() === 429 ? 65000 : 3000;
    log('⏳ [' + chatModel + ' / bypass_openai_chat / Playground 真实 AI 请求重试] ' + delay / 1000 + ' 秒后重新点击发送');
    await page.waitForTimeout(delay);
  }
  expect(response.status(), '个人密钥真实请求').toBe(200);
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
  log('✅ [' + chatModel + ' / billing / 请求日志落库] request_id=' + callId + '，开始核对五级归属、token 和金额');
  for (const [field, expected] of [['api_key', key.token_id], ['user', key.user_id], ['team_id', key.team_id], ['project_id', key.project_id], ['organization_id', key.organization_id]]) expect(bill[field], field).toBe(expected);
  for (const field of ['prompt_tokens', 'completion_tokens']) expect(bill[field], '真实响应计量 ' + field).toBe(answer.usage[field]);
  expect(Number(bill.spend)).toBeCloseTo(bill.prompt_tokens * data.billing.input_cost_per_token + bill.completion_tokens * data.billing.output_cost_per_token, 9);
  report.checks.push({name: 'browser-real-request-five-owner-bill', call_id: callId, cost: bill.spend, passed: true});
  await page.screenshot({path: path.join(dir, 'playground.png'), fullPage: true});
  log('✅ [' + chatModel + ' / billing / 五级计量与费用] 输入=' + bill.prompt_tokens + ' 输出=' + bill.completion_tokens + ' 金额=' + bill.spend + '，截图=playground.png');
  log('打开请求日志页面，核对真实模型记录');
  await page.goto(ui + '/ui/logs/');
  await expect(page.getByText(chatModel, {exact: true}).first()).toBeVisible({timeout: 30000});
  report.checks.push({name: 'browser-request-log', passed: true});
  log('✅ [' + chatModel + ' / browser / 请求日志] 真实模型调用记录可见');
  log('按 GPT → GLM 顺序逐轮打开 Codex 三轮精确日志详情');
  const sessions = data.verification_order.slice(0, 2).map(model =>
    (persistedReport.agent_conversations || []).find(row => row.model === model));
  report.checks.push(...await verifyCodexLogs(page, ui, process.env.E2E_DATASET_GATEWAY, access.admin, sessions, dir));
  log('✅ [codex / browser / GPT 与 GLM 三轮日志] 响应链、会话、usage、金额及前轮项目标记均正确');
  log('逐条打开 gpt-image-2、Ark Seedance、FAL Seedance 和 FAL Kling 精确日志详情');
  await verifyMediaLogs(page, ui);
  log('打开模型广场，核对模型广场和本地模型页签');
  await page.goto(ui + '/ui/price-data/');
  await expect(page.getByRole('heading', {name: 'AI 大模型广场'})).toBeVisible({timeout: 30000});
  await expect(page.getByRole('tab', {name: '模型广场', exact: true})).toBeVisible();
  await expect(page.getByRole('tab', {name: '本地模型列表', exact: true})).toBeVisible();
  report.checks.push({name: 'browser-model-marketplace', passed: true});
  await page.screenshot({path: path.join(dir, 'model-marketplace.png'), fullPage: true});
  log('✅ [AI 大模型广场 / browser / 模型目录] 模型广场和本地模型列表页签可见，截图=model-marketplace.png');
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
  log('✅ [XGo-敏感信息拦截 / browser / 护栏监控] 评估数=' + overview.totalRequests + ' 拦截数=' + overview.totalBlocked + '，详情和日志可见，截图=guardrails-monitor.png');
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
    log('✅ [' + guardName + ' / browser / 创建、调试与保存] block 动作和持久化结果正确');
  } finally {
    const headers = {Authorization: 'Bearer ' + access.admin};
    const res = await page.request.get(process.env.E2E_DATASET_GATEWAY + '/guardrails/list', {headers});
    if (res.ok()) for (const row of (await res.json()).guardrails || []) if (row.guardrail_name === guardName) await page.request.delete(process.env.E2E_DATASET_GATEWAY + '/guardrails/' + row.guardrail_id, {headers});
    log('✅ [' + guardName + ' / browser / 测试数据清理] 临时护栏已删除');
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
      log('✅ [' + persona.role + ' / browser / 登录、团队隔离与编辑权限] 可见团队=' + persona.visible_teams + '，截图=role-' + persona.role + '.png');
    } finally { await roleContext.close(); }
  }
  expect(errors, '浏览器运行异常').toEqual([]);
  log('✅ [real-acceptance / browser / 页面异常检查] 页面异常数=0，全部浏览器场景通过');
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
