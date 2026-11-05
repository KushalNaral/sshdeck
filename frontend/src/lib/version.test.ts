import { describe, expect, it } from 'vitest'
import { compareVersions, versionGap } from './version'

describe('versions', () => {
  it('orders versions numerically, not lexically', () => {
    expect(compareVersions('8.2.9', '8.2.35')).toBeLessThan(0)
    expect(compareVersions('8.4.27', '8.4.27')).toBe(0)
    expect(compareVersions('10.0.0', '9.9.9')).toBeGreaterThan(0)
  })

  it('classifies the gap to the patched target', () => {
    expect(versionGap('8.2.35', '8.2.35')).toBe('current')
    expect(versionGap('8.2.25', '8.2.35')).toBe('patch')
    expect(versionGap('7.4.27', '8.2.35')).toBe('branch')
    expect(versionGap('8.5.4', '8.4.27')).toBe('current')
  })
})
