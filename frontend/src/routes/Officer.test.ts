import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { flushSync, mount, unmount } from "svelte";
import Officer from "$routes/Officer.svelte";
import { currentUser } from "$lib/stores";

const response = (body: unknown) =>
  Promise.resolve(new Response(JSON.stringify(body), { status: 200 }));
describe("Officer workflow", () => {
  let host: HTMLDivElement;
  let component: ReturnType<typeof mount>;
  beforeEach(() => {
    host = document.createElement("div");
    document.body.appendChild(host);
    currentUser.set({ id: 1, displayName: "Officer", battletag: "officer#1", appRole: "officer" });
  });
  afterEach(() => {
    unmount(component);
    host.remove();
    currentUser.set(null);
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });
  it("renders an accessible empty queue", async () => {
    vi.stubGlobal("fetch", (path: string) =>
      response(path.startsWith("/api/officer/queue") ? { items: [] } : []),
    );
    component = mount(Officer, { target: host });
    await new Promise((resolve) => setTimeout(resolve));
    flushSync();
    expect(host.textContent).toContain("No characters need review.");
    expect(host.querySelector("[aria-live='polite']")).not.toBeNull();
  });
  it("confirms and submits bounded bulk review actions", async () => {
    const fetch = vi.fn((path: string, init?: RequestInit) =>
      response(
        path.startsWith("/api/officer/queue")
          ? { items: [{ characterId: 3, name: "Alpha", reason: "stale", tags: [] }] }
          : path === "/api/officer/activity"
            ? []
            : { completed: 1 },
      ),
    );
    vi.stubGlobal("fetch", fetch);
    vi.stubGlobal("confirm", () => true);
    component = mount(Officer, { target: host });
    await new Promise((resolve) => setTimeout(resolve));
    flushSync();
    (host.querySelector("input[type=checkbox]") as HTMLInputElement).click();
    flushSync();
    [...host.querySelectorAll("button")]
      .find((b) => b.textContent?.includes("Mark reviewed"))!
      .click();
    await new Promise((resolve) => setTimeout(resolve));
    expect(fetch).toHaveBeenCalledWith(
      "/api/officer/queue/complete",
      expect.objectContaining({ method: "POST" }),
    );
    expect(
      fetch.mock.calls.find(([path]) => path === "/api/officer/queue/complete")?.[1]?.body,
    ).toBe(JSON.stringify({ characterIds: [3], syncRunIds: [], confirm: true }));
  });
  it("allows a failed sync run to be selected and completed", async () => {
    const fetch = vi.fn((path: string, _init?: RequestInit) =>
      response(
        path.startsWith("/api/officer/queue")
          ? {
              items: [
                { syncRunId: 9, name: "Sync run #9", reason: "recent_sync_failure", tags: [] },
              ],
            }
          : path === "/api/officer/activity"
            ? []
            : { completed: 1 },
      ),
    );
    vi.stubGlobal("fetch", fetch);
    vi.stubGlobal("confirm", () => true);
    component = mount(Officer, { target: host });
    await new Promise((resolve) => setTimeout(resolve));
    flushSync();
    (host.querySelector("input[type=checkbox]") as HTMLInputElement).click();
    flushSync();
    [...host.querySelectorAll("button")]
      .find((b) => b.textContent?.includes("Mark reviewed"))!
      .click();
    await new Promise((resolve) => setTimeout(resolve));
    expect(
      fetch.mock.calls.find(([path]) => path === "/api/officer/queue/complete")?.[1]?.body,
    ).toBe(JSON.stringify({ characterIds: [], syncRunIds: [9], confirm: true }));
  });
  it("loads selected metadata and preserves it when saving a note", async () => {
    const fetch = vi.fn((path: string, init?: RequestInit) =>
      response(
        path.startsWith("/api/officer/queue")
          ? { items: [{ characterId: 3, name: "Alpha", reason: "stale", tags: ["queue"] }] }
          : path === "/api/officer/activity"
            ? [
                {
                  id: 1,
                  actor: "Officer",
                  action: "character_update",
                  targetId: 3,
                  createdAt: "2026-01-01T00:00:00Z",
                  changes: { noteChanged: true },
                },
              ]
            : path === "/api/officer/characters/3"
              ? {
                  id: 3,
                  note: "existing",
                  lifecycleStatus: "active",
                  tags: ["core"],
                  noteAuthor: "Officer",
                }
              : { updated: true },
      ),
    );
    vi.stubGlobal("fetch", fetch);
    component = mount(Officer, { target: host });
    await new Promise((resolve) => setTimeout(resolve));
    (host.querySelector("input[type=checkbox]") as HTMLInputElement).click();
    await new Promise((resolve) => setTimeout(resolve));
    flushSync();
    const textarea = host.querySelector("textarea") as HTMLTextAreaElement;
    expect(textarea.value).toBe("existing");
    expect(host.textContent).toContain("target #3");
    textarea.value = "changed";
    textarea.dispatchEvent(new Event("input", { bubbles: true }));
    [...host.querySelectorAll("button")]
      .find((b) => b.textContent?.includes("Save details"))!
      .click();
    await new Promise((resolve) => setTimeout(resolve));
    const save = fetch.mock.calls.find(
      ([path, init]) => path === "/api/officer/characters/3" && init?.method === "PUT",
    )?.[1] as RequestInit;
    expect(save.body).toBe(
      JSON.stringify({ note: "changed", lifecycleStatus: "active", tags: ["core"] }),
    );
  });
});
