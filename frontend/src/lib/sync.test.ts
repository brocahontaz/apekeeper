import { describe, it, expect } from "vitest";
import {
  canTriggerSync,
  canViewRuns,
  failureDetails,
  notifyStatusChip,
  type AppRole,
} from "$lib/sync";

describe("failureDetails", () => {
  it("parses a detail map into alphabetically sorted rows", () =>
    expect(failureDetails({ detail: { Zog: "rate limited", Amp: "404: not found" } })).toEqual([
      { name: "Amp", error: "404: not found" },
      { name: "Zog", error: "rate limited" },
    ]));

  it("returns nothing when detail is null", () =>
    expect(failureDetails({ detail: null })).toEqual([]));

  it("returns nothing when detail is missing", () =>
    expect(failureDetails({ detail: undefined })).toEqual([]));

  it("rejects non-object detail payloads", () => {
    expect(failureDetails({ detail: ["boom"] })).toEqual([]);
    expect(failureDetails({ detail: "boom" })).toEqual([]);
    expect(failureDetails({ detail: 7 })).toEqual([]);
  });

  it("keeps only string values", () =>
    expect(failureDetails({ detail: { Amp: "404", Zog: 3, Nog: null } })).toEqual([
      { name: "Amp", error: "404" },
    ]));

  it("handles an empty detail map", () => expect(failureDetails({ detail: {} })).toEqual([]));
});

describe("notifyStatusChip", () => {
  it("maps recorded delivery results onto labelled chips", () => {
    expect(notifyStatusChip("sent")).toEqual({ label: "notify sent", kind: "up" });
    expect(notifyStatusChip("skipped")).toEqual({ label: "notify skipped", kind: "stale" });
    expect(notifyStatusChip("failed")).toEqual({ label: "notify failed", kind: "error" });
  });

  it("renders nothing when delivery was never recorded or is unknown", () => {
    expect(notifyStatusChip(null)).toBeNull();
    expect(notifyStatusChip(undefined)).toBeNull();
    expect(notifyStatusChip("pending")).toBeNull();
  });
});

describe("sync role gating", () => {
  const cases: [AppRole, boolean, boolean][] = [
    ["member", false, false],
    ["officer", false, true],
    ["admin", true, true],
    ["superadmin", true, true],
  ];
  it("allows triggering only for admin and superadmin", () => {
    for (const [role, trigger] of cases) expect(canTriggerSync(role)).toBe(trigger);
    expect(canTriggerSync(undefined)).toBe(false);
  });
  it("allows viewing runs for officer and above", () => {
    for (const [role, , view] of cases) expect(canViewRuns(role)).toBe(view);
    expect(canViewRuns(undefined)).toBe(false);
  });
});
