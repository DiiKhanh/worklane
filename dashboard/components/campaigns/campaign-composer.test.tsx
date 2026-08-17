import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { CampaignComposer } from "./campaign-composer";

describe("CampaignComposer", () => {
  it("shows recipient scope for the default audience and enables send", () => {
    render(<CampaignComposer onBack={vi.fn()} onQueue={vi.fn()} />);
    // Newsletter subscribers = 8241 in scope.
    expect(screen.getByText("8,241")).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: /send campaign/i }),
    ).toBeEnabled();
  });
});
