import { useProxyConfig } from "@/app/(dashboard)/hooks/proxyConfig/useProxyConfig";
import { useStoreRequestInSpendLogs } from "@/app/(dashboard)/hooks/storeRequestInSpendLogs/useStoreRequestInSpendLogs";
import { toast } from "@/lib/toast";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { renderWithProviders } from "../../../../../tests/test-utils";
import LoggingSettings from "./LoggingSettings";

vi.mock("@/app/(dashboard)/hooks/storeRequestInSpendLogs/useStoreRequestInSpendLogs");
vi.mock("@/app/(dashboard)/hooks/proxyConfig/useProxyConfig", async () => {
  const actual = await vi.importActual<typeof import("@/app/(dashboard)/hooks/proxyConfig/useProxyConfig")>(
    "@/app/(dashboard)/hooks/proxyConfig/useProxyConfig",
  );
  return { ...actual, useProxyConfig: vi.fn() };
});

const mockUseStoreRequestInSpendLogs = vi.mocked(useStoreRequestInSpendLogs);
const mockUseProxyConfig = vi.mocked(useProxyConfig);

describe("LoggingSettings", () => {
  const mockMutate = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();
    mockUseStoreRequestInSpendLogs.mockReturnValue({ mutate: mockMutate, isPending: false } as never);
    mockUseProxyConfig.mockReturnValue({
      data: [{ field_name: "store_prompts_in_spend_logs", field_value: false }],
      isLoading: false,
    } as never);
  });

  it("saves the prompt storage switch and nothing else", async () => {
    const user = userEvent.setup();
    renderWithProviders(<LoggingSettings />);

    expect(screen.getByText("Logging Settings")).toBeInTheDocument();
    expect(screen.queryByText(/Retention Period/)).not.toBeInTheDocument();
    expect(screen.queryByText(/Cleanup/)).not.toBeInTheDocument();

    await user.click(screen.getByRole("switch"));
    await user.click(screen.getByRole("button", { name: "Save Settings" }));

    await waitFor(() => {
      expect(mockMutate).toHaveBeenCalledWith(
        { store_prompts_in_spend_logs: true },
        expect.objectContaining({ onSuccess: expect.any(Function), onError: expect.any(Function) }),
      );
    });
    const options = mockMutate.mock.calls[0][1] as { onSuccess: () => void };
    options.onSuccess();
    expect(toast.success).toHaveBeenCalled();
  });
});
