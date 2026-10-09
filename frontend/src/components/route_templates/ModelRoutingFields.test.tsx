import React, { useState } from "react";
import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import ModelRoutingFields from "./ModelRoutingFields";
import type { ModelRoutingRule, SplitDeployment } from "./templateForm";

const deployments: SplitDeployment[] = [
  { deployment_id: "east", model_name: "shared", model: "vendor/a", api_base: "https://east" },
  { deployment_id: "west", model_name: "shared", model: "vendor/b", api_base: "https://west" },
  { deployment_id: "other", model_name: "other", model: "other", api_base: "https://other" },
];

/** 受控模板规则测试宿主；参数为初始规则和回调，组件卸载后清理内存状态。 */
function Harness({
  initial = [],
  changed = () => {},
}: {
  initial?: ModelRoutingRule[];
  changed?: (rules: ModelRoutingRule[]) => void;
}) {
  const [rules, setRules] = useState(initial);
  return (
    <ModelRoutingFields
      rules={rules}
      deployments={deployments}
      onChange={(next) => {
        setRules(next);
        changed(next);
      }}
    />
  );
}

describe("模型规则表单", () => {
  /** 验证按公开模型添加规则，并在添加后从候选列表移除以防重复。 */
  it("只添加公开模型和路由策略", () => {
    const changed = vi.fn();
    render(<Harness changed={changed} />);
    fireEvent.change(screen.getByLabelText("公开模型"), { target: { value: "shared" } });
    fireEvent.click(screen.getByText("添加模型规则"));
    expect(changed).toHaveBeenLastCalledWith([{ model: "shared", strategy: "simple-shuffle" }]);
    expect(screen.queryByRole("option", { name: "shared" })).not.toBeInTheDocument();
  });

  /** 验证策略可修改；省略模板权重时继承默认，切换非加权策略后清除权重。 */
  it("修改策略并恢复非加权配置", () => {
    const changed = vi.fn();
    render(<Harness initial={[{ model: "shared", strategy: "traffic-split" }]} changed={changed} />);
    expect(screen.getByText(/实时继承模型管理的默认权重/)).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText("shared 路由逻辑"), { target: { value: "least-busy" } });
    expect(changed).toHaveBeenLastCalledWith([{ model: "shared", strategy: "least-busy" }]);
    expect(screen.queryByRole("spinbutton")).not.toBeInTheDocument();
  });

  /** 验证删除一条模板规则只恢复该模型默认分配，其他规则保留。 */
  it("删除规则恢复该模型默认分配", () => {
    const changed = vi.fn();
    render(
      <Harness
        initial={[
          { model: "shared", strategy: "least-busy" },
          { model: "other", strategy: "random" },
        ]}
        changed={changed}
      />,
    );
    fireEvent.click(within(screen.getByRole("region", { name: "模型规则 shared" })).getByText("删除规则"));
    expect(changed).toHaveBeenLastCalledWith([{ model: "other", strategy: "random" }]);
  });
  /** 验证自定义权重独立编辑、可读部署身份、空输入及恢复继承；内存宿主卸载清理。 */
  it("自定义端点权重且可切回默认", () => {
    const changed = vi.fn();
    render(<Harness initial={[{ model: "shared", strategy: "traffic-split" }]} changed={changed} />);
    fireEvent.change(screen.getByLabelText("shared 权重来源"), { target: { value: "custom" } });
    expect(screen.getByText(/vendor\/a/)).toBeInTheDocument();
    expect(screen.getByText(/vendor\/b/)).toBeInTheDocument();
    expect(screen.queryByLabelText("模板部署 other 权重")).not.toBeInTheDocument();
    fireEvent.change(screen.getByLabelText("模板部署 east 权重"), { target: { value: "0" } });
    expect(changed.mock.lastCall![0][0].allocations).toContainEqual({ deployment_id: "east", weight: 0 });
    fireEvent.change(screen.getByLabelText("模板部署 west 权重"), { target: { value: "" } });
    expect(
      Number.isNaN(changed.mock.lastCall![0][0].allocations.find((a: any) => a.deployment_id === "west").weight),
    ).toBe(true);
    fireEvent.change(screen.getByLabelText("shared 权重来源"), { target: { value: "inherit" } });
    expect(changed).toHaveBeenLastCalledWith([{ model: "shared", strategy: "traffic-split" }]);
    expect(screen.queryByRole("spinbutton")).not.toBeInTheDocument();
  });
});
