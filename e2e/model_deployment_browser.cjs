const {expect} = require('../frontend/node_modules/@playwright/test');

/** 用途：在模型分组中按精确部署 ID 核对一行的连接、上游型号、协议和完整用户入口。
 * 参数：page 为 Playwright 页面，deployment 为真实数据集部署，timeout 为每项等待毫秒数。
 * 返回：核对成功的部署行；供真实验收及离线 E2E 共用，只读页面。
 * 边界：入口必须是非空字符串数组；缺失、重复部署行或任一字段不符时抛错，禁止由同组其他部署补足证据。 */
async function verifyModelDeployment(page, deployment, timeout = 30000) {
  const endpoints = deployment.endpoint_types;
  if (!Array.isArray(endpoints) || endpoints.length === 0 || endpoints.some(value => typeof value !== 'string' || !value.trim())) {
    throw new Error('部署 ' + deployment.id + ' 的用户入口声明必须是非空字符串数组');
  }
  const region = page.getByRole('region', {name: '公开模型 ' + deployment.public_name, exact: true});
  await expect(region, '公开模型 ' + deployment.public_name).toBeVisible({timeout});
  const row = region.getByRole('row').filter({has: page.getByText(deployment.id, {exact: true})});
  await expect(row, '部署 ID ' + deployment.id + ' 必须唯一').toHaveCount(1, {timeout});
  await expect(row, '部署 ' + deployment.id + ' 必须可见').toBeVisible({timeout});
  // 页面将入口按声明顺序合并为一个文本节点；完整匹配可同时发现入口缺失、多出和顺序变化。
  for (const [field, value] of [
    ['供应商连接', deployment.provider], ['上游型号', deployment.model],
    ['上游协议', deployment.transport], ['用户入口', endpoints.join(' · ')],
  ]) {
    await expect(row.getByText(value, {exact: true}), '部署 ' + deployment.id + ' 的' + field).toBeVisible({timeout});
  }
  return row;
}

module.exports = {verifyModelDeployment};
