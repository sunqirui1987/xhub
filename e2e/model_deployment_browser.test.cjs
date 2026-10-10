const {test, before, after} = require('node:test');
const assert = require('node:assert/strict');
const {chromium} = require('../frontend/node_modules/@playwright/test');
const {verifyModelDeployment} = require('./model_deployment_browser.cjs');
let browser;
before(async () => { browser = await chromium.launch({headless: true}); });
after(async () => { await browser?.close(); });
const deployment = {id: 'model-primary', public_name: 'grouped-model', provider: 'primary', model: 'upstream-primary', transport: 'bypass_openai_chat', endpoint_types: ['chat', 'responses']};

/** 用途：构建两个部署的最小语义表格；参数为首行入口文本和是否重复 ID，返回 HTML。
 * 调用场景：定位器单元测试；固定数据无外部请求，第二行专门提供可能混淆的供应商和入口。 */
function markup(endpoints, duplicate = false) {
  return '<section aria-label="公开模型 grouped-model"><table><tbody>' +
    '<tr><td><p>primary</p><p>upstream-primary</p><p>model-primary</p></td><td><p>bypass_openai_chat</p><p>' + endpoints + '</p></td></tr>' +
    '<tr><td><p>secondary</p><p>upstream-secondary</p><p>' + (duplicate ? 'model-primary' : 'model-secondary') + '</p></td><td><p>bypass_openai_chat</p><p>chat · responses</p></td></tr>' +
    '</tbody></table></section>';
}

/** 用途：在独立 DOM 中执行一次核对；参数为 HTML 和预期部署，返回核对完成的 Promise。
 * 前置本地 Chromium，正常或异常均关闭页面；无真实后台及持久化副作用。 */
async function verify(html, expected) {
  const page = await browser.newPage();
  try {
    await page.setContent(html);
    await verifyModelDeployment(page, expected, 150);
  } finally { await page.close(); }
}

// 目的：覆盖多入口、单入口及带标点媒体入口；前置两个部署 DOM，完整文本应通过，每次自动关闭页面。
for (const endpoints of [['chat', 'responses'], ['chat'], ['bypass:openai-images'], ['bypass:ark-video'], ['fal:queue']]) {
  test('完整入口文本可见：' + endpoints.join(' · '), async () => {
    await verify(markup(endpoints.join(' · ')), {...deployment, endpoint_types: endpoints});
  });
}

// 目的：覆盖缺失、多出和顺序错误；前置其他行入口正确，目标行仍须失败，页面自动关闭。
for (const text of ['chat', 'chat · responses · messages', 'responses · chat']) {
  test('不能由其他部署补足错误入口：' + text, async () => {
    await assert.rejects(verify(markup(text), deployment), /model-primary 的用户入口/);
  });
}

/** 目的：同组其他供应商不能代替目标连接；前置第二行具有 secondary，预期核对失败，自动关闭页面。 */
test('供应商必须属于同一部署行', async () => {
  await assert.rejects(verify(markup('chat · responses'), {...deployment, provider: 'secondary'}), /model-primary 的供应商连接/);
});

/** 目的：缺失或重复 ID 不得使用第一条结果蒙混；前置固定 DOM，预期唯一性失败，自动关闭页面。 */
test('部署 ID 必须存在且唯一', async () => {
  await assert.rejects(verify(markup('chat · responses'), {...deployment, id: 'missing'}), /missing 必须唯一/);
  await assert.rejects(verify(markup('chat · responses', true), deployment), /model-primary 必须唯一/);
});

/** 目的：空数组、缺失声明和非法成员不能绕过入口核对；前置无页面对象，预期立即拒绝，无清理数据。 */
test('非法入口声明不能跳过验收', async () => {
  for (const endpoint_types of [[], undefined, 'chat', [''], [null], ['chat', 1]]) {
    await assert.rejects(verifyModelDeployment(null, {...deployment, endpoint_types}), /非空字符串数组/);
  }
});
