import { afterEach, describe, it, expect, vi } from "vitest";
import { mount, tick, unmount } from "svelte";
import RosterTable from "./RosterTable.svelte";
import { filterRoster, rosterQuery } from "$lib/roster";
const rows: any[] = [
  { name: "Alpha", className: "Mage", stale: false },
  { name: "Bravo", className: "Rogue", stale: true },
];
describe("filterRoster", () =>
  it("filters search class and stale rows", () => {
    expect(filterRoster(rows, { search: "alp", class: "Mage" })).toHaveLength(1);
    expect(filterRoster(rows, { stale: true })).toEqual([rows[1]]);
  }));

describe("rosterQuery", () => {
  it("includes the active sort and filters when fetching a page", () => {
    expect(
      rosterQuery({
        search: "alp",
        class: "Mage",
        stale: true,
        page: 2,
        pageSize: 25,
        sort: "mythicRating",
        direction: "descending",
      }).toString(),
    ).toBe(
      "page=2&pageSize=25&sort=mythicRating&direction=descending&search=alp&class=Mage&stale=true",
    );
  });

  it("includes spec and numeric thresholds when set", () => {
    expect(
      rosterQuery({
        spec: "Arcane",
        minLevel: 70,
        minRating: 1000,
        page: 1,
        pageSize: 25,
        sort: "guildRank",
        direction: "ascending",
      }).toString(),
    ).toBe(
      "page=1&pageSize=25&sort=guildRank&direction=ascending&spec=Arcane&minLevel=70&minRating=1000",
    );
  });

  it("omits unset spec and numeric filters from the query string", () => {
    expect(
      rosterQuery({
        minLevel: 0,
        page: 1,
        pageSize: 25,
        sort: "guildRank",
        direction: "ascending",
      }).toString(),
    ).toBe("page=1&pageSize=25&sort=guildRank&direction=ascending");
  });
});

describe("RosterTable presentation", () => {
  let host: HTMLDivElement;
  let component: ReturnType<typeof mount>;
  const tableRows = [
    {
      ...rows[1],
      name: "Bravo",
      realm: "Azeroth",
      classId: 4,
      specName: "Combat",
      level: 70,
      itemLevel: 600,
      mythicRating: 0,
      bestKeyLevel: 0,
      syncedAt: "2026-01-01T00:00:00Z",
      role: "member",
    },
    {
      ...rows[0],
      name: "Alpha",
      realm: "Azeroth",
      classId: 8,
      specName: "Arcane",
      level: 70,
      itemLevel: 610,
      mythicRating: 2200,
      bestKeyLevel: 12,
      syncedAt: "2026-01-02T00:00:00Z",
      role: "officer",
    },
  ] as any;

  afterEach(() => {
    if (component) unmount(component);
    host?.remove();
    localStorage.clear();
    vi.restoreAllMocks();
  });

  function render(groupBy = "none") {
    host = document.createElement("div");
    document.body.append(host);
    component = mount(RosterTable, {
      target: host,
      props: { rows: tableRows, sort: "name", direction: "ascending", groupBy, onSort: vi.fn() },
    });
  }

  it("groups rows and retains mobile card links with the roster query", () => {
    history.replaceState({}, "", "/roster?class=Mage&page=2");
    render("class");
    expect(host.querySelectorAll(".roster-group")).toHaveLength(2);
    expect(host.querySelectorAll(".roster-card")).toHaveLength(2);
    expect(
      host.querySelector(
        '.roster-card[href="/characters/Bravo?roster=%3Fclass%3DMage%26page%3D2"]',
      ),
    ).not.toBeNull();
  });

  it("persists column visibility and exposes sortable headers accessibly", async () => {
    render();
    const checkboxes = host.querySelectorAll<HTMLInputElement>(
      '.column-picker input[type="checkbox"]',
    );
    checkboxes[1].click();
    await tick();
    expect(localStorage.getItem("apekeeper.roster.columns")).toContain("name");
    expect(host.querySelector('th[aria-sort="ascending"]')).not.toBeNull();
    expect(host.querySelectorAll("thead th")).toHaveLength(7);
    if (component) unmount(component);
    host.remove();
    render();
    expect(host.querySelectorAll("thead th")).toHaveLength(7);
  });
});
