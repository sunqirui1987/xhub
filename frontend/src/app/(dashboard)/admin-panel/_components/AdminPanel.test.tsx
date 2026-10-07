import { screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { renderWithProviders } from "../../../../../tests/test-utils";
import AdminPanel from "./AdminPanel";

vi.mock("@/components/Settings/AdminSettings/LoggingSettings/LoggingSettings", () => ({
  default: () => <div>Logging Settings</div>,
}));

describe("AdminPanel", () => {
  it("keeps prompt storage and drops the unused settings tabs", () => {
    renderWithProviders(<AdminPanel />);

    expect(screen.getByText("Logging Settings")).toBeInTheDocument();
    for (const gone of [
      "SSO Settings",
      "Security Settings",
      "SCIM",
      "Hashicorp Vault",
      "CyberArk Conjur",
      "Plugins",
    ]) {
      expect(screen.queryByRole("tab", { name: gone })).not.toBeInTheDocument();
    }
  });
});
