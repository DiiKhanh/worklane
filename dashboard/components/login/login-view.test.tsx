import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { LoginView } from "./login-view";

const push = vi.fn();
vi.mock("next/navigation", () => ({ useRouter: () => ({ push }) }));
vi.mock("@/lib/api/auth", () => ({
  login: vi.fn().mockResolvedValue({
    token: "tok",
    expiresAt: "",
    user: { id: "u1", email: "a@b.co", tenantId: "t1" },
  }),
}));

describe("LoginView", () => {
  beforeEach(() => {
    push.mockClear();
    localStorage.clear();
  });

  it("logs in, stores the token, and redirects", async () => {
    render(<LoginView />);
    fireEvent.change(screen.getByLabelText("Email"), { target: { value: "a@b.co" } });
    fireEvent.change(screen.getByLabelText("Password"), { target: { value: "pw" } });
    fireEvent.click(screen.getByRole("button", { name: /sign in/i }));
    await waitFor(() => expect(push).toHaveBeenCalledWith("/"));
    expect(localStorage.getItem("worklane-token")).toBe("tok");
  });
});
