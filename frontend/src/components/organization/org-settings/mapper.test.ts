import { describe, expect, it } from "vitest";

import type { Organization } from "@/components/networking";

import { buildOrgPatch, orgToForm } from "./mapper";

const org: Organization = {
  organization_id: "org-1",
  organization_alias: "acme",
  budget_id: "budget-1",
  metadata: { cost_center: "eng" },
  models: ["gpt-5.2"],
  spend: 0,
  model_spend: {},
  created_at: "2026-01-01T00:00:00Z",
  created_by: "admin",
  updated_at: "2026-01-01T00:00:00Z",
  updated_by: "admin",
  litellm_budget_table: { max_budget: 100.5, budget_duration: "30d", tpm_limit: 1000, rpm_limit: 50 },
  teams: null,
  users: null,
  members: null,
  object_permission: {
    object_permission_id: "op-1",
    mcp_servers: ["srv-1"],
    mcp_access_groups: ["group-1"],
    mcp_toolsets: ["ts-1"],
    vector_stores: ["vs-1"],
  },
};

describe("orgToForm", () => {
  it("maps the server row onto widget-space strings and arrays", () => {
    expect(orgToForm(org)).toEqual({
      organization_alias: "acme",
      models: ["gpt-5.2"],
      max_budget: "100.5",
      budget_duration: "30d",
      tpm_limit: "1000",
      rpm_limit: "50",
      metadata: JSON.stringify({ cost_center: "eng" }, null, 2),
    });
  });

  it("maps missing budget values and permissions to empty widget state", () => {
    const bare: Organization = {
      ...org,
      metadata: {},
      litellm_budget_table: { max_budget: null, budget_duration: null, tpm_limit: null, rpm_limit: null },
      object_permission: undefined,
    };

    expect(orgToForm(bare)).toEqual({
      organization_alias: "acme",
      models: ["gpt-5.2"],
      max_budget: "",
      budget_duration: "",
      tpm_limit: "",
      rpm_limit: "",
      metadata: "",
    });
  });
});

describe("buildOrgPatch", () => {
  it("returns an empty patch when nothing is dirty", () => {
    expect(buildOrgPatch({})).toEqual({});
  });

  it("sends only the dirty scalar and converts it to a number", () => {
    expect(buildOrgPatch({ tpm_limit: "2000" })).toEqual({ tpm_limit: 2000 });
  });

  it("clears scalars with null when the widget is emptied", () => {
    expect(buildOrgPatch({ max_budget: "", tpm_limit: "", budget_duration: "" })).toEqual({
      max_budget: null,
      tpm_limit: null,
      budget_duration: null,
    });
  });

  it("keeps an empty models list as [] rather than dropping or nulling it", () => {
    expect(buildOrgPatch({ models: [] })).toEqual({ models: [] });
  });

  it("clears metadata with null and parses non-empty metadata JSON", () => {
    expect(buildOrgPatch({ metadata: "" })).toEqual({ metadata: null });
    expect(buildOrgPatch({ metadata: '{"a": 1}' })).toEqual({ metadata: { a: 1 } });
  });

  it("omits agent, MCP, and vector-store fields from an organization patch", () => {
    expect(buildOrgPatch({ organization_alias: "acme-2" })).toEqual({ organization_alias: "acme-2" });
    expect(buildOrgPatch({ organization_alias: "acme-2" })).not.toHaveProperty("object_permission");
  });
});
