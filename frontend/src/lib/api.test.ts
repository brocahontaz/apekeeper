import { describe, it, expect, vi } from "vitest";
import { api, ApiError } from "./api";
import { get } from "svelte/store";
import { sessionExpired, currentUser } from "./stores";
describe("api", () => {
  it("marks session expired on 401", async () => {
    vi.stubGlobal(
      "fetch",
      vi
        .fn()
        .mockResolvedValue(
          new Response(JSON.stringify({ error: "unauthenticated" }), { status: 401 }),
        ),
    );
    await expect(api("/api/dashboard")).rejects.toBeInstanceOf(ApiError);
    expect(get(sessionExpired)).toBe(true);
  });
  it("navigates to /login via popstate and clears user on 401", async () => {
    window.history.replaceState({}, "", "/");
    currentUser.set({ id: 1, displayName: "x", battletag: "x#1", appRole: "member" });
    let pops = 0;
    const listener = () => pops++;
    window.addEventListener("popstate", listener);
    vi.stubGlobal(
      "fetch",
      vi
        .fn()
        .mockResolvedValue(
          new Response(JSON.stringify({ error: "unauthenticated" }), { status: 401 }),
        ),
    );
    await expect(api("/api/dashboard")).rejects.toBeInstanceOf(ApiError);
    window.removeEventListener("popstate", listener);
    expect(location.pathname).toBe("/login");
    expect(pops).toBe(1);
    expect(get(currentUser)).toBe(null);
  });
});
