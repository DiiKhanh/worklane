import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { getDataSource } from "@/lib/api";
import { Playground } from "./playground";

afterEach(() => vi.restoreAllMocks());

function renderPlayground() {
  const client = new QueryClient();
  render(
    <QueryClientProvider client={client}>
      <Playground />
    </QueryClientProvider>,
  );
  return userEvent.setup();
}

describe("Playground feedback", () => {
  it("reports a send failure without exposing the underlying error", async () => {
    vi.spyOn(getDataSource(), "send").mockRejectedValueOnce(new Error("private upstream details"));
    const user = renderPlayground();
    await user.type(screen.getAllByLabelText("Recipient email")[0], "send@example.com");
    await user.click(screen.getByRole("button", { name: "Send code" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Could not send the code");
    expect(screen.queryByText(/private upstream/)).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Send code" })).toBeEnabled();
  });

  it("reports a verification failure and permits retry", async () => {
    vi.spyOn(getDataSource(), "verify").mockRejectedValueOnce(new Error("upstream unavailable"));
    const user = renderPlayground();
    await user.type(screen.getAllByLabelText("Recipient email")[1], "verify@example.com");
    await user.type(screen.getByLabelText("6-digit code"), "123456");
    await user.click(screen.getByRole("button", { name: "Verify code" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Could not verify the code");
    expect(screen.getByRole("button", { name: "Verify code" })).toBeEnabled();
  });

  it("keeps the issued code paired with the submitted recipient", async () => {
    const user = renderPlayground();
    const recipient = screen.getAllByLabelText("Recipient email")[0];
    await user.type(recipient, "original@example.com");
    await user.click(screen.getByRole("button", { name: "Send code" }));
    const useCode = await screen.findByRole("button", { name: "Use this code to verify" });
    await user.clear(recipient);
    await user.type(recipient, "different@example.com");
    await user.click(useCode);
    expect(screen.getAllByLabelText("Recipient email")[1]).toHaveValue("original@example.com");
    await user.click(screen.getByRole("button", { name: "Verify code" }));
    expect(await screen.findByText("Verified")).toBeInTheDocument();
    expect(within(screen.getByRole("status", { name: "Verification result" })).getByText("Verified")).toBeInTheDocument();
  });
});
