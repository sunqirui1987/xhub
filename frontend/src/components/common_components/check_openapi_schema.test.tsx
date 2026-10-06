import { render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import SchemaFormFields from "./check_openapi_schema";
import { getOpenAPISchema } from "../networking";

vi.mock("../networking", () => ({
  getOpenAPISchema: vi.fn(),
}));

describe("SchemaFormFields", () => {
  beforeEach(() => {
    vi.mocked(getOpenAPISchema).mockReset();
  });

  it("does not render a crash when the document has no schemas", async () => {
    vi.mocked(getOpenAPISchema).mockResolvedValue({ error: { message: "not found" } });
    const onAvailable = vi.fn();

    const { container } = render(
      <SchemaFormFields schemaComponent="GenerateKeyRequest" setValue={vi.fn()} onAvailable={onAvailable} />,
    );

    await waitFor(() => expect(onAvailable).toHaveBeenCalledWith(false));
    expect(container).toBeEmptyDOMElement();
    expect(screen.queryByText(/schemas/)).not.toBeInTheDocument();
  });
});
