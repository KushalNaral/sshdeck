import { describe, expect, it } from 'vitest'
import { basename, crumbs, dirname, humanSize, join, shortDate, splitHist, targetOf, validName } from './files'

describe('files helpers', () => {
  it('splits and joins slash paths', () => {
    expect(dirname('/a/b/c.txt')).toBe('/a/b')
    expect(dirname('/a')).toBe('/')
    expect(basename('/a/b/c.txt')).toBe('c.txt')
    expect(join('/', 'etc')).toBe('/etc')
    expect(join('/etc', 'hosts')).toBe('/etc/hosts')
  })
  it('makes breadcrumbs', () => {
    expect(crumbs('/')).toEqual([{ name: '/', path: '/' }])
    expect(crumbs('/var/log').map((c) => c.path)).toEqual(['/', '/var', '/var/log'])
  })
  it('formats sizes', () => {
    expect(humanSize(0)).toBe('0 B')
    expect(humanSize(1536)).toBe('1.5 KB')
    expect(humanSize(5 * 1024 * 1024)).toBe('5.0 MB')
    expect(humanSize(300 * 1024 * 1024)).toBe('300 MB')
  })
  it('shows time for recent files and the year for old ones', () => {
    const now = Date.UTC(2025, 10, 6, 12)
    expect(shortDate(Date.UTC(2025, 10, 3, 9, 5) / 1000, now)).toMatch(/^\d+ Nov \d\d:\d\d$/)
    expect(shortDate(Date.UTC(2023, 0, 20, 9) / 1000, now)).toBe('20 Jan 2023')
  })
  it('rejects bad names', () => {
    expect(validName('')).not.toBeNull()
    expect(validName('a/b')).not.toBeNull()
    expect(validName('..')).not.toBeNull()
    expect(validName('notes.md')).toBeNull()
  })
  it('tells container files from the server\'s own in history', () => {
    expect(splitHist('docker:web:/etc/app.env')).toEqual({ container: 'web', path: '/etc/app.env' })
    expect(splitHist('/etc/hosts')).toEqual({ container: '', path: '/etc/hosts' })
    expect(splitHist('docker:bad name:/x')).toEqual({ container: '', path: 'docker:bad name:/x' })
    expect(targetOf('srv', '')).toBe('srv')
    expect(targetOf('srv', 'web')).toBe('srv@web')
  })
})
