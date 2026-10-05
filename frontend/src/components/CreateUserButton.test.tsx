import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { CreateUserButton } from "./CreateUserButton";
import * as networking from "./networking";

vi.mock("./networking", () => ({
  userCreateCall: vi.fn(),
  teamMemberAddCall: vi.fn(),
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

const mockUserCreateCall = vi.mocked(networking.userCreateCall);
const mockTeamMemberAddCall = vi.mocked(networking.teamMemberAddCall);

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

describe("CreateUserButton", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockUserCreateCall.mockResolvedValue({ user_id: "user-new" });
    mockTeamMemberAddCall.mockResolvedValue({});
  });

  it("opens a create form that offers only the two account roles", async () => {
    const user = userEvent.setup();
    renderButton();
    await user.click(screen.getByRole("button", { name: /\+ create user/i }));

    expect(screen.getByRole("dialog", { name: /create user/i })).toBeInTheDocument();
    expect(screen.queryByText(/team\/org specific roles/i)).not.toBeInTheDocument();
    expect(screen.queryByLabelText("Organization")).not.toBeInTheDocument();
    await user.click(screen.getByRole("combobox", { name: /account role/i }));
    expect(screen.getByRole("option", { name: /Regular user/ })).toBeInTheDocument();
    expect(screen.getByRole("option", { name: /Platform administrator/ })).toBeInTheDocument();
  });

  it("creates a regular user and does not add a team when none is chosen", async () => {
    const user = userEvent.setup();
    renderButton(true);
    await fillAccount(user);
    await user.click(screen.getByRole("button", { name: /create user/i }));

    await waitFor(() => expect(mockUserCreateCall).toHaveBeenCalled());
    expect(mockUserCreateCall.mock.calls[0][2]).toMatchObject({
      user_email: "new@example.com",
      password: "password1",
      user_role: "user",
    });
    expect(mockTeamMemberAddCall).not.toHaveBeenCalled();
  });

  it("refuses a password shorter than 8 characters", async () => {
    const user = userEvent.setup();
    renderButton(true);
    await user.type(screen.getByLabelText("User Email"), "new@example.com");
    await user.type(screen.getByLabelText("Initial password"), "short");
    await user.click(screen.getByRole("button", { name: /create user/i }));

    expect(mockUserCreateCall).not.toHaveBeenCalled();
  });

  it("adds the new account to the chosen team as a team admin", async () => {
    const user = userEvent.setup();
    renderButton();
    await user.click(screen.getByRole("button", { name: /\+ create user/i }));
    await fillAccount(user);
    await user.selectOptions(screen.getByLabelText("Add to a team"), "team-1");
    await user.click(await screen.findByRole("combobox", { name: "Role in this team" }));
    await user.click(await screen.findByRole("option", { name: /Team admin/ }));
    await user.click(screen.getByRole("button", { name: /create user/i }));

    await waitFor(() => expect(mockTeamMemberAddCall).toHaveBeenCalled());
    expect(mockTeamMemberAddCall).toHaveBeenCalledWith("token", "team-1", {
      user_email: "new@example.com",
      user_id: "user-new",
      role: "admin",
    });
  });
});
