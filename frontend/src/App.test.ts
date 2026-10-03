import { afterEach, describe, expect, it, vi } from "vitest";
import { mount, unmount } from "svelte";
import App from "./App.svelte";
import { currentUser, selectedGuildRole } from "$lib/stores";
import { navigate } from "$lib/router";

const historyPayload = {
  from: "2026-01-01T00:00:00Z",
  to: "2026-01-02T00:00:00Z",
  retentionDays: 90,
  comparison: null,
  snapshots: [],
};
const rosterPayload = { items: [], total: 0, page: 1, pageSize: 25, classes: [], specs: [] };

function character(name: string) {
  return {
    id: name === "Alpha" ? 7 : 8,
    name,
    realm: "Area 52",
    classId: 1,
    className: "Warrior",
    specId: 71,
    specName: "Arms",
    level: 80,
    itemLevel: 620,
    mythicRating: 2500,
    bestKeyLevel: 12,
    stale: false,
  };
}

describe("App resilience and navigation", () => {
  let host: HTMLDivElement;
  let component: ReturnType<typeof mount>;

  afterEach(() => {
    if (component) unmount(component);
    host?.remove();
    currentUser.set(null);
    selectedGuildRole.set(null);
    history.replaceState({}, "", "/");
    vi.unstubAllGlobals();
  });

  it("reloads character data when navigation changes the character URL", async () => {
    history.replaceState({}, "", "/characters/Alpha");
    const requests: string[] = [];
    let resolveAlpha!: (response: Response) => void;
    const alpha = new Promise<Response>((resolve) => (resolveAlpha = resolve));
    vi.stubGlobal("fetch", (input: string) => {
      requests.push(input);
      if (input === "/api/auth/me")
        return Promise.resolve(
          new Response(
            JSON.stringify({ id: 1, displayName: "Keeper", battletag: "k#1", appRole: "member" }),
          ),
        );
      if (input === "/api/guilds") return Promise.resolve(new Response("[]"));
      if (input.includes("/history"))
        return Promise.resolve(new Response(JSON.stringify(historyPayload)));
      if (input === "/api/characters/Alpha") return alpha;
      return Promise.resolve(
        new Response(JSON.stringify(character(input.endsWith("Beta") ? "Beta" : "Alpha"))),
      );
    });
    host = document.createElement("div");
    document.body.append(host);
    component = mount(App, { target: host });

    await vi.waitFor(() => expect(host.querySelector("main")).not.toBeNull());
    navigate("/characters/Beta");
    await vi.waitFor(() => expect(host.querySelector("h1")?.textContent).toBe("Beta"));
    resolveAlpha(new Response(JSON.stringify(character("Alpha"))));
    await Promise.resolve();
    expect(host.querySelector("h1")?.textContent).toBe("Beta");
    expect(requests).toContain("/api/characters/Beta");
  });

  it("exposes logout failures instead of leaving a rejected promise", async () => {
    history.replaceState({}, "", "/");
    vi.stubGlobal("fetch", (input: string) => {
      if (input === "/api/auth/logout") return Promise.reject(new Error("network down"));
      if (input === "/api/auth/me")
        return Promise.resolve(
          new Response(
            JSON.stringify({ id: 1, displayName: "Keeper", battletag: "k#1", appRole: "member" }),
          ),
        );
      if (input === "/api/guilds") return Promise.resolve(new Response("[]"));
      if (input === "/api/dashboard")
        return Promise.resolve(new Response(JSON.stringify({ rosterSize: 0 })));
      return Promise.resolve(new Response(JSON.stringify({})));
    });
    host = document.createElement("div");
    document.body.append(host);
    component = mount(App, { target: host });

    await vi.waitFor(() => expect(host.querySelector("button")?.textContent).toBe("Logout"));
    (host.querySelector("button") as HTMLButtonElement).click();
    await vi.waitFor(() =>
      expect(host.querySelector('[role="alert"]')?.textContent).toContain("network down"),
    );
  });

  it("announces route changes and moves focus to the main target", async () => {
    history.replaceState({}, "", "/");
    vi.stubGlobal("fetch", (input: string) => {
      if (input === "/api/auth/me")
        return Promise.resolve(
          new Response(
            JSON.stringify({ id: 1, displayName: "Keeper", battletag: "k#1", appRole: "member" }),
          ),
        );
      if (input === "/api/guilds") return Promise.resolve(new Response("[]"));
      if (input === "/api/dashboard")
        return Promise.resolve(new Response(JSON.stringify({ rosterSize: 0 })));
      if (input.startsWith("/api/roster"))
        return Promise.resolve(new Response(JSON.stringify(rosterPayload)));
      return Promise.resolve(new Response(JSON.stringify({})));
    });
    host = document.createElement("div");
    document.body.append(host);
    component = mount(App, { target: host });

    await vi.waitFor(() => expect(host.querySelector("main[tabindex='-1']")).not.toBeNull());
    navigate("/roster");
    await vi.waitFor(() =>
      expect(host.querySelector('[role="status"]')?.textContent).toContain("Roster loaded"),
    );
    await vi.waitFor(() => expect(document.activeElement).toBe(host.querySelector("main")));
  });

  it("shows guild-switch failures accessibly", async () => {
    history.replaceState({}, "", "/roster");
    vi.stubGlobal("fetch", (input: string) => {
      if (input === "/api/auth/me")
        return Promise.resolve(
          new Response(
            JSON.stringify({ id: 1, displayName: "Keeper", battletag: "k#1", appRole: "member" }),
          ),
        );
      if (input === "/api/guilds")
        return Promise.resolve(
          new Response(
            JSON.stringify([
              { id: 1, slug: "one", name: "One", realm: "A", region: "us", role: "member" },
              { id: 2, slug: "two", name: "Two", realm: "B", region: "us", role: "member" },
            ]),
          ),
        );
      if (input === "/api/guilds/select") return Promise.reject(new Error("switch unavailable"));
      if (input.startsWith("/api/roster"))
        return Promise.resolve(new Response(JSON.stringify(rosterPayload)));
      return Promise.resolve(new Response(JSON.stringify({})));
    });
    host = document.createElement("div");
    document.body.append(host);
    component = mount(App, { target: host });

    await vi.waitFor(() => expect(host.querySelector("select")).not.toBeNull());
    const select = host.querySelector("select") as HTMLSelectElement;
    select.value = "two";
    select.dispatchEvent(new Event("change", { bubbles: true }));
    await vi.waitFor(() =>
      expect(host.querySelector('[role="alert"]')?.textContent).toContain("switch unavailable"),
    );
    expect(select.value).toBe("one");
  });

  it("displays the selected guild membership role", async () => {
    history.replaceState({}, "", "/");
    vi.stubGlobal("fetch", (input: string) => {
      if (input === "/api/auth/me")
        return Promise.resolve(
          new Response(
            JSON.stringify({ id: 1, displayName: "Keeper", battletag: "k#1", appRole: "admin" }),
          ),
        );
      if (input === "/api/guilds")
        return Promise.resolve(
          new Response(
            JSON.stringify([
              {
                id: 1,
                slug: "one",
                name: "One",
                realm: "A",
                region: "us",
                role: "officer",
                selected: true,
              },
            ]),
          ),
        );
      if (input === "/api/dashboard")
        return Promise.resolve(new Response(JSON.stringify({ rosterSize: 0 })));
      return Promise.resolve(new Response(JSON.stringify({})));
    });
    host = document.createElement("div");
    document.body.append(host);
    component = mount(App, { target: host });
    await vi.waitFor(() => expect(host.textContent).toContain("Officer"));
    expect(host.textContent).not.toContain("Guild Master");
  });
});
