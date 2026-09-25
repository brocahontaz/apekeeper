import { describe, it, expect } from "vitest";
import { classColor } from "$lib/theme";
import { sortClasses, type ClassCount } from "$lib/distribution";
describe("class distribution palette", () =>
  it("uses canonical mage color", () => expect(classColor(8)).toBe("#3FC7EB")));

describe("sortClasses", () => {
  it("sorts classes and specs by name without mutating the input", () => {
    const input: ClassCount[] = [
      {
        classId: 1,
        className: "Warrior",
        count: 2,
        specs: [
          { name: "Fury", count: 1 },
          { name: "Arms", count: 1 },
        ],
      },
      { classId: 8, className: "Mage", count: 1, specs: [{ name: "Arcane", count: 1 }] },
    ];
    const sorted = sortClasses(input);
    expect(sorted.map((c) => c.className)).toEqual(["Mage", "Warrior"]);
    expect(sorted[1].specs.map((s) => s.name)).toEqual(["Arms", "Fury"]);
    expect(input.map((c) => c.className)).toEqual(["Warrior", "Mage"]);
    expect(input[0].specs.map((s) => s.name)).toEqual(["Fury", "Arms"]);
  });

  it("passes classes without specs through", () => {
    const row: ClassCount = { classId: 4, className: "Rogue", count: 1, specs: [] };
    expect(sortClasses([row])).toEqual([row]);
  });
});
