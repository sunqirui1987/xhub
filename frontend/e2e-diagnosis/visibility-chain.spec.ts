import { GATEWAY } from "./helpers";
import { expect, test, type Page } from "@playwright/test";
import { login, loginAdmin, stableGoto, t, watchGateway } from "./helpers";

/**
 * The visibility chain, driven through the console.
 *
 * One realistic tenant is built from the top down — a platform administrator,
 * two organizations, an account administering each, teams under them with their
 * own administrators, and plain members — and then every account signs in
 * through the real login form and is asked the same questions.
 *
 * The Go suite (internal/gateway/visibility_chain_test.go) asserts this matrix
 * against the API. This file exists for the part only a browser reaches: that
 * the console renders what the server returns, that a page or a control closed
 * to a tier is actually absent, and that the two agree. A server that is right
 * and a UI that shows everything anyway is still a leak.
 */

const GW = GATEWAY;
const PASSWORD = "chain-password-1";

let seq = 0;
const uniq = (name: string) => `e2e-chain-${Date.now()}-${++seq}-${name}`;

/** sessionFrom reads the console session id out of the token cookie's JWT. */
async function sessionFrom(page: Page): Promise<string> {
  const cookies = await page.context().cookies();
  const token = cookies.find((c) => c.name === "token")?.value;
  expect(token, "the console token cookie is missing").toBeTruthy();
  return JSON.parse(Buffer.from(token!.split(".")[1], "base64url").toString()).key as string;
}

/**
 * as returns a caller bound to one session. Every read in this file is made by
 * somebody, and which somebody is the point of the assertion, so the session is
 * bound once here rather than passed on every call.
 */
function as(page: Page, session: string) {
  const send = (method: string, path: string, data?: unknown) =>
    page.request.fetch(`${GW}${path}`, {
      method,
      headers: { Authorization: `Bearer ${session}`, "Content-Type": "application/json" },
      data: data === undefined ? undefined : JSON.stringify(data),
    });

  return {
    /** ok issues a call that must succeed and returns the parsed body. */
    async ok(method: "GET" | "POST" | "PATCH", path: string, data?: unknown) {
      const res = await send(method, path, data);
      const text = await res.text();
      expect(res.ok(), `${method} ${path}: ${res.status()} ${text}`).toBeTruthy();
      return JSON.parse(text);
    },
    /** denied reports whether a call was refused. A 2xx means it was not. */
    async denied(method: "GET" | "POST", path: string, data?: unknown) {
      const res = await send(method, path, data);
      return !res.ok();
    },
    /** raw returns the response, for a listing whose body is a bare array. */
    raw: (method: "GET", path: string) => send(method, path),
  };
}

/** signInAs signs an account in through the console's own login form. */
async function signInAs(page: Page, email: string) {
  await login(page, email, PASSWORD);
  await expect(page.getByText(t("nav.apiKeys")).first()).toBeVisible({ timeout: 20_000 });
  return { email, session: await sessionFrom(page) };
}

test.describe.configure({ mode: "serial" });

test("the visibility chain holds at every tier", async ({ page }) => {
  test.setTimeout(240_000);
  const guard = watchGateway(page);

  // ---------- the platform administrator builds the tenant ----------
  await loginAdmin(page);
  const admin = as(page, await sessionFrom(page));

  const orgA = await admin.ok("POST", "/organization/new", { organization_alias: uniq("orgA") });
  const orgB = await admin.ok("POST", "/organization/new", { organization_alias: uniq("orgB") });

  const teamA1 = await admin.ok("POST", "/team/new", {
    organization_id: orgA.organization_id,
    team_alias: uniq("teamA1"),
  });
  const teamA2 = await admin.ok("POST", "/team/new", {
    organization_id: orgA.organization_id,
    team_alias: uniq("teamA2"),
  });
  const teamB1 = await admin.ok("POST", "/team/new", {
    organization_id: orgB.organization_id,
    team_alias: uniq("teamB1"),
  });

  const orgAdminAEmail = `${uniq("orgadminA")}@example.com`;
  const teamAdminA1Email = `${uniq("teamadminA1")}@example.com`;
  const memberA1Email = `${uniq("memberA1")}@example.com`;
  const orgAdminBEmail = `${uniq("orgadminB")}@example.com`;
  const memberB1Email = `${uniq("memberB1")}@example.com`;

  // Every account is created with its team, its team role and its organization
  // in one request. That is the shape the console now sends: the membership is
  // no longer a second call that could fail and leave a person behind.
  const newUser = (email: string, extra: Record<string, unknown>) => {
    const account = { user_email: email, password: PASSWORD, user_role: "user", ...extra };
    return admin.ok("POST", "/user/new", account);
  };

  // The organization administrator is deliberately a plain member of a team in
  // their own organization: their reach comes from the organization, not from a
  // team role, and the two must not be conflated.
  const orgAdminAScope = {
    team_id: teamA1.team_id,
    team_role: "user",
    organization_id: orgA.organization_id,
    max_budget: 12.5,
  };
  const orgAdminA = await newUser(orgAdminAEmail, orgAdminAScope);
  const teamAdminA1 = await newUser(teamAdminA1Email, { team_id: teamA1.team_id, team_role: "admin" });
  const memberA1 = await newUser(memberA1Email, { team_id: teamA1.team_id, team_role: "user" });
  // The account that runs org B, which the org A administrator must not reach.
  const orgAdminB = await newUser(orgAdminBEmail, {
    team_id: teamB1.team_id,
    team_role: "user",
    organization_id: orgB.organization_id,
  });
  const memberB1 = await newUser(memberB1Email, { team_id: teamB1.team_id, team_role: "user" });

  // The budget the create named is stored, not accepted and dropped.
  expect(orgAdminA.max_budget, "the create must store the budget it was given").toBe(12.5);

  const projectA1 = await admin.ok("POST", "/project/new", {
    team_id: teamA1.team_id,
    project_alias: uniq("projectA1"),
  });

  const tenant = {
    orgA: orgA.organization_id as string,
    orgB: orgB.organization_id as string,
    teamA1: teamA1.team_id as string,
    teamA2: teamA2.team_id as string,
    teamB1: teamB1.team_id as string,
    projectA1: projectA1.project_id as string,
    orgAdminAId: orgAdminA.user_id as string,
    teamAdminA1Id: teamAdminA1.user_id as string,
    memberA1Id: memberA1.user_id as string,
    orgAdminBId: orgAdminB.user_id as string,
    memberB1Id: memberB1.user_id as string,
  };

  const teamIDs = async (caller: ReturnType<typeof as>) => {
    const body = await caller.ok("GET", "/v2/team/list?page=1&page_size=500");
    return (body.teams as Array<{ team_id: string }>).map((x) => x.team_id);
  };
  const userIDs = async (caller: ReturnType<typeof as>) => {
    const body = await caller.ok("GET", "/user/list");
    return (body.users as Array<{ user_id: string }>).map((x) => x.user_id);
  };
  const projectIDs = async (caller: ReturnType<typeof as>) => {
    const body = await caller.ok("GET", "/project/list");
    return (body.projects as Array<{ project_id: string }>).map((x) => x.project_id);
  };

  // ---------- the platform administrator sees everything ----------
  const adminTeams = await teamIDs(admin);
  for (const id of [tenant.teamA1, tenant.teamA2, tenant.teamB1]) {
    expect(adminTeams, "the platform administrator must see every team").toContain(id);
  }

  const adminUsers = await userIDs(admin);
  for (const id of Object.values(tenant).filter((v) => v.startsWith("user"))) {
    expect(adminUsers, "the platform administrator must see every account").toContain(id);
  }

  expect(await projectIDs(admin), "the platform administrator must see every project").toContain(tenant.projectA1);

  // The organization listing is a bare array rather than a wrapped collection.
  const adminOrgs = (await (await admin.raw("GET", "/organization/list")).json()) as Array<{ organization_id: string }>;
  const adminOrgIDs = adminOrgs.map((o) => o.organization_id);
  expect(adminOrgIDs).toContain(tenant.orgA);
  expect(adminOrgIDs).toContain(tenant.orgB);

  // ---------- the organization administrator of A ----------
  const orgAdmin = as(page, (await signInAs(page, orgAdminAEmail)).session);

  const orgAdminTeams = await teamIDs(orgAdmin);
  expect(orgAdminTeams, "an org admin must see their own team").toContain(tenant.teamA1);
  expect(orgAdminTeams, "an org admin must see every team of their organization").toContain(tenant.teamA2);
  expect(orgAdminTeams, "an org admin must not see another organization").not.toContain(tenant.teamB1);

  expect(await projectIDs(orgAdmin), "an org admin must see their organization's projects").toContain(
    tenant.projectA1,
  );

  const orgAdminUsers = await userIDs(orgAdmin);
  expect(orgAdminUsers, "an org admin must see the people of their organization").toContain(tenant.memberA1Id);
  expect(orgAdminUsers, "an org admin must not see another organization").not.toContain(tenant.memberB1Id);

  expect(
    await orgAdmin.denied("GET", `/team/info?team_id=${tenant.teamB1}`),
    "an org admin must not read another organization's team",
  ).toBe(true);

  // ---------- the team administrator of team A1 ----------
  const teamAdmin = as(page, (await signInAs(page, teamAdminA1Email)).session);

  const teamAdminTeams = await teamIDs(teamAdmin);
  expect(teamAdminTeams, "a team admin must see their own team").toContain(tenant.teamA1);
  expect(teamAdminTeams, "a team admin must not see a sibling team").not.toContain(tenant.teamA2);
  expect(teamAdminTeams, "a team admin must not see another organization").not.toContain(tenant.teamB1);

  // The budget and the model list are the platform's, even on their own team.
  expect(
    await teamAdmin.denied("POST", "/team/update", { team_id: tenant.teamA1, max_budget: 1 }),
    "a team admin must not raise their own team's budget",
  ).toBe(true);
  expect(
    await teamAdmin.denied("POST", "/team/update", { team_id: tenant.teamA1, models: ["gpt-4o-mini"] }),
    "a team admin must not widen their own team's model list",
  ).toBe(true);

  // ---------- the plain member ----------
  const member = as(page, (await signInAs(page, memberA1Email)).session);

  const memberTeams = await teamIDs(member);
  expect(memberTeams, "a member must see the team they belong to").toContain(tenant.teamA1);
  expect(memberTeams, "a member must not see any other team").not.toContain(tenant.teamA2);
  expect(memberTeams, "a member must not see another organization").not.toContain(tenant.teamB1);

  // Another account is unreadable, and the refusal does not say whether it exists.
  expect(
    await member.denied("GET", `/user/info?user_id=${tenant.teamAdminA1Id}`),
    "a member must not read another account",
  ).toBe(true);

  for (const [label, path] of [
    ["the platform-wide usage report", "/global/activity"],
    ["the account picker", "/user/filter/ui"],
    ["the audit trail", "/audit/logs"],
  ] as const) {
    expect(await member.denied("GET", path), `a member must not read ${label}`).toBe(true);
  }
  expect(
    await member.denied("POST", "/user/new", { user_email: "e2e-chain-smuggled@example.com", password: PASSWORD }),
    "a member must not create an account",
  ).toBe(true);

  // ---------- the console hides what the server refuses ----------
  // The server being right is not enough: a page that renders rows it should not
  // have is a leak even when every individual call was authorized.
  await stableGoto(page, "/users");
  await expect(page.getByTestId("admin-shell")).toBeVisible({ timeout: 20_000 });
  await expect(page.getByText(t("pages.users.noUsersFound")).or(page.getByRole("row")).first()).toBeVisible({
    timeout: 20_000,
  });
  const rendered = await page.getByRole("row").allInnerTexts();
  expect(
    rendered.some((row) => row.includes(teamAdminA1Email) || row.includes(orgAdminBEmail)),
    `a member's account page rendered other people: ${rendered.join(" | ")}`,
  ).toBe(false);

  // ---------- the password reset follows the chain ----------
  // A team administrator hands their own member a new password. This is the
  // write that replaced the reset link, which pointed at an onboarding page
  // whose claim endpoint discarded the password.
  const newPassword = "chain-replaced-password-1";
  const reset = await teamAdmin.ok("POST", "/user/set_password", {
    user_id: tenant.memberA1Id,
    password: newPassword,
  });
  expect(reset.password_updated, "the reset must report that it applied").toBe(true);

  // It takes effect immediately: the member signs in with the new password, and
  // the session they had before it no longer works.
  const staleSession = member;
  await login(page, memberA1Email, newPassword);
  await expect(page.getByText(t("nav.apiKeys")).first()).toBeVisible({ timeout: 20_000 });
  expect(
    await staleSession.denied("GET", "/user/info"),
    "the session issued before the reset must stop working",
  ).toBe(true);

  // A team administrator reaches their own member and nobody else: not their
  // organization's administrator, and not an account outside their teams.
  expect(
    await teamAdmin.denied("POST", "/user/set_password", {
      user_id: tenant.orgAdminAId,
      password: "chain-should-not-apply-1",
    }),
    "a team admin must not reset their organization's administrator",
  ).toBe(true);
  expect(
    await teamAdmin.denied("POST", "/user/set_password", {
      user_id: tenant.memberB1Id,
      password: "chain-should-not-apply-1",
    }),
    "a team admin must not reset an account outside their teams",
  ).toBe(true);

  // ---------- the platform administrator can reset anybody ----------
  const adminReset = await admin.ok("POST", "/user/set_password", {
    user_id: tenant.orgAdminAId,
    password: "chain-admin-reset-1",
  });
  expect(adminReset.password_updated).toBe(true);
  await login(page, orgAdminAEmail, "chain-admin-reset-1");
  await expect(page.getByText(t("nav.apiKeys")).first()).toBeVisible({ timeout: 20_000 });

  guard.assertOk();
});
