import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { renderWithProviders } from "../../../../tests/test-utils";
import CompliancePanel from "./CompliancePanel";

const compliance = vi.hoisted(() => ({
  eu: vi.fn(),
  gdpr: vi.fn(),
}));

vi.mock("@/components/networking", () => ({
  checkEuAiActCompliance: (...args: unknown[]) => compliance.eu(...args),
  checkGdprCompliance: (...args: unknown[]) => compliance.gdpr(...args),
}));

const logEntry = {
  request_id: "req-1",
  user: "user-1",
  model: "gpt-4o",
  startTime: "2026-10-08T00:00:00Z",
};

describe("CompliancePanel", () => {
  beforeEach(() => {
    compliance.eu.mockReset();
    compliance.gdpr.mockReset();
  });

  it("shows an unavailable state when the response has no checks array", async () => {
    compliance.eu.mockResolvedValue({ status: "ok", exported_count: 0 });
    compliance.gdpr.mockResolvedValue({ status: "ok", exported_count: 0 });

    renderWithProviders(<CompliancePanel accessToken="token" logEntry={logEntry} />);

    await waitFor(() => expect(screen.getAllByText("UNAVAILABLE")).toHaveLength(2));
    expect(screen.queryByText("NON-COMPLIANT")).not.toBeInTheDocument();

    await userEvent.click(screen.getByText("EU AI Act"));
    expect(screen.getByText("Compliance check returned an unexpected response")).toBeInTheDocument();
  });

  it("lists each check when the response has a checks array", async () => {
    compliance.eu.mockResolvedValue({
      compliant: true,
      regulation: "EU AI Act",
      checks: [{ check_name: "Guardrails applied", article: "Art. 9", passed: true, detail: "1 guardrail(s) applied" }],
    });
    compliance.gdpr.mockResolvedValue({
      compliant: false,
      regulation: "GDPR",
      checks: [{ check_name: "Data protection applied", article: "Art. 32", passed: false, detail: "No pre-call data protection applied" }],
    });

    renderWithProviders(<CompliancePanel accessToken="token" logEntry={logEntry} />);

    await waitFor(() => expect(screen.getByText("COMPLIANT")).toBeInTheDocument());
    await userEvent.click(screen.getByText("EU AI Act"));
    expect(screen.getByText("Guardrails applied")).toBeInTheDocument();
    expect(screen.getByText("1 guardrail(s) applied")).toBeInTheDocument();
  });
});
