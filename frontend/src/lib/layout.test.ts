import { describe, expect, it } from 'vitest'
import { clamp, DEFAULT, LIMITS, sanitize } from './layout'

describe('layout', () => {
  it('clamps to the limits', () => {
    expect(clamp(10, LIMITS.rackW)).toBe(220)
    expect(clamp(9999, LIMITS.rackW)).toBe(560)
    expect(clamp(300.4, LIMITS.rackW)).toBe(300)
  })
  it('falls back to defaults for junk', () => {
    expect(sanitize(null)).toEqual(DEFAULT)
    expect(sanitize('x')).toEqual(DEFAULT)
    expect(sanitize({ rackW: 'wide', sideW: NaN, dock: 'top', cols: 5 })).toEqual(DEFAULT)
  })
  it('keeps good values and clamps bad ones', () => {
    const l = sanitize({ rackW: 400, sideW: 5, dock: 'left', rackOpen: false, cols: { size: 90, modified: 9999 } })
    expect(l.rackW).toBe(400)
    expect(l.sideW).toBe(LIMITS.sideW[0])
    expect(l.dock).toBe('left')
    expect(l.rackOpen).toBe(false)
    expect(l.sideOpen).toBe(true)
    expect(l.cols).toEqual({ size: 90, modified: 200 })
  })
})
