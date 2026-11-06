import { describe, expect, it } from 'vitest'
import { levelOf, makeFilter, pushLines } from './logs'

describe('pushLines', () => {
  it('keeps the newest lines when over the limit', () => {
    expect(pushLines(['a', 'b'], ['c', 'd'], 3)).toEqual(['b', 'c', 'd'])
  })
  it('does not change the array it was given', () => {
    const old = ['a']
    pushLines(old, ['b'], 5)
    expect(old).toEqual(['a'])
  })
  it('copes with a batch bigger than the limit', () => {
    expect(pushLines([], ['1', '2', '3', '4'], 2)).toEqual(['3', '4'])
  })
})

describe('makeFilter', () => {
  it('matches plain text without regard to case', () => {
    const f = makeFilter('Error')
    expect(f('an ERROR happened')).toBe(true)
    expect(f('all fine')).toBe(false)
  })
  it('treats /text/ as a regular expression', () => {
    const f = makeFilter('/^(warn|err)/')
    expect(f('warn: x')).toBe(true)
    expect(f('x warn')).toBe(false)
  })
  it('shows everything for an empty or unfinished filter', () => {
    expect(makeFilter('')('x')).toBe(true)
    expect(makeFilter('/(/')('x')).toBe(true)
  })
})

describe('levelOf', () => {
  it('spots errors and warnings', () => {
    expect(levelOf('Nov 6 sshd[1]: error: bad')).toBe('error')
    expect(levelOf('[crit] 12#12: failed')).toBe('error')
    expect(levelOf('level=warning msg=x')).toBe('warn')
    expect(levelOf('GET / 200')).toBe('')
  })
})
