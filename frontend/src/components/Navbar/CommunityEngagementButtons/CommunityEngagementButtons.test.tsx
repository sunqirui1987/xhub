import { describe, expect, it } from "vitest";
import { renderWithProviders, screen } from "../../../../tests/test-utils";
import { CommunityEngagementButtons } from "./CommunityEngagementButtons";

describe("CommunityEngagementButtons", () => {
  it("does not render upstream community links", () => {
    const { container } = renderWithProviders(<CommunityEngagementButtons />);
    expect(container).toBeEmptyDOMElement();
    expect(screen.queryByRole("link")).not.toBeInTheDocument();
  });
});
