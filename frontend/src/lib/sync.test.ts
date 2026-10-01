import { describe, it, expect } from "vitest";
import {
  canTriggerSync,
  canViewProgress,
  canViewRuns,
  failureDetails,
  notifyStatusChip,
  phaseLabel,
  progressActive,
  progressTerminal,
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
  it("allows viewing live progress for the same set as run history", () => {
    for (const [role, , view] of cases) expect(canViewProgress(role)).toBe(view);
    expect(canViewProgress(undefined)).toBe(false);
  });
});

describe("phaseLabel", () => {
  it("maps live phases onto expedition-log wording", () => {
    expect(phaseLabel("queued")).toBe("Queued");
    expect(phaseLabel("roster")).toBe("Reading the roster");
    expect(phaseLabel("tier-anchor")).toBe("Anchoring the tier");
    expect(phaseLabel("characters")).toBe("Updating the crew");
    expect(phaseLabel("finalizing")).toBe("Writing the ledger");
  });

  it("falls back to the queued wording for unknown or missing phases", () => {
    expect(phaseLabel("")).toBe("Queued");
    expect(phaseLabel("mystery")).toBe("Queued");
    expect(phaseLabel(undefined)).toBe("Queued");
  });
});

describe("progressActive", () => {
  it("stays true while a run is in flight", () => {
    expect(progressActive({ active: true, status: "running" })).toBe(true);
  });

  it("ends on an idle snapshot or a terminal status", () => {
    expect(progressActive({ active: false, status: "success" })).toBe(false);
    expect(progressActive({ active: true, status: "success" })).toBe(false);
    expect(progressActive({ active: true, status: "partial" })).toBe(false);
    expect(progressActive({ active: true, status: "failed" })).toBe(false);
  });

  it("treats only settled run statuses as terminal", () => {
    expect(progressTerminal("running")).toBe(false);
    expect(progressTerminal("")).toBe(false);
    expect(progressTerminal("success")).toBe(true);
    expect(progressTerminal("partial")).toBe(true);
    expect(progressTerminal("failed")).toBe(true);
  });
});
