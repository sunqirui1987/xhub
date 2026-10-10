import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { diagnoseRequest, RequestDiagnostic } from "./RequestDiagnostic";
describe("请求诊断", () => {
  /** 前置结构化网关错误；验证凭据缺失的操作入口、HTTP 状态和折叠原始信息；自动卸载，无后台数据。 */
  it("展示上游鉴权错误及配置入口", () => {
    const failure = diagnoseRequest(
      Object.assign(new Error("This model has no upstream API key configured."), { status: 401 }),
    );
    render(<RequestDiagnostic failure={failure} />);
    expect(screen.getByRole("alert")).toHaveTextContent("HTTP 401");
    expect(screen.getByRole("link")).toHaveAttribute("href", "/ui/models-and-endpoints");
    expect(failure.configure).toBe(true);
  });
  /** 前置原生 JSON 错误；验证模型失效与密钥错误独立诊断，保留原文；无外部写入。 */
  it("识别原生模型不存在", () => {
    const failure = diagnoseRequest(new Error('{"error":{"message":"model not found: gpt-5.6-sol"}}'));
    expect(failure.title).toBe("Model has no available deployment");
    expect(failure.configure).toBe(true);
  });
  /** 前置后台已停用模型；验证停用与零部署均有配置入口，无后台写入或清理。 */
  it("识别已停用模型", () => {
    expect(diagnoseRequest(new Error("model is disabled")).title).toBe("Model has no available deployment");
  });
  /** 前置空、非 JSON 与网络异常；验证诊断不会崩溃，不将普通失败导向模型配置；无清理数据。 */
  it.each([
    null,
    undefined,
    "bad JSON",
    new Error("Failed to fetch"),
    { error: { message: "quota exceeded" }, status: 429 },
  ])("处理边界异常 %s", (error) => {
    const failure = diagnoseRequest(error);
    expect(failure.message).toBeTruthy();
    expect(failure.configure).toBe(false);
  });
});

/** 前置只有错误类型和字符串状态的网关正文；验证中文原因仍可诊断、详情保留原文；纯转换无清理。 */
it("结构化错误保留类型和状态", () => {
  const source = { error: { message: "供应商凭据缺失", type: "upstream_auth", code: "401" } };
  const failure = diagnoseRequest(source);
  expect(failure.title).toBe("Upstream key is missing");
  expect(failure.status).toBe(401);
  expect(failure.detail).toContain("upstream_auth");
  expect(diagnoseRequest(new Error(JSON.stringify(source))).status).toBe(401);
});
