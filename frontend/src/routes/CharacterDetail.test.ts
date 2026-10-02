import { afterEach, describe, expect, it, vi } from "vitest";
import { mount, unmount } from "svelte";
import CharacterDetail from "./CharacterDetail.svelte";
import type { Character, History } from "$lib/api";

const character: Character = {
  id: 7,
  name: "Alpha",
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

const history: History = {
  from: "2026-01-01T00:00:00Z",
  to: "2026-01-02T00:00:00Z",
  retentionDays: 90,
  comparison: null,
  snapshots: [],
};

function response(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), { status });
}

describe("CharacterDetail history", () => {
  let host: HTMLDivElement;
  let component: ReturnType<typeof mount>;

  afterEach(() => {
    if (component) unmount(component);
    host?.remove();
    vi.unstubAllGlobals();
  });

  it("shows a loading state until history is available, then renders its empty branch", async () => {
    let resolveHistory!: (value: Response) => void;
    const pendingHistory = new Promise<Response>((resolve) => {
      resolveHistory = resolve;
    });
    vi.stubGlobal("fetch", (path: string) =>
      Promise.resolve(path === "/api/characters/Alpha" ? response(character) : pendingHistory),
    );
    host = document.createElement("div");
    document.body.append(host);
    component = mount(CharacterDetail, { target: host, props: { name: "Alpha" } });

    await vi.waitFor(() => expect(host.textContent).toContain("Loading expedition history"));

    resolveHistory(response(history));
    await vi.waitFor(() => expect(host.textContent).toContain("No snapshots in this date range."));
  });

  it("keeps character details visible when the history route fails", async () => {
    vi.stubGlobal("fetch", (path: string) =>
      Promise.resolve(
        path === "/api/characters/Alpha"
          ? response(character)
          : response({ error: "unavailable" }, 500),
      ),
    );
    host = document.createElement("div");
    document.body.append(host);
    component = mount(CharacterDetail, { target: host, props: { name: "Alpha" } });

    await vi.waitFor(() => expect(host.querySelector("h1")?.textContent).toBe("Alpha"));
    expect(host.textContent).toContain("unavailable");
  });

  it("restores the roster query from the character link", async () => {
    window.history.pushState({}, "", "/characters/Alpha?roster=%3Fclass%3DMage%26page%3D2");
    vi.stubGlobal("fetch", (path: string) =>
      Promise.resolve(path === "/api/characters/Alpha" ? response(character) : response(history)),
    );
    host = document.createElement("div");
    document.body.append(host);
    component = mount(CharacterDetail, { target: host, props: { name: "Alpha" } });

    await vi.waitFor(() =>
      expect(host.querySelector('a[href="/roster?class=Mage&page=2"]')).not.toBeNull(),
    );
  });

  it("skips malformed history entries without breaking the snapshot table", async () => {
    vi.stubGlobal("fetch", (path: string) =>
      Promise.resolve(
        path === "/api/characters/Alpha"
          ? response(character)
          : response({
              ...history,
              snapshots: [
                null,
                {},
                {
                  id: 1,
                  characterId: 7,
                  capturedAt: "2026-01-01T00:00:00Z",
                  itemLevel: 620,
                  mythicRating: 2500,
                  bestKeyLevel: 12,
                },
              ],
            }),
      ),
    );
    host = document.createElement("div");
    document.body.append(host);
    component = mount(CharacterDetail, { target: host, props: { name: "Alpha" } });

    await vi.waitFor(() => expect(host.querySelector("h1")?.textContent).toBe("Alpha"));
    await vi.waitFor(() => expect(host.querySelectorAll("tbody tr")).toHaveLength(1));
    expect(host.querySelectorAll("select")).toHaveLength(0);
  });
});
