import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { BlogDropdown } from "./BlogDropdown";

describe("BlogDropdown", () => {
  it("does not render the blog menu or outbound post links", () => {
    const { container } = render(<BlogDropdown />);
    expect(container).toBeEmptyDOMElement();
    expect(screen.queryByRole("link")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /blog/i })).not.toBeInTheDocument();
  });
});
