import { renderWithProviders, screen } from "../../tests/test-utils";
import { vi } from "vitest";
import { NoRedisWarningBanner } from "./NoRedisWarningBanner";
import type { HealthReadinessDetailsResponse } from "@/app/(dashboard)/hooks/healthReadiness/useHealthReadinessDetails";
import type { UseQueryResult } from "@tanstack/react-query";

vi.mock("@/app/(dashboard)/hooks/healthReadiness/useHealthReadinessDetails", () => ({
  useHealthReadinessDetails: vi.fn(),
}));

import { useHealthReadinessDetails } from "@/app/(dashboard)/hooks/healthReadiness/useHealthReadinessDetails";

const mockDetails = (data: Partial<HealthReadinessDetailsResponse> | undefined) => {
  vi.mocked(useHealthReadinessDetails).mockReturnValue({ data } as UseQueryResult<HealthReadinessDetailsResponse>);
};

describe("NoRedisWarningBanner", () => {
  it("should warn that Redis is recommended when the proxy reports no Redis", () => {
    mockDetails({ status: "healthy", show_no_redis_warning: true });
    renderWithProviders(<NoRedisWarningBanner accessToken="token" />);
    expect(screen.getByRole("alert")).toBeInTheDocument();
    expect(screen.getByText(/No Redis configured\. Redis is highly recommended/i)).toBeInTheDocument();
    expect(screen.getByText(/more than one worker/i)).toBeInTheDocument();
  });

  it("does not link out to external Redis docs", () => {
    mockDetails({ status: "healthy", show_no_redis_warning: true });
    renderWithProviders(<NoRedisWarningBanner accessToken="token" />);
    expect(screen.queryByRole("link")).not.toBeInTheDocument();
  });

  it("should explain the per-worker limit risk", () => {
    mockDetails({ status: "healthy", show_no_redis_warning: true });
    renderWithProviders(<NoRedisWarningBanner accessToken="token" />);
    expect(screen.getByText(/rate limits, budgets, router state, and cache invalidation are per worker/i)).toBeInTheDocument();
  });

  it("should render nothing when the proxy reports the warning is not needed", () => {
    mockDetails({ status: "healthy", show_no_redis_warning: false });
    const { container } = renderWithProviders(<NoRedisWarningBanner accessToken="token" />);
    expect(container).toBeEmptyDOMElement();
  });

  it("should render nothing when readiness details are unavailable", () => {
    mockDetails(undefined);
    const { container } = renderWithProviders(<NoRedisWarningBanner accessToken={null} />);
    expect(container).toBeEmptyDOMElement();
  });

  it("should pass the access token to the readiness hook", () => {
    mockDetails(undefined);
    renderWithProviders(<NoRedisWarningBanner accessToken="my-token" />);
    expect(useHealthReadinessDetails).toHaveBeenCalledWith("my-token");
  });
});
