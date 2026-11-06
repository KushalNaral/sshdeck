import type { Sample } from '../types'

export const RANGES = [
  { id: '15m', label: '15 min', sec: 15 * 60 },
  { id: '1h', label: '1 hour', sec: 3600 },
  { id: '6h', label: '6 hours', sec: 6 * 3600 },
  { id: '24h', label: '24 hours', sec: 24 * 3600 },
  { id: '7d', label: '7 days', sec: 7 * 86400 },
] as const
export type RangeId = (typeof RANGES)[number]['id']

/** "1.2 MB", "340 KB", "12 B". Binary units, one decimal under 10. */
export function fmtBytes(n: number): string {
  if (!Number.isFinite(n) || n < 0) return '0 B'
  const u = ['B', 'KB', 'MB', 'GB', 'TB']
  let i = 0
  while (n >= 1024 && i < u.length - 1) {
    n /= 1024
    i++
  }
  return `${i === 0 || n >= 10 ? Math.round(n) : n.toFixed(1)} ${u[i]}`
}
export const fmtRate = (bps: number) => `${fmtBytes(bps)}/s`

export interface Pt {
  t: number
  /** null is a hole: the line breaks here instead of joining its neighbours. */
  v: number | null
}

/** One value per sample. Rates are holes for a reading that had none (the first one after a connect or a reboot). */
export function series(samples: Sample[], pick: (s: Sample) => number, rate = false): Pt[] {
  return samples.map((s) => ({ t: s.t, v: rate && !s.hasRates ? null : pick(s) }))
}

/**
 * Puts a hole between two points that are much further apart than the usual spacing, so a stretch with no
 * readings (the app was closed, the server was disconnected) shows as a gap and not as a straight line.
 */
export function withGaps(pts: Pt[]): Pt[] {
  if (pts.length < 3) return pts
  const d = pts.slice(1).map((p, i) => p.t - pts[i].t).sort((a, b) => a - b)
  const limit = Math.max(10, d[Math.floor(d.length / 2)] * 3)
  const out: Pt[] = [pts[0]]
  for (let i = 1; i < pts.length; i++) {
    if (pts[i].t - pts[i - 1].t > limit) out.push({ t: pts[i - 1].t + 1, v: null })
    out.push(pts[i])
  }
  return out
}

export function worstDisk(s: Sample | undefined): { mount: string; usedPct: number } | null {
  if (!s?.disks?.length) return null
  return s.disks.reduce((a, b) => (b.usedPct > a.usedPct ? b : a))
}

/** live: a reading arrived recently; stale: the last one is old (sampling stopped or the link is slow); none: never. */
export function freshness(s: Sample | undefined, nowSec: number, everySec = 5): 'live' | 'stale' | 'none' {
  if (!s) return 'none'
  return nowSec - s.t <= Math.max(15, everySec * 3) ? 'live' : 'stale'
}

/** Appends a reading to a ring of at most max, newest last. */
export function pushRing(ring: Sample[], s: Sample, max: number): Sample[] {
  if (ring.length && ring[ring.length - 1].t >= s.t) return ring // the same reading again
  const next = [...ring, s]
  return next.length > max ? next.slice(next.length - max) : next
}

/** The y-axis top for a series: the next "round" number above the data, never under floor. */
export function niceMax(values: (number | null)[], floor: number): number {
  const m = Math.max(floor, ...values.map((v) => v ?? 0))
  const pow = 10 ** Math.floor(Math.log10(m))
  for (const f of [1, 1.5, 2, 3, 5, 7.5, 10]) if (f * pow >= m) return f * pow
  return 10 * pow
}

/** "5m ago", "2h ago" for an alert's start. */
export function ago(sec: number, nowSec: number): string {
  const m = Math.max(0, Math.round((nowSec - sec) / 60))
  if (m < 1) return 'just now'
  if (m < 60) return `${m}m ago`
  const h = Math.round(m / 60)
  return h < 48 ? `${h}h ago` : `${Math.round(h / 24)}d ago`
}
