import test from "node:test";
import assert from "node:assert/strict";
import { handleSearchKey } from "../src/keyboard.mjs";

/** 创建快捷键测试夹具；参数为按键与焦点状态，返回事件、对话框及计数访问器，单元测试调用；不创建浏览器或文件，无需清理。 */
function fixture(key, activeElement = { tagName: "BODY" }, options = {}) {
  let prevented = 0;
  let opened = 0;
  let closed = 0;
  const event = { key, ...options, preventDefault() { prevented++; } };
  const dialog = { open: Boolean(options.open), close() { closed++; this.open = false; } };
  return { run: () => handleSearchKey(event, activeElement, dialog, () => { opened++; }), counts: () => ({ prevented, opened, closed }) };
}

// 前置为页面焦点或空焦点，验证斜杠和两种平台组合键可打开搜索；夹具无副作用，无需清理。
test("搜索快捷键支持斜杠、Command K 与 Ctrl K", () => {
  for (const setup of [fixture("/"), fixture("/", null), fixture("K", { tagName: "INPUT" }, { metaKey: true }), fixture("k", null, { ctrlKey: true })]) {
    assert.equal(setup.run(), true);
    assert.deepEqual(setup.counts(), { prevented: 1, opened: 1, closed: 0 });
  }
});

// 前置为输入框、可编辑正文或无关按键，验证不会截获正常输入；无 DOM 与业务数据，无需清理。
test("编辑文字时保留斜杠且忽略无关按键", () => {
  for (const setup of [fixture("/", { tagName: "INPUT" }), fixture("/", { tagName: "TEXTAREA" }), fixture("/", { tagName: "SELECT" }), fixture("/", { tagName: "DIV", isContentEditable: true }), fixture("x")]) {
    assert.equal(setup.run(), false);
    assert.deepEqual(setup.counts(), { prevented: 0, opened: 0, closed: 0 });
  }
});

// 前置为打开或关闭的对话框，验证 Escape 仅关闭已打开的搜索且避免重复取消；测试夹具无需清理。
test("Escape 显式关闭搜索并忽略已关闭对话框", () => {
  const setup = fixture("Escape", { tagName: "INPUT" }, { open: true });
  assert.equal(setup.run(), true);
  assert.equal(setup.run(), false);
  assert.deepEqual(setup.counts(), { prevented: 1, opened: 0, closed: 1 });
});
