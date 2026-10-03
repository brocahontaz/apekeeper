import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { flushSync, mount, unmount } from "svelte";
import Officer from "$routes/Officer.svelte";
import { currentUser, selectedGuildRole } from "$lib/stores";

const response = (body: unknown) =>
  Promise.resolve(new Response(JSON.stringify(body), { status: 200 }));
const deferred = <T>() => {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  return { promise: new Promise<T>((r, j) => ((resolve = r), (reject = j))), resolve, reject };
};
describe("Officer workflow", () => {
  let host: HTMLDivElement;
  let component: ReturnType<typeof mount> | undefined;
  beforeEach(() => {
    host = document.createElement("div");
    document.body.appendChild(host);
    currentUser.set({ id: 1, displayName: "Officer", battletag: "officer#1", appRole: "officer" });
  });
  afterEach(() => {
    if (component) unmount(component);
    host.remove();
    currentUser.set(null);
    selectedGuildRole.set(null);
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
  it("hides officer workflow for a member of the selected guild", async () => {
    currentUser.set({ id: 1, displayName: "Admin elsewhere", battletag: "a#1", appRole: "admin" });
    selectedGuildRole.set("member");
    component = mount(Officer, { target: host });
    flushSync();
    expect(host.textContent).toContain("Officer access is required.");
    expect(host.querySelector("button")).toBeNull();
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
  it("ignores metadata for a selection that has since changed", async () => {
    let resolveFirst!: (value: Response) => void;
    let resolveSecond!: (value: Response) => void;
    vi.stubGlobal("fetch", (path: string) => {
      if (path === "/api/officer/characters/3")
        return new Promise<Response>((resolve) => (resolveFirst = resolve));
      if (path === "/api/officer/characters/4")
        return new Promise<Response>((resolve) => (resolveSecond = resolve));
      return response(
        path.startsWith("/api/officer/queue")
          ? {
              items: [
                { characterId: 3, name: "Alpha", reason: "stale", tags: [] },
                { characterId: 4, name: "Beta", reason: "stale", tags: [] },
              ],
            }
          : [],
      );
    });
    component = mount(Officer, { target: host });
    await new Promise((resolve) => setTimeout(resolve));
    const checkboxes = host.querySelectorAll("input[type=checkbox]");
    (checkboxes[0] as HTMLInputElement).click();
    flushSync();
    (checkboxes[1] as HTMLInputElement).click();
    flushSync();
    (checkboxes[0] as HTMLInputElement).click();
    flushSync();
    await new Promise((resolve) => setTimeout(resolve));
    resolveSecond(
      new Response(JSON.stringify({ id: 4, note: "current", lifecycleStatus: "active", tags: [] })),
    );
    await new Promise((resolve) => setTimeout(resolve));
    flushSync();
    expect((host.querySelector("textarea") as HTMLTextAreaElement).value).toBe("current");
    resolveFirst(
      new Response(JSON.stringify({ id: 3, note: "stale", lifecycleStatus: "retired", tags: [] })),
    );
    await new Promise((resolve) => setTimeout(resolve));
    flushSync();
    expect((host.querySelector("textarea") as HTMLTextAreaElement).value).toBe("current");
  });

  it("keeps the newest queue load state when an older refresh settles", async () => {
    const firstQueue = deferred<Response>();
    const secondQueue = deferred<Response>();
    let queueCalls = 0;
    vi.stubGlobal("fetch", (path: string) => {
      if (path.startsWith("/api/officer/queue"))
        return (++queueCalls === 1 ? firstQueue : secondQueue).promise;
      return response([]);
    });
    component = mount(Officer, { target: host });
    await new Promise((resolve) => setTimeout(resolve));
    host.querySelector("button")!.click();
    secondQueue.resolve(
      new Response(
        JSON.stringify({ items: [{ characterId: 8, name: "Current", reason: "stale", tags: [] }] }),
        {
          status: 200,
        },
      ),
    );
    await new Promise((resolve) => setTimeout(resolve));
    flushSync();
    expect(host.textContent).toContain("Current");
    firstQueue.reject(new Error("stale refresh failure"));
    await new Promise((resolve) => setTimeout(resolve));
    flushSync();
    expect(host.textContent).toContain("Current");
    expect(host.textContent).not.toContain("stale refresh failure");
    expect(host.textContent).not.toContain("Loading officer queue…");
  });

  it("suppresses a selected-character failure after selection and teardown change", async () => {
    const character = deferred<Response>();
    vi.stubGlobal("fetch", (path: string) =>
      path === "/api/officer/characters/3"
        ? character.promise
        : response(
            path.startsWith("/api/officer/queue")
              ? { items: [{ characterId: 3, name: "Alpha", reason: "stale", tags: [] }] }
              : [],
          ),
    );
    component = mount(Officer, { target: host });
    await new Promise((resolve) => setTimeout(resolve));
    (host.querySelector("input[type=checkbox]") as HTMLInputElement).click();
    flushSync();
    expect(host.textContent).toContain("Loading character details…");
    (host.querySelector("input[type=checkbox]") as HTMLInputElement).click();
    flushSync();
    unmount(component!);
    component = undefined;
    character.reject(new Error("stale character failure"));
    await new Promise((resolve) => setTimeout(resolve));
    expect(host.textContent).not.toContain("stale character failure");
  });

  it("ignores stale mutation success and failure after a newer review", async () => {
    const first = deferred<Response>();
    const second = deferred<Response>();
    const third = deferred<Response>();
    const fourth = deferred<Response>();
    let mutations = 0;
    vi.stubGlobal("fetch", (path: string, init?: RequestInit) => {
      if (path === "/api/officer/bulk-tags")
        return [first, second, third, fourth][mutations++].promise;
      if (path.startsWith("/api/officer/characters/"))
        return response({ id: 3, note: "", lifecycleStatus: "active", tags: [] });
      return response(
        path.startsWith("/api/officer/queue")
          ? { items: [{ characterId: 3, name: "Alpha", reason: "stale", tags: [] }] }
          : [],
      );
    });
    vi.stubGlobal("confirm", () => true);
    component = mount(Officer, { target: host });
    await new Promise((resolve) => setTimeout(resolve));
    (host.querySelector("input[type=checkbox]") as HTMLInputElement).click();
    flushSync();
    const add = [...host.querySelectorAll("button")].find((b) => b.textContent === "Add tags")!;
    add.click();
    add.click();
    await vi.waitFor(() => expect(mutations).toBe(2));
    second.resolve(new Response(JSON.stringify({ updated: 2 }), { status: 200 }));
    await new Promise((resolve) => setTimeout(resolve));
    flushSync();
    expect(host.textContent).toContain("Updated 2 characters.");
    first.resolve(new Response(JSON.stringify({ updated: 1 }), { status: 200 }));
    await new Promise((resolve) => setTimeout(resolve));
    flushSync();
    expect(host.textContent).toContain("Updated 2 characters.");
    expect(host.textContent).not.toContain("Update failed");
    (host.querySelector("input[type=checkbox]") as HTMLInputElement).click();
    flushSync();
    const secondAdd = [...host.querySelectorAll("button")].find(
      (b) => b.textContent === "Add tags",
    )!;
    secondAdd.click();
    secondAdd.click();
    await vi.waitFor(() => expect(mutations).toBe(4));
    fourth.resolve(new Response(JSON.stringify({ updated: 4 }), { status: 200 }));
    await new Promise((resolve) => setTimeout(resolve));
    third.reject(new Error("stale update failure"));
    await new Promise((resolve) => setTimeout(resolve));
    flushSync();
    expect(host.textContent).toContain("Updated 4 characters.");
    expect(host.textContent).not.toContain("stale update failure");
  });

  it("does not let a refresh started during a mutation replace its newer result", async () => {
    const refresh = deferred<Response>();
    const mutation = deferred<Response>();
    let queueCalls = 0;
    vi.stubGlobal("fetch", (path: string, init?: RequestInit) => {
      if (path.startsWith("/api/officer/queue")) {
        queueCalls++;
        if (queueCalls === 2) return refresh.promise;
        return response({
          items: [
            {
              characterId: 3,
              name: queueCalls === 1 ? "Initial" : "Saved",
              reason: "stale",
              tags: [],
            },
          ],
        });
      }
      if (path === "/api/officer/bulk-tags") return mutation.promise;
      if (path.startsWith("/api/officer/characters/"))
        return response({ id: 3, note: "", lifecycleStatus: "active", tags: [] });
      return response([]);
    });
    vi.stubGlobal("confirm", () => true);
    component = mount(Officer, { target: host });
    await new Promise((resolve) => setTimeout(resolve));
    (host.querySelector("input[type=checkbox]") as HTMLInputElement).click();
    flushSync();
    [...host.querySelectorAll("button")].find((b) => b.textContent === "Add tags")!.click();
    [...host.querySelectorAll("button")].find((b) => b.textContent === "Refresh")!.click();
    await vi.waitFor(() => expect(queueCalls).toBe(2));
    refresh.resolve(
      new Response(
        JSON.stringify({ items: [{ characterId: 3, name: "Refresh", reason: "stale", tags: [] }] }),
        { status: 200 },
      ),
    );
    await new Promise((resolve) => setTimeout(resolve));
    mutation.resolve(new Response(JSON.stringify({ updated: 1 }), { status: 200 }));
    await new Promise((resolve) => setTimeout(resolve));
    flushSync();
    expect(host.querySelector("tbody")?.textContent).toContain("Saved");
    expect(host.querySelector("tbody")?.textContent).not.toContain("Refresh");
  });

  it("preserves a newer selection when review completion settles", async () => {
    const completion = deferred<Response>();
    let queueCalls = 0;
    vi.stubGlobal("fetch", (path: string, init?: RequestInit) => {
      if (path === "/api/officer/queue/complete") return completion.promise;
      if (path.startsWith("/api/officer/queue")) {
        queueCalls++;
        return response({
          items: [
            { characterId: 3, name: "Alpha", reason: "stale", tags: [] },
            { characterId: 4, name: "Beta", reason: "stale", tags: [] },
          ],
        });
      }
      if (path.startsWith("/api/officer/characters/"))
        return response({ id: 3, note: "", lifecycleStatus: "active", tags: [] });
      return response([]);
    });
    vi.stubGlobal("confirm", () => true);
    component = mount(Officer, { target: host });
    await new Promise((resolve) => setTimeout(resolve));
    const checkboxes = host.querySelectorAll("input[type=checkbox]");
    (checkboxes[0] as HTMLInputElement).click();
    flushSync();
    [...host.querySelectorAll("button")]
      .find((b) => b.textContent?.includes("Mark reviewed"))!
      .click();
    (checkboxes[0] as HTMLInputElement).click();
    (checkboxes[1] as HTMLInputElement).click();
    await vi.waitFor(() => expect(completion).toBeDefined());
    completion.resolve(new Response(JSON.stringify({ completed: 1 }), { status: 200 }));
    await new Promise((resolve) => setTimeout(resolve));
    flushSync();
    expect((host.querySelectorAll("input[type=checkbox]")[1] as HTMLInputElement).checked).toBe(
      true,
    );
    expect(host.textContent).toContain("Completed 1 reviews.");
  });
});
