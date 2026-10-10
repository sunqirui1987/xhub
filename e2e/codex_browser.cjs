const path = require('path');
const {expect} = require('../frontend/node_modules/@playwright/test');

/** 用途：按传入顺序验证 Codex 会话三轮日志的真实接口和浏览器详情。
 * 参数为页面、UI/网关地址、管理员令牌、有序会话及可选截图目录；返回脱敏检查数组。
 * 保留数据集与隔离 Playwright 共用，错误抛断言；确认文本会话没有视频媒体卡片，只读取账单，截图写入指定报告目录。 */
async function verifyCodexLogs(page, ui, gateway, admin, sessions, directory) {
  const checks = [];
  for (const session of sessions) {
    expect(session, '模型必须有完整 Codex 会话').toBeTruthy();
    expect(session.agent_type).toBe('codex');
    expect(session.turns).toHaveLength(3);
    expect(new Set(session.turns.map(turn => turn.call_id)).size).toBe(3);
    expect(new Set(session.turns.map(turn => turn.response_id)).size).toBe(3);
    let previous = null;
    for (const turn of session.turns) {
      expect(turn.previous_response_id, '三轮响应链应连续').toBe(previous);
      const detail = await page.request.get(gateway + '/spend/logs/ui/' + turn.call_id, {
        headers: {Authorization: 'Bearer ' + admin},
      });
      expect(detail.status(), 'Codex 精确账单接口').toBe(200);
      const bill = await detail.json();
      expect(bill.session_id).toBe(session.session_id);
      expect(bill.api_key).toBe(session.key_id);
      expect(bill.model).toBe(session.model);
      expect(bill.response.id).toBe(turn.response_id);
      expect(bill.response.status).toBe('completed');
      expect(bill.prompt_tokens).toBe(turn.prompt_tokens);
      expect(bill.completion_tokens).toBe(turn.completion_tokens);
      expect(bill.response.usage.input_tokens).toBe(turn.prompt_tokens);
      expect(bill.response.usage.output_tokens).toBe(turn.completion_tokens);
      expect(Number(bill.spend)).toBeCloseTo(turn.cost, 9);
      await page.goto(ui + '/ui/logs/?log_id=' + encodeURIComponent(turn.call_id));
      await expect(page.getByText(turn.prompt, {exact: true}).first()).toBeVisible({timeout: 30000});
      // 后续轮次的折叠 HISTORY 中也有同一标记；只检查当前可见回答，避免选中隐藏的历史消息。
      await expect(page.getByText(session.marker, {exact: true}).filter({visible: true})).toHaveCount(1);
      await expect(page.getByTestId('media-request'), 'Codex 文本日志不能显示为视频').toHaveCount(0);
      previous = turn.response_id;
    }
    if (directory) {
      const slug = session.model.replace(/[^a-z0-9]+/gi, '-').toLowerCase();
      await page.screenshot({path: path.join(directory, 'codex-' + slug + '.png'), fullPage: true});
    }
    checks.push({name: 'browser-codex-multi-turn', model: session.model, agent_type: 'codex',
      session_id: session.session_id, requests: 3, passed: true});
  }
  return checks;
}

module.exports = {verifyCodexLogs};
