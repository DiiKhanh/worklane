import { describe, it, expect } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { SendForm } from "./send-form";

function renderWithClient(ui: ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}>{ui}</QueryClientProvider>);
}

describe("SendForm", () => {
  it("shows a validation error for an invalid email", async () => {
    const user = userEvent.setup();
    renderWithClient(<SendForm />);
    await user.type(screen.getByLabelText(/recipient email/i), "not-an-email");
    await user.click(screen.getByRole("button", { name: /send code/i }));
    expect(
      await screen.findByText(/valid email address/i),
    ).toBeInTheDocument();
  });

  it("issues a code and shows the returned request id", async () => {
    const user = userEvent.setup();
    renderWithClient(<SendForm />);
    await user.type(screen.getByLabelText(/recipient email/i), "dev@worklane.io");
    await user.click(screen.getByRole("button", { name: /send code/i }));
    await waitFor(() =>
      expect(screen.getByText(/^req_/)).toBeInTheDocument(),
    );
  });
});
