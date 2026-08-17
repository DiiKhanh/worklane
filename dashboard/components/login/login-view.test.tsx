import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { LoginView } from "./login-view";

describe("LoginView", () => {
  it("moves from email step to code step after submitting an email", async () => {
    render(<LoginView />);
    await userEvent.type(
      screen.getByLabelText(/work email/i),
      "you@company.com",
    );
    await userEvent.click(screen.getByRole("button", { name: /send code/i }));
    expect(screen.getByText(/Enter your code/i)).toBeInTheDocument();
    expect(screen.getByText(/Roadmap screen/)).toBeInTheDocument();
  });
});
