import { act, renderHook } from "@testing-library/react";
import { beforeEach, expect, it } from "vitest";
import { useCodeInterpreter } from "./useCodeInterpreter";

/** 清理会话缓存；无参数和返回值，各用例仅写测试浏览器存储，不创建外部容器。 */
beforeEach(() => sessionStorage.clear());

/** 前置缺失、损坏及错误类型缓存；验证安全回退关闭，自动卸载并由下一用例清理存储。 */
it.each(["{bad", "null", "[]", '"true"', "1", "false"])("缓存 %s 安全回退", (saved) => {
  sessionStorage.setItem("codeInterpreterEnabled", saved);
  expect(renderHook(() => useCodeInterpreter()).result.current.enabled).toBe(false);
});

/** 前置合法开关缓存；验证恢复、切换及结果清空，纯浏览器存储由下一用例清理。 */
it("恢复开关并持久化显式切换", () => {
  sessionStorage.setItem("codeInterpreterEnabled", "true");
  const { result } = renderHook(() => useCodeInterpreter());
  expect(result.current.enabled).toBe(true);
  act(() => result.current.toggle());
  expect(result.current.enabled).toBe(false);
  expect(sessionStorage.getItem("codeInterpreterEnabled")).toBe("false");
  act(() => result.current.clearResult());
  expect(result.current.result).toBeNull();
});
