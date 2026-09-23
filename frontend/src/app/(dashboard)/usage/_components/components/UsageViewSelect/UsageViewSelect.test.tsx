import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { chooseSelectOption } from "@/../tests/test-utils";
import { UsageViewSelect } from "./UsageViewSelect";
import { translate } from "@/i18n/translate";

const openMenu = async (user: ReturnType<typeof userEvent.setup>) => {
  await user.click(screen.getByRole("combobox"));
};

// The listbox is portalled outside the render container in both antd and Base UI, so an
// option is "offered" when the label appears more times on the page than inside the trigger.
const offers = (container: HTMLElement, label: string) =>
  screen.queryAllByText(label).length > within(container).queryAllByText(label).length;

describe("UsageViewSelect", () => {
  const mockOnChange = vi.fn();

  beforeEach(() => {
    mockOnChange.mockClear();
  });

  it("should render", async () => {
    const user = userEvent.setup();
    const { container } = render(<UsageViewSelect value="global" onChange={mockOnChange} userRole="Internal User" />);

    expect(screen.getByText(translate("en", "pages.usage.viewTitle"))).toBeInTheDocument();
    expect(screen.getByText(translate("en", "pages.usage.viewDescription"))).toBeInTheDocument();
    expect(screen.getByRole("combobox")).toBeInTheDocument();

    await openMenu(user);
    expect(offers(container, translate("en", "pages.usage.yours"))).toBe(true);
  });

  it("should call onChange when value changes", async () => {
    const user = userEvent.setup();
    render(<UsageViewSelect value="global" onChange={mockOnChange} userRole="Admin" />);

    await chooseSelectOption(user, screen.getByRole("combobox"), new RegExp(`^${translate("en", "pages.usage.team")}`));

    expect(mockOnChange).toHaveBeenCalled();
    expect(mockOnChange.mock.calls[0][0]).toBe("team");
  });

  it("should show Tag Usage for non-admin users with tag usage permission", async () => {
    const user = userEvent.setup();
    const { container } = render(
      <UsageViewSelect value="global" onChange={mockOnChange} userRole="Internal User" canViewTagUsage={true} />,
    );

    await openMenu(user);
    expect(offers(container, translate("en", "pages.usage.tag"))).toBe(true);
  });

  it("should hide Tag Usage for non-admin users without tag usage permission", async () => {
    const user = userEvent.setup();
    const { container } = render(<UsageViewSelect value="global" onChange={mockOnChange} userRole="Internal User" />);

    await openMenu(user);
    expect(offers(container, translate("en", "pages.usage.tag"))).toBe(false);
  });

  it.each(["pages.usage.organization", "pages.usage.agent"] as const)("should show %s to an admin", async (key) => {
    const user = userEvent.setup();
    const { container } = render(<UsageViewSelect value="global" onChange={mockOnChange} userRole="Admin" />);

    await openMenu(user);
    expect(offers(container, translate("en", key))).toBe(true);
  });

  it.each(["pages.usage.organization", "pages.usage.agent"] as const)(
    "should hide %s from an internal user",
    async (key) => {
    const user = userEvent.setup();
    const { container } = render(
      <UsageViewSelect value="global" onChange={mockOnChange} userRole="Internal User" canViewTagUsage={true} />,
    );

    await openMenu(user);
    expect(offers(container, translate("en", key))).toBe(false);
  });

  // An org admin's session role is "Internal User" — org-admin-ness lives in the
  // membership table — so the two rows above cannot tell them apart from a plain
  // internal user. Organization Usage must open for them, and only that option:
  // the proxy serves them /organization/daily/activity scoped to the orgs they
  // administer, but still refuses the agent usage route.
  it.each([
    ["pages.usage.organization", true],
    ["pages.usage.agent", false],
  ] as const)("should offer %s to an org admin: %s", async (key, expected) => {
    const user = userEvent.setup();
    const { container } = render(
      <UsageViewSelect value="global" onChange={mockOnChange} userRole="Internal User" isOrgAdmin={true} />,
    );

    await openMenu(user);
    expect(offers(container, translate("en", key))).toBe(expected);
  });

  it.each(["pages.usage.team", "pages.usage.tag"] as const)(
    "should keep %s available to an internal user",
    async (key) => {
    const user = userEvent.setup();
    const { container } = render(
      <UsageViewSelect value="global" onChange={mockOnChange} userRole="Internal User" canViewTagUsage={true} />,
    );

    await openMenu(user);
    expect(offers(container, translate("en", key))).toBe(true);
  });

  it("uses Simplified Chinese labels when locale is zh-CN", async () => {
    const { setActiveLocale } = await import("@/i18n/runtime");
    setActiveLocale("zh-CN");
    const user = userEvent.setup();
    const { container } = render(<UsageViewSelect value="global" onChange={mockOnChange} userRole="Admin" />);
    expect(screen.getByText(translate("zh-CN", "pages.usage.viewTitle"))).toBeInTheDocument();
    expect(screen.queryByText(translate("en", "pages.usage.viewTitle"))).not.toBeInTheDocument();
    await openMenu(user);
    expect(offers(container, translate("zh-CN", "pages.usage.global"))).toBe(true);
    expect(offers(container, translate("en", "pages.usage.global"))).toBe(false);
    setActiveLocale("en");
  });
});
