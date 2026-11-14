import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { RequestsView } from "./requests-view";

describe("Requests navigation", () => {
  it("restores the page, sorting and focused row when returning from detail", async () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(<QueryClientProvider client={client}><RequestsView /></QueryClientProvider>);
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: "Recipient" }));
    await user.click(screen.getByRole("button", { name: "Next" }));
    const row = screen.getAllByRole("button").find((element) => element.tagName === "TR")!;
    const requestId = within(row).getByText(/^req_/).textContent!;
    row.focus();
    await user.keyboard("{Enter}");
    expect(screen.getByRole("heading", { name: "Request detail" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "OTP requests" })).toHaveFocus();
    await user.click(screen.getByRole("button", { name: "OTP requests" }));
    expect(screen.getByText(/Page 2 of/)).toBeInTheDocument();
    expect(within(row).getByText(requestId)).toBeInTheDocument();
    expect(row).toHaveFocus();
    expect(screen.getByRole("button", { name: "Recipient" }).querySelector(".lucide-arrow-up")).toBeInTheDocument();
  });
});
