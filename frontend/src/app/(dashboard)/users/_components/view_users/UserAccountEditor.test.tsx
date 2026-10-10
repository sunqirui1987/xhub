import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import UserAccountEditor from "./UserAccountEditor";
import type { UserInfoV2Response } from "@/components/networking";

const { update, fromError } = vi.hoisted(() => ({ update: vi.fn(), fromError: vi.fn() }));
vi.mock("@/components/networking", () => ({ userUpdateUserCall: update }));
vi.mock("@/lib/toast", () => ({ toast: { success: vi.fn(), fromError } }));
const account = { user_id: "u1", user_email: "user@example.com", user_alias: "Before", user_role: "internal_user", max_budget: 10 } as UserInfoV2Response;

describe("UserAccountEditor", () => {
  beforeEach(() => { vi.clearAllMocks(); update.mockResolvedValue({}); });

  /** 正常保存：前置已有账户，验证邮箱、名称和零预算发送正确且刷新回调执行；组件卸载清理，无外部写入。 */
  it("saves supported fields including a zero budget", async () => {
    const user = userEvent.setup();
    const onSaved = vi.fn().mockResolvedValue(undefined);
    render(<UserAccountEditor user={account} accessToken="session" onSaved={onSaved} onCancel={vi.fn()} />);
    expect(screen.getByRole("combobox", { name: "Account role" })).toHaveTextContent("Regular user");
    await user.clear(screen.getByLabelText("User Alias"));
    await user.type(screen.getByLabelText("User Alias"), "After");
    await user.clear(screen.getByLabelText("Max Budget (USD)"));
    await user.type(screen.getByLabelText("Max Budget (USD)"), "0");
    await user.click(screen.getByRole("button", { name: "Save Changes" }));
    await waitFor(() => expect(onSaved).toHaveBeenCalledOnce());
    expect(update).toHaveBeenCalledWith("session", { user_id: "u1", user_email: "user@example.com", user_alias: "After", user_role: "user", max_budget: 0, blocked: false }, null);
  });

  /** 边界保存：前置名称/预算为空且账户禁用，验证空名称和无限预算不会被默认值覆盖；卸载清理。 */
  it("preserves null budgets and allows clearing the name", async () => {
    const user = userEvent.setup();
    render(<UserAccountEditor user={{ ...account, user_alias: null, max_budget: null, blocked: true }} accessToken="session" onSaved={vi.fn().mockResolvedValue(undefined)} onCancel={vi.fn()} />);
    await user.click(screen.getByRole("button", { name: "Save Changes" }));
    await waitFor(() => expect(update).toHaveBeenCalledWith("session", expect.objectContaining({ user_alias: "", max_budget: null, blocked: true }), null));
  });

  /** 失败保存：前置后台拒绝更新，验证错误可见、输入保留、刷新不执行且可重试；卸载清理。 */
  it("keeps the input and reports failed saves", async () => {
    const error = new Error("email already exists");
    update.mockRejectedValue(error);
    const user = userEvent.setup();
    const onSaved = vi.fn();
    render(<UserAccountEditor user={account} accessToken="session" onSaved={onSaved} onCancel={vi.fn()} />);
    await user.click(screen.getByRole("button", { name: "Save Changes" }));
    await waitFor(() => expect(fromError).toHaveBeenCalledWith(error));
    expect(screen.getByLabelText("User Alias")).toHaveValue("Before");
    expect(onSaved).not.toHaveBeenCalled();
    expect(screen.getByRole("button", { name: "Save Changes" })).toBeEnabled();
  });

  /** 取消操作：前置可编辑账户，点击取消仅触发回调，后台不写入；卸载清理。 */
  it("cancels without writing", async () => {
    const onCancel = vi.fn();
    render(<UserAccountEditor user={account} accessToken="session" onSaved={vi.fn()} onCancel={onCancel} />);
    await userEvent.setup().click(screen.getByRole("button", { name: "Cancel" }));
    expect(onCancel).toHaveBeenCalledOnce();
    expect(update).not.toHaveBeenCalled();
  });
});
