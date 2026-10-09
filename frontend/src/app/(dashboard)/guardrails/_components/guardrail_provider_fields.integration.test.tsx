import React from "react";
import { fireEvent, screen, waitFor } from "@testing-library/react";
import { useForm } from "react-hook-form";
import { describe, expect, it, vi } from "vitest";
import { renderWithProviders } from "@/../tests/test-utils";
import GuardrailProviderFields from "./guardrail_provider_fields";
import { populateGuardrailProviderMap } from "./guardrail_info_helpers";
import type { GuardrailFormValues } from "./GuardrailFormField";

const params = {
  local: {
    ui_friendly_name: "Local rules",
    patterns: { param: "patterns", type: "list", required: false, description: "Regular expressions (one per line)" },
  },
};

/**
 * 用途：用真实表单和字段组件验证列表输入到提交数据的完整链路。
 * 参数：onValid：捕获成功提交数据的回调。
 * 返回：React 表单，固定选中本地提供商。
 * 调用：本文件两项集成测试。
 * 测试：空行过滤、正则逗号保留与清空数组。
 */
function Harness({ onValid }: { onValid: (values: GuardrailFormValues) => void }) {
  const form = useForm<GuardrailFormValues>();
  return (
    <form onSubmit={form.handleSubmit(onValid)}>
      <GuardrailProviderFields selectedProvider="Local" control={form.control} providerParams={params} />
      <button type="submit">save</button>
    </form>
  );
}

describe("local guardrail list fields", () => {
  it("submits a string array, removes blank lines and preserves commas inside regexes", async () => {
    populateGuardrailProviderMap(params);
    const onValid = vi.fn();
    renderWithProviders(<Harness onValid={onValid} />);
    const input = screen.getByLabelText(/patterns/);
    fireEvent.change(input, { target: { value: "  secret  \n\n[a-z]{2,4}\n" } });
    fireEvent.blur(input);
    fireEvent.click(screen.getByRole("button", { name: "save" }));
    await waitFor(() => expect(onValid).toHaveBeenCalledTimes(1));
    expect(onValid.mock.calls[0][0].patterns).toEqual(["secret", "[a-z]{2,4}"]);
  });

  it("submits an empty array when the user clears the list", async () => {
    populateGuardrailProviderMap(params);
    const onValid = vi.fn();
    renderWithProviders(<Harness onValid={onValid} />);
    const input = screen.getByLabelText(/patterns/);
    fireEvent.change(input, { target: { value: "secret" } });
    fireEvent.change(input, { target: { value: "" } });
    fireEvent.blur(input);
    fireEvent.click(screen.getByRole("button", { name: "save" }));
    await waitFor(() => expect(onValid).toHaveBeenCalledTimes(1));
    expect(onValid.mock.calls[0][0].patterns).toEqual([]);
  });
});
