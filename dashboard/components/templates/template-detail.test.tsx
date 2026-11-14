import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { getDataSource } from "@/lib/api";
import { qk } from "@/lib/queries/keys";
import { TemplateDetail } from "./template-detail";

afterEach(() => vi.restoreAllMocks());

async function openEditor() {
  const detail = await getDataSource().getTemplate("tpl_email_en");
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  client.setQueryData(qk.template("tpl_email_en"), detail);
  render(
    <QueryClientProvider client={client}>
      <TemplateDetail template={detail.template} onBack={() => {}} />
    </QueryClientProvider>,
  );
  await screen.findByLabelText("Body");
  return userEvent.setup();
}

describe("Template editor feedback", () => {
  it("tracks unsaved changes and confirms a saved draft", async () => {
    const user = await openEditor();
    expect(screen.getByRole("button", { name: "Save draft" })).toBeDisabled();
    await user.type(screen.getByLabelText("Body"), " Updated.");
    expect(screen.getByText("Unsaved changes")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Save draft" }));
    expect(await screen.findByText(/Draft v\d+ saved/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Save draft" })).toBeDisabled();
  });

  it("shows preview failure instead of presenting unrendered text as delivery output", async () => {
    vi.spyOn(getDataSource(), "previewTemplate").mockRejectedValue(new Error("unavailable"));
    await openEditor();
    expect(await screen.findByRole("alert")).toHaveTextContent("Preview unavailable");
    const preview = screen.getByText("Preview", { selector: "h3" }).closest("section")!;
    expect(within(preview).queryByText(/\{\{code\}\}/)).not.toBeInTheDocument();
  });

  it("preserves unsaved edits when publishing is blocked and reports publish failures", async () => {
    await getDataSource().addVersion("tpl_email_en", { subject: "Draft", body: "Code: {{code}}", note: "feedback test" });
    const user = await openEditor();
    const publish = screen.getAllByRole("button", { name: /Make v\d+ live/ })[0];
    await user.type(screen.getByLabelText("Body"), " Changed.");
    expect(publish).toBeDisabled();
    await user.click(screen.getByRole("button", { name: "Save draft" }));
    await screen.findByText(/Draft v\d+ saved/);
    vi.spyOn(getDataSource(), "publishVersion").mockRejectedValueOnce(new Error("unavailable"));
    await user.click(publish);
    expect(await screen.findByRole("alert")).toHaveTextContent("Could not publish this version");
    expect(screen.getByDisplayValue(/Changed\./)).toBe(screen.getByLabelText("Body"));
  });
});
