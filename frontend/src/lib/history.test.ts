import { describe, expect, it } from 'vitest'
import type { Version } from '../types'
import { ago, by, matches, sizeDelta } from './history'

const v = (o: Partial<Version>): Version => ({ id: '1', server: 's', path: '/etc/a.conf', time: 0, author: '', login: '', kind: 'save', hash: 'h', size: 10, ...o })

describe('history helpers', () => {
  const now = Date.UTC(2026, 10, 6, 12)
  it('says how long ago', () => {
    expect(ago(now - 10_000, now)).toBe('just now')
    expect(ago(now - 5 * 60_000, now)).toBe('5 min ago')
    expect(ago(now - 3 * 3600_000, now)).toBe('3 h ago')
    expect(ago(now - 30 * 3600_000, now)).toBe('yesterday')
    expect(ago(now - 4 * 86400_000, now)).toBe('4 days ago')
    expect(ago(now - 90 * 86400_000, now)).toMatch(/2026|Aug/)
  })
  it('names who made a change', () => {
    expect(by(v({ author: 'sam@laptop', login: 'deploy' }))).toBe('sam@laptop as deploy')
    expect(by(v({ kind: 'outside' }))).toBe('someone, outside sshdeck')
    expect(by(v({ kind: 'initial' }))).toBe('before sshdeck')
  })
  it('shows size change', () => {
    expect(sizeDelta(v({ size: 15 }), v({ size: 10 }))).toBe('+5 B')
    expect(sizeDelta(v({ size: 4 }), v({ size: 10 }))).toBe('−6 B')
    expect(sizeDelta(v({}), v({}))).toBe('same size')
    expect(sizeDelta(v({}))).toBe('')
  })
  it('filters by words in path, person, kind or note', () => {
    expect(matches(v({ author: 'sam@desk' }), 'sam')).toBe(true)
    expect(matches(v({ note: 'before upgrade' }), 'upgrade')).toBe(true)
    expect(matches(v({}), 'nginx')).toBe(false)
    expect(matches(v({}), '  ')).toBe(true)
  })
})
