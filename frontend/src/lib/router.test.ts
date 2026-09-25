import { describe, it, expect } from "vitest";
import { match } from "./router";
describe("router", () =>
  it("matches parameters and query", () => {
    const r = match([{ path: "/characters/{id}", component: null }], "/characters/7?tab=history")!;
    expect(r.params.id).toBe("7");
    expect(r.query.get("tab")).toBe("history");
  }));
