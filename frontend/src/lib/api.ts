import { sessionExpired, currentUser } from "./stores";
import { navigate } from "./router";
export type User = {
  id: number;
  displayName: string;
  battletag: string;
  appRole: "superadmin" | "admin" | "officer" | "member";
};
export type Character = {
  id: number;
  name: string;
  realm: string;
  classId: number;
  className: string;
  specId: number;
  specName: string;
  level: number;
  itemLevel: number;
  guildRank?: number;
  mythicRating: number;
  bestKeyLevel: number;
  syncedAt?: string;
  stale: boolean;
  [key: string]: unknown;
};
export type SyncRun = {
  id: number;
  guildId: number;
  startedAt: string;
  finishedAt: string | null;
  trigger: string;
  status: string;
  total: number;
  updated: number;
  failed: number;
  errorSummary: string;
  detail: unknown;
};
export type Dashboard = {
  rosterSize: number;
  maxLevelMembers: number;
  activeMembers: number;
  classDistribution: { classId: number; className: string; count: number }[];
  specDistribution: { className: string; count: number }[];
  mythicPlus: {
    top: { name: string; realm: string; rating: number; bestKey: number }[];
    averageRating: number;
  };
  raidProgression: {
    raidName: string;
    difficulties: Record<string, { progress: number; totalBosses: number }>;
  }[];
  staleCharacters: {
    count: number;
    characters: { name: string; realm: string; syncedAt?: string; stalenessDays: number }[];
  };
  lastSync?: { status?: string; startedAt?: string };
  notableChanges: { name: string; ratingDelta?: number; itemLevelDelta?: number }[];
};
export class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message);
  }
}
export async function api<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, { credentials: "same-origin", ...init });
  if (response.status === 401) {
    sessionExpired.set(true);
    currentUser.set(null);
    if (typeof window !== "undefined" && location.pathname !== "/login") navigate("/login");
  }
  if (!response.ok) {
    let message = response.statusText;
    try {
      message = (await response.json()).error || message;
    } catch {}
    throw new ApiError(response.status, message);
  }
  return response.status === 204 ? (undefined as T) : response.json();
}
export const client = {
  me: () => api<User>("/api/auth/me"),
  dashboard: () => api<Dashboard>("/api/dashboard"),
  roster: (q = "") => api<Character[]>(`/api/roster${q}`),
  character: (id: string) => api<Character>(`/api/characters/${id}`),
  runs: () => api<SyncRun[]>("/api/sync/runs"),
  sync: () => api<{ runId: number }>("/api/sync/run", { method: "POST" }),
  logout: () => api<void>("/api/auth/logout", { method: "POST" }),
};
