import { describe, it, expect } from "vitest";
import { classColor } from "$lib/theme";
describe("class distribution palette", () =>
  it("uses canonical mage color", () => expect(classColor(8)).toBe("#3FC7EB")));
