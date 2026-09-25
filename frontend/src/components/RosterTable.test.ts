import { describe, it, expect } from "vitest";
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

describe("rosterQuery", () =>
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
  }));
