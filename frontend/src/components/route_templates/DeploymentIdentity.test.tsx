import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import DeploymentIdentity from "./DeploymentIdentity";

describe("部署权重的可读身份", () => {
  /** 验证完整目录展示供应商、上游型号、公开模型和协议，ID 为辅助信息；卸载清理 DOM。 */
  it("显示可读身份且不展示凭据地址", () => {
    render(
      <DeploymentIdentity
        id="dep-a"
        deployment={{
          model_name: "chat",
          model: "openai/upstream",
          api_base: "https://secret.invalid",
          supplier: "供应商 A",
          provider: "openai",
          transport: "openai",
          endpoint_types: ["chat_completions"],
        }}
      />,
    );
    expect(screen.getByText("供应商 A · openai/upstream")).toBeInTheDocument();
    expect(screen.getByText("chat · openai · chat_completions")).toBeInTheDocument();
    expect(screen.getByText("ID: dep-a")).toBeInTheDocument();
    expect(screen.queryByText(/secret.invalid/)).not.toBeInTheDocument();
  });

  /** 验证缺少供应商时使用提供商，缺少双方时提示未标注；重渲染仅替换 DOM，无数据写入。 */
  it("为缺少供应商的目录提供回退标签", () => {
    const deployment = { model_name: "chat", model: "upstream", api_base: "", provider: "openai" };
    const view = render(<DeploymentIdentity id="dep-b" deployment={deployment} />);
    expect(screen.getByText("openai · upstream")).toBeInTheDocument();
    view.rerender(<DeploymentIdentity id="dep-b" deployment={{ ...deployment, provider: undefined }} />);
    expect(screen.getByText("提供商未标注 · upstream")).toBeInTheDocument();
  });

  /** 验证失效目录引用显示错误与原 ID，便于定位并修复权重；自动卸载，不产生后台数据。 */
  it("明确展示不存在的部署引用", () => {
    render(<DeploymentIdentity id="deleted-deployment" />);
    expect(screen.getByText("部署已不存在")).toBeInTheDocument();
    expect(screen.getByText("ID: deleted-deployment")).toBeInTheDocument();
  });
});
