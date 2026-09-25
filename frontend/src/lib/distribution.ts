import type { Dashboard } from "./api";
export type ClassCount = Dashboard["classDistribution"][number];
export function sortClasses(classes: ClassCount[]): ClassCount[] {
  return [...classes]
    .sort((a, b) => a.className.localeCompare(b.className))
    .map((c) => ({ ...c, specs: [...c.specs].sort((a, b) => a.name.localeCompare(b.name)) }));
}
