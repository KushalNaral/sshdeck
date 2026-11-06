/** The newest `max` lines after adding a batch. Returns a new array. */
export function pushLines(lines: string[], more: string[], max = 10000): string[] {
  const all = lines.length + more.length > max ? [...lines, ...more].slice(-max) : [...lines, ...more]
  return all
}

/** "text" matches anywhere without regard to case; "/pattern/" is a regular expression. An empty or broken filter shows everything. */
export function makeFilter(text: string): (line: string) => boolean {
  const t = text.trim()
  if (!t) return () => true
  if (t.length > 2 && t.startsWith('/') && t.endsWith('/')) {
    try {
      const re = new RegExp(t.slice(1, -1), 'i')
      return (l) => re.test(l)
    } catch {
      return () => true
    }
  }
  const low = t.toLowerCase()
  return (l) => l.toLowerCase().includes(low)
}

const ERR = /\b(error|err|fatal|crit(ical)?|panic|fail(ed|ure)?|emerg|alert)\b/i
const WARN = /\b(warn(ing)?)\b/i

export const levelOf = (line: string): 'error' | 'warn' | '' => (ERR.test(line) ? 'error' : WARN.test(line) ? 'warn' : '')
