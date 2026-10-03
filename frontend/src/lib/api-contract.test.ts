import { expect, test } from "vitest";
import contract from "../../../contracts/api-contract.json";
import type {
  Character,
  Dashboard,
  Guild,
  History,
  RosterPage,
  Snapshot,
  SyncProgress,
  SyncRun,
  User,
} from "./api";

type Model = { required: Record<string, string>; optional?: Record<string, string> };
const models = (contract as { models: Record<string, Model> }).models;

const representatives = {
  User: { id: 1, displayName: "Ape", battletag: "Ape#1", appRole: "admin" } satisfies User,
  Guild: {
    id: 2,
    slug: "apes",
    name: "Apes",
    realm: "Area 52",
    region: "us",
    role: "member",
    selected: true,
  } satisfies Guild,
  Character: {
    id: 7,
    name: "Alpha",
    realm: "Area 52",
    classId: 1,
    className: "Warrior",
    specId: 71,
    specName: "Arms",
    role: "member",
    level: 80,
    itemLevel: 620.5,
    guildRank: 2,
    raceName: "Human",
    gender: "Male",
    mythicRating: 2500,
    bestKeyLevel: 12,
    syncedAt: "2026-01-02T03:04:05Z",
    stale: false,
  } satisfies Character,
  Snapshot: {
    id: 3,
    characterId: 7,
    capturedAt: "2026-01-02T03:04:05Z",
    itemLevel: 620.5,
    mythicRating: 2500,
    bestKeyLevel: 12,
  } satisfies Snapshot,
  History: {
    from: "2026-01-01T00:00:00Z",
    to: "2026-01-02T00:00:00Z",
    retentionDays: 90,
    snapshots: [] as Snapshot[],
    comparison: null,
  } satisfies History,
  RosterPage: {
    items: [] as Character[],
    total: 0,
    page: 1,
    pageSize: 25,
    classes: ["Warrior"],
    specs: ["Arms"],
  } satisfies RosterPage,
  SyncRun: {
    id: 4,
    guildId: 2,
    startedAt: "2026-01-02T00:00:00Z",
    finishedAt: null,
    trigger: "manual",
    status: "running",
    total: 1,
    updated: 0,
    failed: 0,
    errorSummary: "",
    detail: {},
  } satisfies SyncRun,
  SyncProgress: {
    active: true,
    runId: 4,
    status: "running",
    phase: "characters",
    total: 1,
    updated: 0,
    failed: 0,
    startedAt: "2026-01-02T00:00:00Z",
    dryRun: false,
  } satisfies SyncProgress,
  Dashboard: {
    rosterSize: 1,
    maxLevelMembers: 1,
    activeMembers: 1,
    classDistribution: [],
    mythicPlus: { top: [], averageRating: 0, ratedCount: 0 },
    raidProgression: [],
    staleCharacters: { count: 0, characters: [] },
    notableChanges: [],
    trends: [],
  } satisfies Dashboard,
} as const;

function assertType(value: unknown, type: string, model: string, field: string) {
  if (type === "nullable" && value === null) return;
  if (type === "integer") expect(Number.isInteger(value), `${model}.${field}`).toBe(true);
  else if (type === "number") expect(typeof value, `${model}.${field}`).toBe("number");
  else if (type === "array") expect(Array.isArray(value), `${model}.${field}`).toBe(true);
  else if (type === "object")
    expect(
      typeof value === "object" && value !== null && !Array.isArray(value),
      `${model}.${field}`,
    ).toBe(true);
  else if (type === "boolean") expect(typeof value, `${model}.${field}`).toBe("boolean");
  else expect(typeof value, `${model}.${field}`).toBe("string");
}

test("representative values satisfy every shared response model", () => {
  expect(Object.keys(representatives).sort()).toEqual(Object.keys(models).sort());
  for (const [name, model] of Object.entries(models)) {
    const value = representatives[name as keyof typeof representatives] as Record<string, unknown>;
    for (const [field, type] of Object.entries(model.required)) {
      expect(value, `${name} representative`).toHaveProperty(field);
      assertType(value[field], type, name, field);
    }
  }
});
