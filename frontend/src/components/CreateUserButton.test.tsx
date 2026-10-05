import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { CreateUserButton } from "./CreateUserButton";
import * as networking from "./networking";

vi.mock("./networking", () => ({
  userCreateCall: vi.fn(),
  organizationListCall: vi.fn().mockResolvedValue([
    { organization_id: "org-1", organization_alias: "Platform" },
  ]),
}));

vi.mock("@/app/(dashboard)/hooks/useAuthorized", () => ({
  default: () => ({ accessToken: "token", userId: "admin-1", userRole: "proxy_admin", premiumUser: true }),
}));

vi.mock("./common_components/team_dropdown", () => ({
  default: ({
    id,
    value,
    onChange,
  }: {
    id?: string;
    value?: string | null;
    onChange?: (value: string | null) => void;
  }) => (
    <select aria-label="Add to a team" id={id} value={value ?? ""} onChange={(event) => onChange?.(event.target.value || null)}>
      <option value="">none</option>
      <option value="team-1">Frontend</option>
    </select>
  ),
}));

vi.mock("./common_components/OrganizationDropdown", () => ({
  default: ({
    id,
    value,
    onChange,
  }: {
    id?: string;
    value?: string | null;
    onChange?: (value: string | null) => void;
  }) => (
    <select
      aria-label="Make an organization administrator"
      id={id}
      value={value ?? ""}
      onChange={(event) => onChange?.(event.target.value || null)}
    >
      <option value="">none</option>
      <option value="org-1">Platform</option>
    </select>
  ),
}));

const mockUserCreateCall = vi.mocked(networking.userCreateCall);

const renderButton = (isEmbedded = false) => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <CreateUserButton userID="admin-1" accessToken="token" possibleUIRoles={null} isEmbedded={isEmbedded} />
    </QueryClientProvider>,
  );
};

const fillAccount = async (user: ReturnType<typeof userEvent.setup>) => {
  await user.type(screen.getByLabelText("User Email"), "new@example.com");
  await user.type(screen.getByLabelText("Initial password"), "password1");
};

const submittedPayload = () => mockUserCreateCall.mock.calls[0][2];

describe("CreateUserButton", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockUserCreateCall.mockResolvedValue({ user_id: "user-new" });
  });

  it("opens a create form that offers only the two account roles", async () => {
    const user = userEvent.setup();
    renderButton();
    await user.click(screen.getByRole("button", { name: /\+ create user/i }));

    expect(screen.getByRole("dialog", { name: /create user/i })).toBeInTheDocument();
    await user.click(screen.getByRole("combobox", { name: /account role/i }));
    expect(screen.getByRole("option", { name: /Regular user/ })).toBeInTheDocument();
    expect(screen.getByRole("option", { name: /Platform administrator/ })).toBeInTheDocument();
  });

  it("creates a regular user in one request and sends no scope the form left blank", async () => {
    const user = userEvent.setup();
    renderButton(true);
    await fillAccount(user);
    await user.click(screen.getByRole("button", { name: /create user/i }));

    await waitFor(() => expect(mockUserCreateCall).toHaveBeenCalled());
    expect(submittedPayload()).toMatchObject({
      user_email: "new@example.com",
      password: "password1",
      user_role: "user",
    });
    expect(submittedPayload()).not.toHaveProperty("team_id");
    expect(submittedPayload()).not.toHaveProperty("organization_id");
    expect(submittedPayload()).not.toHaveProperty("max_budget");
  });

  it("refuses a password shorter than 8 characters", async () => {
    const user = userEvent.setup();
    renderButton(true);
    await user.type(screen.getByLabelText("User Email"), "new@example.com");
    await user.type(screen.getByLabelText("Initial password"), "short");
    await user.click(screen.getByRole("button", { name: /create user/i }));

    expect(mockUserCreateCall).not.toHaveBeenCalled();
  });

  it("sends the team and its role in the same request as the account", async () => {
    const user = userEvent.setup();
    renderButton();
    await user.click(screen.getByRole("button", { name: /\+ create user/i }));
    await fillAccount(user);
    await user.selectOptions(screen.getByLabelText("Add to a team"), "team-1");
    await user.click(await screen.findByRole("combobox", { name: "Role in this team" }));
    await user.click(await screen.findByRole("option", { name: /Team admin/ }));
    await user.click(screen.getByRole("button", { name: /create user/i }));

    await waitFor(() => expect(mockUserCreateCall).toHaveBeenCalled());
    // The membership travels with the account, so a failure cannot leave a
    // person who exists but can reach nothing.
    expect(submittedPayload()).toMatchObject({ team_id: "team-1", team_role: "admin" });
  });

  it("sends a budget only when a ceiling was asked for", async () => {
    const user = userEvent.setup();
    renderButton(true);
    await fillAccount(user);
    await user.click(screen.getByRole("switch", { name: /budget/i }));
    await user.type(screen.getByLabelText("Budget in dollars"), "25.5");
    await user.click(screen.getByRole("button", { name: /create user/i }));

    await waitFor(() => expect(mockUserCreateCall).toHaveBeenCalled());
    expect(submittedPayload().max_budget).toBe(25.5);
  });

  it("does not offer to make the account an organization administrator", async () => {
    const user = userEvent.setup();
    renderButton();
    await user.click(screen.getByRole("button", { name: /\+ create user/i }));
    expect(screen.queryByLabelText("Make an organization administrator")).not.toBeInTheDocument();
  });
});
