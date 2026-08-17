import { describe, expect, it } from "vitest";
import { renderPreview, splitVars } from "./template-render";

describe("renderPreview", () => {
  it("substitutes code, name, and link tokens", () => {
    const out = renderPreview({
      subject: "Your code: {{code}}",
      body: 'Hi {{name}}, open {{link "https://x"}}',
    });
    expect(out.subject).toBe("Your code: 418209");
    expect(out.body).toBe("Hi Daniel, open wl.link/7Xq2Ab");
  });
});

describe("splitVars", () => {
  it("marks {{...}} segments as variables", () => {
    const parts = splitVars("a {{code}} b");
    expect(parts.filter((p) => p.isVar).map((p) => p.text)).toEqual(["{{code}}"]);
  });
});
