/** Slash-path helpers for the server side. */
export const dirname = (p: string) => (p.lastIndexOf('/') <= 0 ? '/' : p.slice(0, p.lastIndexOf('/')))
export const basename = (p: string) => p.slice(p.lastIndexOf('/') + 1)
export const join = (d: string, n: string) => (d === '/' ? '/' + n : d + '/' + n)

export function humanSize(n: number): string {
  if (n < 1024) return `${n} B`
  const u = ['KB', 'MB', 'GB', 'TB']
  let v = n / 1024
  let i = 0
  while (v >= 1024 && i < u.length - 1) (v /= 1024, i++)
  return `${v < 10 ? v.toFixed(1) : Math.round(v)} ${u[i]}`
}

/** "3 Nov 2025" for old files, "3 Nov 14:05" within the last six months, like ls. */
export function shortDate(unix: number, now = Date.now()): string {
  const d = new Date(unix * 1000)
  const mon = d.toLocaleString('en-GB', { month: 'short' })
  if (now - d.getTime() < 182 * 86400_000 && d.getTime() <= now) return `${d.getDate()} ${mon} ${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`
  return `${d.getDate()} ${mon} ${d.getFullYear()}`
}

/** Breadcrumb pieces of an absolute path: "/a/b" gives [/, /a, /a/b]. */
export function crumbs(p: string): { name: string; path: string }[] {
  const parts = p.split('/').filter(Boolean)
  return [{ name: '/', path: '/' }, ...parts.map((n, i) => ({ name: n, path: '/' + parts.slice(0, i + 1).join('/') }))]
}

/** A name for a new file or folder: the plain name must be one path component. */
export function validName(n: string): string | null {
  if (!n.trim()) return 'Type a name.'
  if (n.includes('/')) return 'A name cannot contain a slash.'
  if (n === '.' || n === '..') return 'That name is reserved.'
  return null
}

/** The file history names container files "docker:NAME:/path"; this splits that back into its parts. */
export function splitHist(p: string): { container: string; path: string } {
  const m = /^docker:([A-Za-z0-9][A-Za-z0-9_.-]*):(\/.*)$/.exec(p)
  return m ? { container: m[1], path: m[2] } : { container: '', path: p }
}

/** A target names where files are: a server id, or "serverId@container". */
export const targetOf = (serverId: string, container: string) => (container ? `${serverId}@${container}` : serverId)
