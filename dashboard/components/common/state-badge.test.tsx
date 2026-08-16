import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { StateBadge } from "./state-badge";

describe("StateBadge", () => {
  it("renders a readable label for each state", () => {
    const states = ["requested", "sent", "verified", "failed", "expired"] as const;
    for (const s of states) {
      const { unmount } = render(<StateBadge state={s} />);
      expect(screen.getByText(s)).toBeInTheDocument();
      unmount();
    }
  });

  it("maps api-key statuses to a colored pill", () => {
    render(<StateBadge state="active" />);
    const el = screen.getByText("active");
    expect(el.closest("[data-state]")).toHaveAttribute("data-state", "active");
  });
});
