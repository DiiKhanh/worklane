import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { LinksView } from "./links-view";

describe("LinksView", () => {
  it("renders the roadmap note and the Links heading", () => {
    render(<LinksView />);
    expect(screen.getByText(/Roadmap screen/)).toBeInTheDocument();
    expect(
      screen.getByRole("heading", { name: "Links" }),
    ).toBeInTheDocument();
  });
});
