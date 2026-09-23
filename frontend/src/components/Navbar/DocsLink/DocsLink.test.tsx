import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { DocsLink } from "./DocsLink";

describe("DocsLink", () => {
  it("does not render an outbound docs link", () => {
    const { container } = render(<DocsLink />);
    expect(container).toBeEmptyDOMElement();
    expect(screen.queryByRole("link")).not.toBeInTheDocument();
  });
});
