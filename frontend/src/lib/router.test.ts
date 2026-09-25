import { describe, it, expect } from "vitest";
import { match } from "./router";
describe("router", () =>
  it("matches parameters and query", () => {
    const r = match(
      [{ path: "/characters/{name}", component: null }],
      "/characters/Elune%20Ape?tab=history",
    )!;
    expect(r.params.name).toBe("Elune Ape");
    expect(r.query.get("tab")).toBe("history");
  }));
