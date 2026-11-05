import type { Analysis, Server, Session, Task } from '../types'
import { versionGap } from './version'

export type Level = 1 | 2 | 3 | 4 | 5

export const LEVEL_NAME: Record<Level, string> = { 1: 'Immediate', 2: 'Very urgent', 3: 'Urgent', 4: 'Standard', 5: 'Clear' }

export interface Finding {
  level: Level
  text: string
}

export interface Triage {
  /** The worst finding's level: 1 needs you first, 5 owes nothing. */
  level: Level
  /** Why, worst first. Never empty: a level always has a visible reason. */
  findings: Finding[]
}

const plural = (n: number, one: string, many = `${one}s`) => `${n} ${n === 1 ? one : many}`

/**
 * A ward-board severity for one server, from what the app already knows: the last connect, the last scan,
 * the tracked software and the open tasks. Every level comes with the reasons that set it.
 * today is YYYY-MM-DD (a seam for tests).
 */
export function triage(server: Server, a: Analysis | undefined, tasks: Task[], session: Session | undefined, today: string): Triage {
  const f: Finding[] = []
  const add = (level: Level, text: string) => f.push({ level, text })

  if (session?.link?.state === 'lost') add(1, `Connection lost: ${session.link.problem ?? 'the link dropped'}`)
  if (session?.state === 'failed') add(1, `Last connect failed: ${session.error?.problem ?? 'unknown reason'}`)

  if (a) {
    if (a.failedUnits.length) add(1, `${plural(a.failedUnits.length, 'failed service')}: ${a.failedUnits.join(', ')}`)
    for (const d of a.disks) {
      if (d.usedPct >= 95) add(1, `Disk ${d.mount} at ${d.usedPct}%`)
      else if (d.usedPct >= 85) add(2, `Disk ${d.mount} at ${d.usedPct}%`)
    }
    const mem = a.memTotalMb > 0 ? Math.round((a.memUsedMb / a.memTotalMb) * 100) : 0
    if (mem >= 95) add(2, `Memory at ${mem}%`)
    const restarting = (a.containers ?? []).filter((c) => c.status === 'restarting')
    if (restarting.length) add(2, `${plural(restarting.length, 'container')} restarting: ${restarting.map((c) => c.name).join(', ')}`)
    const stopped = (a.containers ?? []).filter((c) => c.status === 'exited')
    if (stopped.length) add(4, `${plural(stopped.length, 'container')} stopped: ${stopped.map((c) => c.name).join(', ')}`)
  }

  for (const x of server.software) {
    if (!x.version || !x.target) continue
    const gap = versionGap(x.version, x.target)
    if (gap === 'branch') add(3, `${x.name} ${x.version} needs an upgrade to ${x.target}`)
    else if (gap === 'patch') add(4, `${x.name} ${x.version} needs a patch to ${x.target}`)
  }

  const mine = tasks.filter((t) => t.status !== 'done' && t.serverIds.includes(server.id))
  const late = mine.filter((t) => t.due && t.due < today)
  if (late.length) add(3, `Overdue: ${late.map((t) => t.title).join(', ')}`)
  const open = mine.length - late.length
  if (open > 0) add(4, `${plural(open, 'open task')}`)

  if (!f.length) add(5, 'Nothing owed')
  f.sort((x, y) => x.level - y.level)
  return { level: f[0].level, findings: f }
}

/** "scanned 4m ago", or "not scanned yet". */
export function scannedLabel(scannedAt: number | undefined, now: number): string {
  if (!scannedAt) return 'not scanned yet'
  const m = Math.max(0, Math.round((now - scannedAt) / 60000))
  if (m < 1) return 'scanned just now'
  if (m < 60) return `scanned ${m}m ago`
  const h = Math.round(m / 60)
  return h < 48 ? `scanned ${h}h ago` : `scanned ${Math.round(h / 24)}d ago`
}
