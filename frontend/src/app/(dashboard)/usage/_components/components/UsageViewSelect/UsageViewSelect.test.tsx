import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { UsageViewSelect } from "./UsageViewSelect";
import { translate } from "@/i18n/translate";

const offers = (label: string) => screen.queryByRole("tab", { name: label }) != null;

describe("UsageViewSelect", () => {
  const mockOnChange = vi.fn();

  beforeEach(() => {
    mockOnChange.mockClear();
  });

  it("should render", async () => {
    const user = userEvent.setup();
    const { container } = render(<UsageViewSelect value="my-usage" onChange={mockOnChange} userRole="Internal User" />);

    expect(screen.getByText(translate("en", "pages.usage.viewTitle"))).toBeInTheDocument();
    expect(screen.getByText(translate("en", "pages.usage.viewDescription"))).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: translate("en", "pages.usage.yours"), selected: true })).toBeInTheDocument();
    expect(offers(translate("en", "pages.usage.yours"))).toBe(true);
    void user;
  });

  it("should call onChange when value changes", async () => {
    const user = userEvent.setup();
    render(<UsageViewSelect value="global" onChange={mockOnChange} userRole="Admin" />);

    await user.click(screen.getByRole("tab", { name: translate("en", "pages.usage.team") }));

    expect(mockOnChange).toHaveBeenCalled();
    expect(mockOnChange.mock.calls[0][0]).toBe("team");
  });

  it("should show Tag Usage for non-admin users with tag usage permission", async () => {
    const user = userEvent.setup();
    render(
      <UsageViewSelect value="global" onChange={mockOnChange} userRole="Internal User" canViewTagUsage={true} />,
    );

    expect(offers(translate("en", "pages.usage.tag"))).toBe(false);
    void user;
  });

  it("should hide Tag Usage for non-admin users without tag usage permission", async () => {
    const user = userEvent.setup();
    render(<UsageViewSelect value="global" onChange={mockOnChange} userRole="Internal User" />);

    expect(offers(translate("en", "pages.usage.tag"))).toBe(false);
    void user;
  });

  it.each(["pages.usage.organization"] as const)("should show %s to an admin", async (key) => {
    const user = userEvent.setup();
    render(<UsageViewSelect value="global" onChange={mockOnChange} userRole="Admin" />);

    expect(offers(translate("en", key))).toBe(true);
    void user;
  });

  it.each(["pages.usage.organization"] as const)(
    "should hide %s from an internal user",
    async (key) => {
    const user = userEvent.setup();
    render(
      <UsageViewSelect value="global" onChange={mockOnChange} userRole="Internal User" canViewTagUsage={true} />,
    );

    expect(offers(translate("en", key))).toBe(false);
    void user;
  });

  // An org admin's session role is "Internal User" — org-admin-ness lives in the
  // membership table — so the two rows above cannot tell them apart from a plain
  // internal user. Organization Usage must open for them, and only that option:
  // the proxy serves them /organization/daily/activity scoped to the orgs they
  // administer, but still refuses the agent usage route.
  it.each([["pages.usage.organization", true]] as const)("should offer %s to an org admin: %s", async (key, expected) => {
    const user = userEvent.setup();
    render(
      <UsageViewSelect value="global" onChange={mockOnChange} userRole="Internal User" isOrgAdmin={true} />,
    );

    expect(offers(translate("en", key))).toBe(expected);
    void user;
  });

  it("offers team and user filters to a team administrator, and not the organization", async () => {
    render(
      <UsageViewSelect value="team" onChange={mockOnChange} userRole="Internal User" isTeamAdmin />,
    );
    expect(offers(translate("en", "pages.usage.team"))).toBe(true);
    expect(offers(translate("en", "pages.usage.user"))).toBe(true);
    expect(offers(translate("en", "pages.usage.organization"))).toBe(false);
  });

  it("hides team and user filters from a plain member", async () => {
    render(<UsageViewSelect value="my-usage" onChange={mockOnChange} userRole="Internal User" />);
    expect(offers(translate("en", "pages.usage.team"))).toBe(false);
    expect(offers(translate("en", "pages.usage.user"))).toBe(false);
    expect(offers(translate("en", "pages.usage.yours"))).toBe(true);
  });

  it("shows only the localized view name on the closed control", async () => {
    const { setActiveLocale } = await import("@/i18n/runtime");
    setActiveLocale("zh-CN");
    render(<UsageViewSelect value="global" onChange={mockOnChange} userRole="Admin" />);
    const selected = screen.getByRole("tab", { selected: true });
    expect(selected).toHaveTextContent("全局用量");
    expect(selected.textContent ?? "").not.toMatch(/(^|\s)global(\s|$)/);
    setActiveLocale("en");
  });

  it("uses Simplified Chinese labels when locale is zh-CN", async () => {
    const { setActiveLocale } = await import("@/i18n/runtime");
    setActiveLocale("zh-CN");
    const user = userEvent.setup();
    render(<UsageViewSelect value="global" onChange={mockOnChange} userRole="Admin" />);
    expect(screen.getByText(translate("zh-CN", "pages.usage.viewTitle"))).toBeInTheDocument();
    expect(screen.queryByText(translate("en", "pages.usage.viewTitle"))).not.toBeInTheDocument();
    expect(offers(translate("zh-CN", "pages.usage.global"))).toBe(true);
    expect(offers(translate("en", "pages.usage.global"))).toBe(false);
    void user;
    setActiveLocale("en");
  });
});
