import { describe, it, expect } from "vitest";
import { filterRoster } from "$lib/roster";
const rows: any[] = [
  { name: "Alpha", className: "Mage", stale: false },
  { name: "Bravo", className: "Rogue", stale: true },
];
describe("filterRoster", () =>
  it("filters search class and stale rows", () => {
    expect(filterRoster(rows, { search: "alp", class: "Mage" })).toHaveLength(1);
    expect(filterRoster(rows, { stale: true })).toEqual([rows[1]]);
  }));
