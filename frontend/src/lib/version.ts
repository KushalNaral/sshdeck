const parts = (v: string) => v.split('.').map((n) => parseInt(n, 10) || 0)

/** Negative when a is older than b. Compares major.minor.patch. */
export function compareVersions(a: string, b: string): number {
  const [x, y] = [parts(a), parts(b)]
  for (let i = 0; i < 3; i++) {
    const d = (x[i] ?? 0) - (y[i] ?? 0)
    if (d !== 0) return d
  }
  return 0
}

export type Gap = 'current' | 'patch' | 'branch'

/** current: at/above target; patch: same major.minor, behind; branch: older major.minor (needs an upgrade). */
export function versionGap(current: string, target: string): Gap {
  if (compareVersions(current, target) >= 0) return 'current'
  const [c, t] = [parts(current), parts(target)]
  return c[0] === t[0] && c[1] === t[1] ? 'patch' : 'branch'
}
