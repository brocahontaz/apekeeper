import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { flushSync, mount, unmount } from "svelte";
import Sync from "$routes/Sync.svelte";
import { currentUser, selectedGuildRole } from "$lib/stores";
import type { SyncProgress, SyncRun } from "$lib/api";

const runningRun: SyncRun = {
  id: 1,
  guildId: 1,
  startedAt: "2026-01-01T00:00:00Z",
  finishedAt: null,
  trigger: "manual",
  status: "running",
  total: 0,
  updated: 0,
  failed: 0,
  errorSummary: "",
  detail: null,
};
const finishedRun: SyncRun = {
  ...runningRun,
  status: "success",
  finishedAt: "2026-01-01T00:05:00Z",
};

function progressSnapshot(call: number): SyncProgress {
  return call < 2
    ? {
        active: true,
        runId: 1,
        status: "running",
        phase: "characters",
        total: 5,
        updated: 3,
        failed: 1,
        startedAt: "2026-01-01T00:00:00Z",
        dryRun: false,
      }
    : {
        active: false,
        runId: 1,
        status: "success",
        phase: "finalizing",
        total: 5,
        updated: 5,
        failed: 0,
        startedAt: "2026-01-01T00:00:00Z",
        dryRun: false,
      };
}

function jsonResponse(body: unknown) {
  return new Response(JSON.stringify(body), { status: 200 });
}

describe("Sync live progress", () => {
  let host: HTMLDivElement;
  let component: ReturnType<typeof mount>;

  beforeEach(() => {
    host = document.createElement("div");
    document.body.appendChild(host);
    currentUser.set({ id: 1, displayName: "officer", battletag: "officer#1", appRole: "officer" });
    vi.useFakeTimers();
  });

  afterEach(() => {
    unmount(component);
    host.remove();
    currentUser.set(null);
    selectedGuildRole.set(null);
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  it("stops polling and reloads the runs list once the snapshot goes terminal", async () => {
    let runsCalls = 0;
    let progressCalls = 0;
    vi.stubGlobal("fetch", (path: string) => {
      if (path === "/api/sync/runs") {
        runsCalls++;
        return Promise.resolve(jsonResponse([runsCalls === 1 ? runningRun : finishedRun]));
      }
      if (path === "/api/sync/progress") {
        progressCalls++;
        return Promise.resolve(jsonResponse(progressSnapshot(progressCalls)));
      }
      return Promise.resolve(jsonResponse({}));
    });

    component = mount(Sync, { target: host });
    await vi.advanceTimersByTimeAsync(0);
    flushSync();
    // The newest run is running, so the panel polls and shows the live phase.
    expect(progressCalls).toBe(1);
    expect(host.textContent).toContain("Expedition in progress");
    expect(host.textContent).toContain("Updating the crew");

    await vi.advanceTimersByTimeAsync(1500);
    flushSync();
    // The terminal snapshot ends the polling and reloads the runs so the
    // finished row replaces the running one without a page refresh.
    expect(progressCalls).toBe(2);
    expect(runsCalls).toBe(2);
    expect(host.textContent).not.toContain("Expedition in progress");
    expect(host.textContent).toContain("success");
    expect(host.querySelector('[role="status"]')?.textContent).toContain(
      "Sync completed successfully.",
    );

    await vi.advanceTimersByTimeAsync(4500);
    expect(progressCalls).toBe(2);
    expect(runsCalls).toBe(2);
  });

  it("renders no live panel for members", async () => {
    currentUser.set({ id: 1, displayName: "member", battletag: "member#1", appRole: "member" });
    let progressCalls = 0;
    vi.stubGlobal("fetch", (path: string) => {
      if (path === "/api/sync/runs") return Promise.resolve(jsonResponse([runningRun]));
      if (path === "/api/sync/progress") {
        progressCalls++;
        return Promise.resolve(jsonResponse(progressSnapshot(1)));
      }
      return Promise.resolve(jsonResponse({}));
    });

    component = mount(Sync, { target: host });
    await vi.advanceTimersByTimeAsync(0);
    flushSync();
    await vi.advanceTimersByTimeAsync(1500);
    expect(progressCalls).toBe(0);
    expect(host.textContent).not.toContain("Expedition in progress");
  });

  it("uses the selected guild role instead of the global app role", async () => {
    currentUser.set({ id: 1, displayName: "member", battletag: "member#1", appRole: "admin" });
    selectedGuildRole.set("member");
    vi.stubGlobal("fetch", (path: string) =>
      Promise.resolve(jsonResponse(path === "/api/sync/runs" ? [] : {})),
    );
    component = mount(Sync, { target: host });
    await vi.advanceTimersByTimeAsync(0);
    flushSync();
    expect(host.textContent).toContain("Trigger restricted to Guild Master");
    expect(host.textContent).toContain("Run history is sealed");
    expect(host.querySelector("button.gold")).toBeNull();
  });
});
