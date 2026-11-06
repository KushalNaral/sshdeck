import type { ActionsApi, ObserveApi } from './api'
import type { Alert, MonitorSettings, Proc, Sample } from './types'

// Browser-build stand-in for the sampler: smooth made-up curves per server, so the Observe view has something to
// draw. Every name is invented; the numbers are not measurements of anything.

const hash = (s: string) => [...s].reduce((a, c) => (a * 31 + c.charCodeAt(0)) >>> 0, 7)
const clamp = (v: number, lo: number, hi: number) => Math.min(hi, Math.max(lo, v))

/** What each demo server looks like at rest. Others get something plausible from their id. */
const PROFILE: Record<string, { cpu: number; mem: number; disk: number }> = {
  web: { cpu: 38, mem: 71, disk: 94 },
  quartz: { cpu: 94, mem: 83, disk: 61 },
  birch: { cpu: 46, mem: 64, disk: 58 },
  atlas: { cpu: 22, mem: 49, disk: 43 },
}

export function synth(id: string, t: number): Sample {
  const h = hash(id)
  const p = PROFILE[id] ?? { cpu: 8 + (h % 30), mem: 30 + (h % 40), disk: 25 + (h % 45) }
  const wave = (period: number, amp: number, phase = 0) => amp * Math.sin(t / period + h + phase)
  const cpu = clamp(p.cpu + wave(260, 9) + wave(37, 3) + wave(5400, 6), 1, 99)
  const cores = 2 + (h % 3) * 2
  const memTotalMb = 4096 * (1 + (h % 4))
  const memPct = clamp(p.mem + wave(900, 2.5) + wave(9000, 4), 5, 99)
  return {
    t: Math.floor(t),
    hasRates: true,
    cpu,
    steal: clamp(0.6 + wave(400, 0.5), 0, 4),
    load1: Math.max(0.05, (cpu / 100) * cores * 0.9 + wave(120, 0.2)),
    load5: Math.max(0.05, (cpu / 100) * cores * 0.8),
    load15: Math.max(0.05, (cpu / 100) * cores * 0.7),
    cores,
    memPct,
    memUsedMb: Math.round((memPct / 100) * memTotalMb),
    memTotalMb,
    swapPct: clamp(p.mem / 8 + wave(700, 1), 0, 100),
    netRx: Math.max(0, 140_000 + wave(190, 90_000) + wave(23, 30_000)),
    netTx: Math.max(0, 60_000 + wave(230, 40_000) + wave(31, 15_000)),
    diskR: Math.max(0, 80_000 + wave(310, 70_000)),
    diskW: Math.max(0, 210_000 + wave(170, 150_000)),
    disks: [
      { mount: '/', usedPct: Math.round(p.disk) },
      { mount: '/var', usedPct: Math.round(clamp(p.disk * 0.8 + 6, 5, 99)) },
    ],
  }
}

const now = () => Date.now() / 1000

export function mockObserve(connected: () => string[]): ObserveApi {
  let settings: MonitorSettings = { off: false, sampleSeconds: 5, scanMinutes: 15, diskWarn: 90, diskCrit: 95, memWarn: 95, cpuWarn: 90, noNotify: false }
  const latest = () => Object.fromEntries(connected().map((id) => [id, synth(id, now())]))
  const alerts = (): Alert[] => {
    const base = Math.floor(now() / 3600) * 3600 - 600
    const out: Alert[] = []
    for (const id of connected()) {
      const s = synth(id, now())
      for (const d of s.disks) {
        if (d.usedPct >= 90) out.push({ id: `${id}/disk/${d.mount}`, server: id, kind: 'disk', subject: d.mount, severity: d.usedPct >= 95 ? 'critical' : 'warning', text: `Disk ${d.mount} at ${d.usedPct}%`, since: base })
      }
      if (id === 'quartz') {
        out.push({ id: 'quartz/service/php8.5-fpm.service', server: id, kind: 'service', subject: 'php8.5-fpm.service', severity: 'critical', text: 'Service php8.5-fpm.service failed', since: base - 1500 })
        if (s.cpu >= 90) out.push({ id: 'quartz/cpu/', server: id, kind: 'cpu', subject: '', severity: 'warning', text: `CPU at ${Math.round(s.cpu)}%`, since: base + 200 })
      }
    }
    const old: Alert = { id: 'birch/memory/', server: 'birch', kind: 'memory', subject: '', severity: 'warning', text: 'Memory at 96%', since: base - 86400, resolved: base - 82000 }
    return [...out.sort((a, b) => a.since - b.since).reverse(), ...(connected().includes('birch') ? [old] : [])]
  }
  const every = <T,>(ms: number, get: () => T, cb: (v: T) => void) => {
    const id = setInterval(() => cb(get()), ms)
    return () => clearInterval(id)
  }
  return {
    latest: async () => latest(),
    async range(id, from, to) {
      const n = 120
      const step = (to - from) / n
      return Array.from({ length: n + 1 }, (_, i) => synth(id, from + i * step))
    },
    onSamples: (cb) => every(5000, latest, cb),
    alerts: async () => alerts(),
    onAlerts: (cb) => every(10000, alerts, cb),
    onAnalysis: () => () => {},
    async top(id) {
      const s = synth(id, now())
      const rows: [string, string, number, number][] = [
        ['php-fpm: pool www', 'www-data', s.cpu * 0.5, 4.1],
        ['postgres: writer', 'postgres', s.cpu * 0.2, 6.3],
        ['nginx: worker', 'www-data', s.cpu * 0.12, 0.8],
        ['node server.js', 'deploy', s.cpu * 0.08, 3.2],
        ['dockerd', 'root', 1.1, 2.4],
        ['sshd: deploy', 'deploy', 0.2, 0.1],
      ]
      return rows.map(([command, user, cpu, mem], i): Proc => ({ pid: 812 + i * 97, user, cpu: Math.round(cpu * 10) / 10, mem, command }))
    },
    settings: async () => settings,
    async saveSettings(s) {
      settings = s
    },
  }
}

export const mockActions: ActionsApi = {
  async service(_id, unit, action) {
    await new Promise((r) => setTimeout(r, 500))
    if (action === 'status') {
      return { exit: 3, output: `● ${unit} - Demo service\n     Loaded: loaded (/lib/systemd/system/${unit}; enabled)\n     Active: failed (Result: exit-code) since Mon 2026-11-02 09:14:07 UTC; 4 days ago\n    Process: 1182 ExecStart=/usr/bin/demo (code=exited, status=1/FAILURE)\n\nNov 02 09:14:07 demo systemd[1]: ${unit}: Main process exited, code=exited, status=1/FAILURE\nNov 02 09:14:07 demo systemd[1]: ${unit}: Failed with result 'exit-code'.` }
    }
    return { exit: 0, output: '' }
  },
  async container(_id, _name, _action) {
    await new Promise((r) => setTimeout(r, 500))
    return { exit: 0, output: '' }
  },
}
