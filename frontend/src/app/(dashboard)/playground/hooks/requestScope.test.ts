import { describe, it, expect, vi } from "vitest";
import { guardCallbacks } from "./requestScope";
describe("requestScope", () => {
  /** 前置活动请求；验证参数原样传递及返回值，无外部状态，mock 随测试销毁。 */
  it("传递当前请求回调", () => {
    const callback = vi.fn((value: string) => value.length);
    expect(guardCallbacks({ callback }, () => true).callback("hello")).toBe(5);
    expect(callback).toHaveBeenCalledWith("hello");
  });
  /** 前置请求已结束；验证空回调集合与取消后的迟到响应均无写入，无清理数据。 */
  it("取消后忽略所有迟到更新", () => {
    let active = true;
    const callback = vi.fn();
    const guarded = guardCallbacks({ callback }, () => active);
    guarded.callback();
    active = false;
    guarded.callback("late");
    expect(callback).toHaveBeenCalledTimes(1);
    expect(guardCallbacks({}, () => false)).toEqual({});
  });
});
