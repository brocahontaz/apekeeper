import { writable } from "svelte/store";
import type { User } from "./api";
export const currentUser = writable<User | null>(null);
export const sessionExpired = writable(false);
// Populated by App after guild selection. A null value keeps standalone route
// tests and the initial bootstrap compatible with the user's app role.
export const selectedGuildRole = writable<string | null>(null);

export function effectiveRole(
  user: User | null,
  guildRole: string | null,
): User["appRole"] | undefined {
  if (!user || guildRole === null) return user?.appRole;
  if (user.appRole === "superadmin") return "superadmin";
  switch (guildRole) {
    case "owner":
    case "admin":
      return "admin";
    case "officer":
      return "officer";
    default:
      return "member";
  }
}
