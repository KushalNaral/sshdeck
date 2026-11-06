import type { Version, VersionKind } from '../types'

export const KIND_LABEL: Record<VersionKind, string> = {
  initial: 'First seen',
  save: 'Saved',
  restore: 'Restored',
  outside: 'Changed outside sshdeck',
}

/** "3 min ago", "yesterday", "12 Oct". Exact time goes in the tooltip. */
export function ago(ms: number, now = Date.now()): string {
  const s = Math.max(0, Math.round((now - ms) / 1000))
  if (s < 45) return 'just now'
  if (s < 3600) return `${Math.round(s / 60)} min ago`
  if (s < 86400) return `${Math.round(s / 3600)} h ago`
  if (s < 172800) return 'yesterday'
  if (s < 30 * 86400) return `${Math.round(s / 86400)} days ago`
  return new Date(ms).toLocaleDateString('en-GB', { day: 'numeric', month: 'short', year: 'numeric' })
}

export const exact = (ms: number) => new Date(ms).toLocaleString('en-GB')

/** Who made the change: "sam@laptop as deploy", or what we know when nobody here did. */
export function by(v: Pick<Version, 'author' | 'login' | 'kind'>): string {
  if (!v.author) return v.kind === 'initial' ? 'before sshdeck' : 'someone, outside sshdeck'
  return v.login ? `${v.author} as ${v.login}` : v.author
}

export function sizeDelta(v: Version, older?: Version): string {
  if (!older) return ''
  const d = v.size - older.size
  return d === 0 ? 'same size' : `${d > 0 ? '+' : '−'}${Math.abs(d)} B`
}

export function matches(v: Version, q: string): boolean {
  const t = q.trim().toLowerCase()
  return !t || `${v.path} ${by(v)} ${KIND_LABEL[v.kind]} ${v.note ?? ''}`.toLowerCase().includes(t)
}
