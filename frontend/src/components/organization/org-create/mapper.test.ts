import { describe, expect, it } from "vitest";

import { buildOrgCreateBody, emptyOrgFormValues } from "./mapper";

describe("buildOrgCreateBody", () => {
  it("sends only alias and models for a minimal form", () => {
    expect(buildOrgCreateBody({ ...emptyOrgFormValues, organization_alias: "acme" })).toStrictEqual({
      organization_alias: "acme",
      models: [],
    });
  });

  it("maps every field when the whole form is filled", () => {
    const filledForm = {
      organization_alias: "acme",
      models: ["gpt-5.2"],
      max_budget: "12.5",
      budget_duration: "30d",
      tpm_limit: "1000",
      rpm_limit: "50",
      metadata: '{"env": "prod"}',
    };
    const expectedBody = {
      organization_alias: "acme",
      models: ["gpt-5.2"],
      max_budget: 12.5,
      budget_duration: "30d",
      tpm_limit: 1000,
      rpm_limit: 50,
      metadata: { env: "prod" },
    };

    expect(buildOrgCreateBody(filledForm)).toStrictEqual(expectedBody);
  });

  it("does not send MCP, skill, policy, or vector-store fields", () => {
    const body = buildOrgCreateBody({ ...emptyOrgFormValues, organization_alias: "acme" });
    expect(body).not.toHaveProperty("object_permission");
    expect(body).not.toHaveProperty("mcp_servers");
    expect(body).not.toHaveProperty("policies");
  });

  it("parses metadata into an object instead of sending the raw string", () => {
    expect(
      buildOrgCreateBody({ ...emptyOrgFormValues, organization_alias: "acme", metadata: '{"a": 1}' }).metadata,
    ).toStrictEqual({ a: 1 });
  });
});
