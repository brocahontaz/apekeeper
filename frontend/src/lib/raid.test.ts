import { describe, it, expect } from "vitest";
import { raidRows, progressPct, type RaidProgression } from "$lib/raid";
const progression: RaidProgression = [
  {
    raidName: "Nerub-ar Palace",
    difficulties: {
      Heroic: { progress: 8, totalBosses: 8 },
      Mythic: { progress: 1, totalBosses: 8 },
      Timewalking: { progress: 2, totalBosses: 8 },
      Normal: { progress: 8, totalBosses: 8 },
      Sealed: { progress: 0, totalBosses: 0 },
    },
  },
  {
    raidName: "Liberation of Undermine",
    difficulties: {
      Mythic: { progress: 3, totalBosses: 8 },
      Normal: { progress: 5, totalBosses: 8 },
      "Raid Finder": { progress: 5, totalBosses: 8 },
    },
  },
];

describe("raidRows", () => {
  it("flattens every difficulty of every raid into one row each", () =>
    expect(raidRows(progression)).toHaveLength(7));

  it("sorts by raid name alphabetically", () => {
    const rows = raidRows(progression);
    expect(rows[0].raidName).toBe("Liberation of Undermine");
    expect(rows[rows.length - 1].raidName).toBe("Nerub-ar Palace");
  });

  it("orders difficulties canonically with unknowns last", () => {
    const rows = raidRows(progression);
    expect(
      rows.filter((r) => r.raidName === "Liberation of Undermine").map((r) => r.difficulty),
    ).toEqual(["Raid Finder", "Normal", "Mythic"]);
    expect(rows.filter((r) => r.raidName === "Nerub-ar Palace").map((r) => r.difficulty)).toEqual([
      "Normal",
      "Heroic",
      "Mythic",
      "Timewalking",
    ]);
  });

  it("skips difficulties without bosses", () =>
    expect(raidRows(progression).some((r) => r.difficulty === "Sealed")).toBe(false));

  it("tolerates a missing progression payload", () => {
    expect(raidRows([])).toEqual([]);
    expect(raidRows([{ raidName: "Empty", difficulties: {} }])).toEqual([]);
  });
});

describe("progressPct", () => {
  const row = { progress: 6, totalBosses: 8 };
  it("returns the progress ratio as a percentage", () => expect(progressPct(row)).toBe(75));
  it("clamps overshooting progress to 100", () =>
    expect(progressPct({ progress: 9, totalBosses: 8 })).toBe(100));
  it("clamps negative progress to 0", () =>
    expect(progressPct({ progress: -2, totalBosses: 8 })).toBe(0));
  it("avoids dividing by zero", () => expect(progressPct({ progress: 1, totalBosses: 0 })).toBe(0));
});
