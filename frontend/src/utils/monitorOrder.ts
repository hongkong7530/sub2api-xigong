type MonitorIdentity = { provider: string; name: string; id: number }
const compare = (a: string, b: string) => a < b ? -1 : a > b ? 1 : 0

/** Stable machine identifiers, never live status, timing or translated labels. */
export function sortMonitors<T extends MonitorIdentity>(items: readonly T[]): T[] {
  return [...items].sort((a, b) => compare(a.provider, b.provider) || compare(a.name, b.name) || a.id - b.id)
}
