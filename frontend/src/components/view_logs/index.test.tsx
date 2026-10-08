import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import SpendLogsTable from "./index";
vi.mock("./RequestLogsPanel", () => ({
  default: ({ isActive }: { isActive: boolean }) => (
    <div data-testid="request-logs-panel">{isActive ? "active" : "inactive"}</div>
  ),
}));
vi.mock("./AuditLogsPanel", () => ({ default: () => <div data-testid="audit-logs-panel" /> }));
const props = { accessToken: "sk-test", token: "jwt-test", userRole: "Admin", userID: "user-1", premiumUser: true };
describe("Request logs page", () => {
  it.each(["Admin", "Internal User", "Internal Viewer"])("only mounts request logs for %s", (userRole) => {
    render(<SpendLogsTable {...props} userRole={userRole} />);
    expect(screen.getByTestId("request-logs-panel")).toHaveTextContent("active");
    expect(screen.queryByTestId("audit-logs-panel")).not.toBeInTheDocument();
    expect(screen.queryByRole("tab")).not.toBeInTheDocument();
  });
  it("waits for credentials", () => {
    render(<SpendLogsTable {...props} accessToken={null} />);
    expect(screen.getByRole("status")).toBeInTheDocument();
    expect(screen.queryByTestId("request-logs-panel")).not.toBeInTheDocument();
  });
});
