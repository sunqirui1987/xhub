import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import { I18nProvider } from "@/i18n/I18nProvider";
import { ApiRunnerDialog } from "./ApiRunnerDialog";

/** 目的：验证弹窗主动打开、关闭清除凭据与重新打开；前置双语组件，无请求上游，测试框架卸载清理 DOM。 */
it.each([
  ["en", "Run online", "Close runner", "XHub API key"],
  ["zh-CN", "在线运行", "关闭在线运行", "XHub API 密钥"],
] as const)("opens and resets the modal in %s", async (locale, title, close, keyLabel) => {
  const fetch = vi.spyOn(globalThis, "fetch");
  render(
    <I18nProvider initialLocale={locale}>
      <ApiRunnerDialog base="https://gateway.example" endpoint="/models" method="GET" />
    </I18nProvider>,
  );
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: title }));
  expect(await screen.findByRole("dialog", { name: title })).toBeVisible();
  fireEvent.change(screen.getByLabelText(keyLabel), { target: { value: "private-key" } });
  fireEvent.click(screen.getByRole("button", { name: close }));
  await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  fireEvent.click(screen.getByRole("button", { name: title }));
  expect(await screen.findByLabelText(keyLabel)).toHaveValue("");
  expect(fetch).not.toHaveBeenCalled();
  fetch.mockRestore();
});
