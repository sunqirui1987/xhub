import { render, screen } from "@testing-library/react";
import { describe, it, expect } from "vitest";
import { UnavailableEndpoint } from "./UnavailableEndpoint";

describe("UnavailableEndpoint", () => {
  /** 前置后台校验原因；验证原因、配置入口和无输入控件，测试环境自动卸载清理。 */
  it("shows an actionable reason without a misleading composer", () => {
    render(<UnavailableEndpoint reason="model_info.transport is required" />);
    expect(screen.getByRole("status")).toHaveTextContent("该模型没有可调用的端点");
    expect(screen.getByRole("status")).toHaveTextContent("model_info.transport is required");
    expect(screen.getByRole("link", { name: "前往模型配置" })).toHaveAttribute("href", "/models-and-endpoints");
    expect(screen.queryByRole("textbox")).not.toBeInTheDocument();
  });
  /** 前置仅缺少文本对比端点且无错误原因；验证对比限定提示，测试环境自动卸载清理。 */
  it("explains a missing comparison endpoint without inventing metadata", () => {
    render(<UnavailableEndpoint compare />);
    expect(screen.getByRole("status")).toHaveTextContent("该模型没有可用于对比的文本端点");
    expect(screen.queryByText("undefined")).not.toBeInTheDocument();
  });
});
