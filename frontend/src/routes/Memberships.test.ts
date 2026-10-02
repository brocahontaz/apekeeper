import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { flushSync, mount, unmount } from "svelte";
import Memberships from "$routes/Memberships.svelte";
import { currentUser } from "$lib/stores";

const response = (body: unknown) =>
  Promise.resolve(new Response(JSON.stringify(body), { status: 200 }));
const deferred = <T>() => {
  let resolve!: (value: T) => void;
  return { promise: new Promise<T>((r) => (resolve = r)), resolve };
};

describe("Memberships", () => {
  let host: HTMLDivElement;
  let component: ReturnType<typeof mount> | undefined;

  beforeEach(() => {
    host = document.createElement("div");
    document.body.appendChild(host);
    currentUser.set({ id: 1, displayName: "Admin", battletag: "admin#1", appRole: "admin" });
  });

  afterEach(() => {
    if (component) unmount(component);
    host.remove();
    currentUser.set(null);
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("ignores an older member search response", async () => {
    let resolveOld!: (value: Response) => void;
    let resolveNew!: (value: Response) => void;
    vi.stubGlobal("fetch", (path: string) => {
      if (path.startsWith("/api/guild/members/search?q=old"))
        return new Promise<Response>((resolve) => (resolveOld = resolve));
      if (path.startsWith("/api/guild/members/search?q=new"))
        return new Promise<Response>((resolve) => (resolveNew = resolve));
      if (path === "/api/guilds")
        return response([
          {
            id: 1,
            slug: "guild",
            name: "Guild",
            realm: "Realm",
            region: "us",
            role: "admin",
            selected: true,
          },
        ]);
      return response([]);
    });
    component = mount(Memberships, { target: host });
    await new Promise((resolve) => setTimeout(resolve));
    const input = host.querySelector("input[aria-label='Find user']") as HTMLInputElement;
    const find = () =>
      [...host.querySelectorAll("button")].find((button) => button.textContent === "Find")!;
    input.value = "old";
    input.dispatchEvent(new Event("input", { bubbles: true }));
    find().click();
    input.value = "new";
    input.dispatchEvent(new Event("input", { bubbles: true }));
    find().click();
    resolveNew(
      new Response(
        JSON.stringify([{ id: 2, displayName: "New", battletag: "new#1", appRole: "member" }]),
      ),
    );
    await new Promise((resolve) => setTimeout(resolve));
    flushSync();
    expect(host.textContent).toContain("new#1");
    resolveOld(
      new Response(
        JSON.stringify([{ id: 3, displayName: "Old", battletag: "old#1", appRole: "member" }]),
      ),
    );
    await new Promise((resolve) => setTimeout(resolve));
    flushSync();
    expect(host.textContent).toContain("new#1");
    expect(host.textContent).not.toContain("old#1");
  });

  it("clears a search that completes after a successful grant", async () => {
    const lateSearch = deferred<Response>();
    let invite!: (value: Response) => void;
    vi.stubGlobal("fetch", (path: string, init?: RequestInit) => {
      if (path === "/api/guilds")
        return response([
          { id: 1, slug: "guild", name: "Guild", realm: "Realm", region: "us", role: "admin" },
        ]);
      if (path === "/api/guild/members" && init?.method === "POST")
        return new Promise<Response>((r) => (invite = r));
      if (path === "/api/guild/members") return response([]);
      if (path.includes("q=first"))
        return response([{ id: 2, displayName: "First", battletag: "first#1", appRole: "member" }]);
      if (path.includes("q=late")) return lateSearch.promise;
      return response([]);
    });
    component = mount(Memberships, { target: host });
    await new Promise((resolve) => setTimeout(resolve));
    const input = host.querySelector("input[aria-label='Find user']") as HTMLInputElement;
    const find = () => [...host.querySelectorAll("button")].find((b) => b.textContent === "Find")!;
    input.value = "first";
    input.dispatchEvent(new Event("input", { bubbles: true }));
    find().click();
    await new Promise((resolve) => setTimeout(resolve));
    flushSync();
    input.value = "late";
    input.dispatchEvent(new Event("input", { bubbles: true }));
    find().click();
    (host.querySelector("p button") as HTMLButtonElement).click();
    await new Promise((resolve) => setTimeout(resolve));
    invite(new Response("{}", { status: 200 }));
    await new Promise((resolve) => setTimeout(resolve));
    lateSearch.resolve(
      new Response(
        JSON.stringify([{ id: 3, displayName: "Late", battletag: "late#1", appRole: "member" }]),
        {
          status: 200,
        },
      ),
    );
    await new Promise((resolve) => setTimeout(resolve));
    flushSync();
    expect(host.textContent).not.toContain("late#1");
  });

  it("does not update after teardown while a nested member load is pending", async () => {
    const members = deferred<Response>();
    vi.stubGlobal("fetch", (path: string) =>
      path === "/api/guilds"
        ? response([
            { id: 1, slug: "guild", name: "Guild", realm: "Realm", region: "us", role: "admin" },
          ])
        : members.promise,
    );
    component = mount(Memberships, { target: host });
    await new Promise((resolve) => setTimeout(resolve));
    unmount(component!);
    component = undefined;
    members.resolve(
      new Response(
        JSON.stringify([{ id: 2, displayName: "Late", battletag: "late#1", role: "member" }]),
        {
          status: 200,
        },
      ),
    );
    await new Promise((resolve) => setTimeout(resolve));
    expect(host.textContent).not.toContain("Late");
  });

  it("ignores stale member mutation success and failure", async () => {
    const first = deferred<Response>();
    const second = deferred<Response>();
    let updates = 0;
    vi.stubGlobal("fetch", (path: string, init?: RequestInit) => {
      if (path === "/api/guilds")
        return response([
          { id: 1, slug: "guild", name: "Guild", realm: "Realm", region: "us", role: "admin" },
        ]);
      if (path === "/api/guild/members/2" && init?.method === "PUT")
        return (++updates === 1 ? first : second).promise;
      if (path === "/api/guild/members")
        return response([{ id: 2, displayName: "Member", battletag: "member#1", role: "admin" }]);
      return response([]);
    });
    component = mount(Memberships, { target: host });
    await new Promise((resolve) => setTimeout(resolve));
    const role = host.querySelector("select[aria-label='Role for Member']") as HTMLSelectElement;
    role.value = "officer";
    role.dispatchEvent(new Event("change", { bubbles: true }));
    role.value = "admin";
    role.dispatchEvent(new Event("change", { bubbles: true }));
    second.resolve(new Response(null, { status: 204 }));
    await new Promise((resolve) => setTimeout(resolve));
    first.resolve(new Response(null, { status: 204 }));
    await new Promise((resolve) => setTimeout(resolve));
    flushSync();
    expect(host.textContent).toContain("admin");
    expect(host.textContent).not.toContain("Membership update failed");
  });

  it("preserves a search started after a grant mutation", async () => {
    const grant = deferred<Response>();
    const currentSearch = deferred<Response>();
    vi.stubGlobal("fetch", (path: string, init?: RequestInit) => {
      if (path === "/api/guilds")
        return response([
          { id: 1, slug: "guild", name: "Guild", realm: "Realm", region: "us", role: "admin" },
        ]);
      if (path === "/api/guild/members" && init?.method === "POST") return grant.promise;
      if (path === "/api/guild/members") return response([]);
      if (path.includes("q=first"))
        return response([{ id: 2, displayName: "First", battletag: "first#1", appRole: "member" }]);
      if (path.includes("q=current")) return currentSearch.promise;
      return response([]);
    });
    component = mount(Memberships, { target: host });
    await new Promise((resolve) => setTimeout(resolve));
    const input = host.querySelector("input[aria-label='Find user']") as HTMLInputElement;
    const find = () => [...host.querySelectorAll("button")].find((b) => b.textContent === "Find")!;
    input.value = "first";
    input.dispatchEvent(new Event("input", { bubbles: true }));
    find().click();
    await new Promise((resolve) => setTimeout(resolve));
    flushSync();
    (host.querySelector("p button") as HTMLButtonElement).click();
    input.value = "current";
    input.dispatchEvent(new Event("input", { bubbles: true }));
    find().click();
    grant.resolve(new Response("{}", { status: 200 }));
    await new Promise((resolve) => setTimeout(resolve));
    currentSearch.resolve(
      new Response(
        JSON.stringify([
          { id: 4, displayName: "Current", battletag: "current#1", appRole: "member" },
        ]),
        { status: 200 },
      ),
    );
    await new Promise((resolve) => setTimeout(resolve));
    flushSync();
    expect(input.value).toBe("current");
    expect(host.textContent).toContain("current#1");
  });
});
