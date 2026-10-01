import { describe, expect, it } from "vitest";
import { flushSync, mount, unmount } from "svelte";
import HistoryCharts from "./HistoryCharts.svelte";
import type { Snapshot } from "$lib/api";

const snapshots: Snapshot[] = [
  {
    id: 1,
    characterId: 1,
    capturedAt: "2026-01-01T00:00:00Z",
    itemLevel: 610,
    mythicRating: 2000,
    bestKeyLevel: 10,
  },
  {
    id: 2,
    characterId: 1,
    capturedAt: "2026-01-02T00:00:00Z",
    itemLevel: 620,
    mythicRating: 2200,
    bestKeyLevel: 12,
  },
];

describe("HistoryCharts", () => {
  it("renders chart labels and non-visual summaries", () => {
    const host = document.createElement("div");
    document.body.append(host);
    const component = mount(HistoryCharts, { target: host, props: { snapshots } });
    flushSync();
    expect(host.querySelectorAll("svg")).toHaveLength(2);
    expect(host.textContent).toContain("Item level changed from 610 to 620");
    expect(host.querySelector("svg")?.getAttribute("aria-label")).toContain("Item level changed");
    unmount(component);
    host.remove();
  });

  it("keeps malformed or empty history from breaking the page", () => {
    const host = document.createElement("div");
    document.body.append(host);
    const component = mount(HistoryCharts, {
      target: host,
      props: {
        snapshots: [null, { ...snapshots[0], itemLevel: Number.NaN }] as unknown as Snapshot[],
      },
    });
    flushSync();
    expect(host.textContent).toContain("History data is incomplete");
    unmount(component);
    host.remove();
  });

  it("explains when the selected range has no snapshots", () => {
    const host = document.createElement("div");
    document.body.append(host);
    const component = mount(HistoryCharts, { target: host, props: { snapshots: [] } });
    flushSync();
    expect(host.textContent).toContain("No snapshots in this date range.");
    expect(host.querySelectorAll("svg")).toHaveLength(0);
    unmount(component);
    host.remove();
  });
});
