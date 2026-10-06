import { fireEvent, render } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import TagSelector from "./TagSelector";
import { tagListCall } from "../networking";

vi.mock("../networking", () => ({
  tagListCall: vi.fn().mockResolvedValue({}),
}));

vi.mock("@/app/(dashboard)/hooks/useAuthorized", () => ({
  default: () => ({ userRole: "Internal User" }),
}));

describe("TagSelector", () => {
  it("should render the tag selector", () => {
    render(<TagSelector onChange={() => {}} accessToken="test-token" />);
    expect(tagListCall).not.toHaveBeenCalled();
  });

  it("should allow creating new tags", () => {
    const { container } = render(<TagSelector onChange={() => {}} accessToken="test-token" />);
    const tagSelector = container.querySelector("input");
    expect(tagSelector).toBeInTheDocument();
    if (tagSelector) {
      fireEvent.change(tagSelector, { target: { value: "new-tag" } });
      expect(tagSelector).toHaveValue("new-tag");
      fireEvent.keyDown(tagSelector, { key: "Enter" });
      expect(tagSelector).toHaveValue("new-tag");
    }
  });
});
