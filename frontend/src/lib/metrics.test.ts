import { describe, expect, it } from 'vitest'
import type { Sample } from '../types'
import { ago, fmtBytes, fmtRate, freshness, niceMax, pushRing, series, withGaps, worstDisk } from './metrics'

const smp = (t: number, over: Partial<Sample> = {}): Sample => ({
  t, hasRates: true, cpu: 10, steal: 0, load1: 0, load5: 0, load15: 0, cores: 2, memPct: 50, memUsedMb: 1, memTotalMb: 2, swapPct: 0,
  netRx: 0, netTx: 0, diskR: 0, diskW: 0, disks: [], ...over,
})

describe('formatting', () => {
  it('uses binary units and one decimal under 10', () => {
    expect(fmtBytes(0)).toBe('0 B')
    expect(fmtBytes(900)).toBe('900 B')
    expect(fmtBytes(1536)).toBe('1.5 KB')
    expect(fmtBytes(15 * 1024 * 1024)).toBe('15 MB')
    expect(fmtRate(2048)).toBe('2.0 KB/s')
  })
  it('never prints nonsense for bad input', () => {
    expect(fmtBytes(-5)).toBe('0 B')
    expect(fmtBytes(NaN)).toBe('0 B')
  })
  it('says how long ago', () => {
    expect(ago(1000, 1010)).toBe('just now')
    expect(ago(1000, 1000 + 300)).toBe('5m ago')
    expect(ago(1000, 1000 + 7200)).toBe('2h ago')
    expect(ago(1000, 1000 + 86400 * 3)).toBe('3d ago')
  })
})

describe('series', () => {
  it('leaves a hole where a rate was not measured', () => {
    const s = series([smp(1, { hasRates: false, cpu: 0 }), smp(2, { cpu: 40 })], (x) => x.cpu, true)
    expect(s.map((p) => p.v)).toEqual([null, 40])
  })
  it('keeps levels even without rates', () => {
    const s = series([smp(1, { hasRates: false, memPct: 70 })], (x) => x.memPct)
    expect(s[0].v).toBe(70)
  })
})

describe('withGaps', () => {
  it('breaks the line across a long silence only', () => {
    const pts = [0, 5, 10, 15, 400, 405].map((t) => ({ t, v: 1 as number | null }))
    const out = withGaps(pts)
    expect(out.filter((p) => p.v === null)).toHaveLength(1)
    expect(out.findIndex((p) => p.v === null)).toBe(4)
  })
  it('leaves an even series alone', () => {
    const pts = [0, 5, 10, 15].map((t) => ({ t, v: 1 as number | null }))
    expect(withGaps(pts)).toEqual(pts)
  })
})

describe('fleet helpers', () => {
  it('finds the fullest disk', () => {
    expect(worstDisk(smp(1, { disks: [{ mount: '/', usedPct: 40 }, { mount: '/var', usedPct: 94 }] }))).toEqual({ mount: '/var', usedPct: 94 })
    expect(worstDisk(smp(1))).toBeNull()
    expect(worstDisk(undefined)).toBeNull()
  })
  it('knows live from stale', () => {
    expect(freshness(undefined, 100)).toBe('none')
    expect(freshness(smp(95), 100)).toBe('live')
    expect(freshness(smp(10), 100)).toBe('stale')
  })
  it('keeps a bounded ring and ignores a repeat', () => {
    let r: Sample[] = []
    for (let i = 1; i <= 5; i++) r = pushRing(r, smp(i), 3)
    expect(r.map((x) => x.t)).toEqual([3, 4, 5])
    expect(pushRing(r, smp(5), 3)).toBe(r)
  })
  it('rounds an axis up to a readable top', () => {
    expect(niceMax([3, 8], 5)).toBe(10)
    expect(niceMax([0.4], 1)).toBe(1)
    expect(niceMax([130], 10)).toBe(150)
    expect(niceMax([null], 4)).toBe(5)
  })
})
