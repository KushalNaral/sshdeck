import { describe, expect, it } from 'vitest'
import type { Analysis, Server, Session, Task } from '../types'
import { triage } from './triage'

const server = { id: 's1', name: 'Atlas', php: '8.2.35', phpTarget: '8.2.35', projects: [] } as unknown as Server
const healthy = {
  disks: [{ mount: '/', usedPct: 40 }],
  memUsedMb: 2000,
  memTotalMb: 8000,
  failedUnits: [],
  containers: null,
} as unknown as Analysis
const task = (over: Partial<Task>): Task => ({ id: 't', title: 'Patch', status: 'todo', tags: [], serverIds: ['s1'], created: 0, updated: 0, ...over })
const TODAY = '2026-11-04'

describe('triage level', () => {
  it('is 5 when nothing is owed', () => {
    const t = triage(server, healthy, [], undefined, TODAY)
    expect(t.level).toBe(5)
    expect(t.findings[0].text).toMatch(/nothing owed/i)
  })

  it('is 1 for a failed service, a nearly full disk, or a failed connect', () => {
    expect(triage(server, { ...healthy, failedUnits: ['php8.2-fpm.service'] }, [], undefined, TODAY).level).toBe(1)
    expect(triage(server, { ...healthy, disks: [{ mount: '/var', usedPct: 96 }] }, [], undefined, TODAY).level).toBe(1)
    const failed = { state: 'failed', error: { problem: 'wrong password', recovery: '' } } as unknown as Session
    const t = triage(server, undefined, [], failed, TODAY)
    expect(t.level).toBe(1)
    expect(t.findings[0].text).toContain('wrong password')
  })

  it('is 2 for a disk at 85%, memory at 95%, or a restarting container', () => {
    expect(triage(server, { ...healthy, disks: [{ mount: '/', usedPct: 88 }] }, [], undefined, TODAY).level).toBe(2)
    expect(triage(server, { ...healthy, memUsedMb: 7700 }, [], undefined, TODAY).level).toBe(2)
    const c = { ...healthy, containers: [{ name: 'queue', image: 'x', status: 'restarting', ports: '' }] } as unknown as Analysis
    expect(triage(server, c, [], undefined, TODAY).level).toBe(2)
  })

  it('is 3 when PHP needs an upgrade or a task is overdue', () => {
    expect(triage({ ...server, php: '7.4.27' }, healthy, [], undefined, TODAY).level).toBe(3)
    const late = triage(server, healthy, [task({ due: '2026-11-01' })], undefined, TODAY)
    expect(late.level).toBe(3)
    expect(late.findings[0].text).toMatch(/overdue/i)
  })

  it('is 4 when PHP needs a patch, a task is open, or a container has stopped', () => {
    expect(triage({ ...server, php: '8.2.25' }, healthy, [], undefined, TODAY).level).toBe(4)
    expect(triage(server, healthy, [task({ due: '2026-12-01' })], undefined, TODAY).level).toBe(4)
    const c = { ...healthy, containers: [{ name: 'old', image: 'x', status: 'exited', ports: '' }] } as unknown as Analysis
    expect(triage(server, c, [], undefined, TODAY).level).toBe(4)
  })

  it('ignores done tasks and tasks about other servers', () => {
    const t = triage(server, healthy, [task({ status: 'done', due: '2020-01-01' }), task({ serverIds: ['other'] })], undefined, TODAY)
    expect(t.level).toBe(5)
  })

  it('takes the worst finding as the level and lists findings worst first', () => {
    const t = triage({ ...server, php: '8.2.25' }, { ...healthy, failedUnits: ['a.service', 'b.service'] }, [task({})], undefined, TODAY)
    expect(t.level).toBe(1)
    expect(t.findings.map((f) => f.level)).toEqual([...t.findings.map((f) => f.level)].sort((a, b) => a - b))
    expect(t.findings[0].text).toContain('2 failed services')
  })

  it('does not call a server with no recorded PHP version behind', () => {
    expect(triage({ ...server, php: '' }, healthy, [], undefined, TODAY).level).toBe(5)
  })

  it('judges from what is known: with no scan, only PHP and tasks count', () => {
    expect(triage({ ...server, php: '7.4.27' }, undefined, [], undefined, TODAY).level).toBe(3)
  })
})
