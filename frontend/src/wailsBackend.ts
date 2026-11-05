import * as App from '../wailsjs/go/main/App'
import { EventsOn } from '../wailsjs/runtime/runtime'
import { ConnectError, type Backend } from './api'
import type { Analysis, Link, Step } from './types'

const fromBase64 = (b64: string) => Uint8Array.from(atob(b64), (c) => c.charCodeAt(0))

/** The real backend: every call goes to Go. Progress and terminal output arrive as events. */
export const wailsBackend: Backend = {
  async connect(server, _vpn, onSteps) {
    const off = EventsOn(`connect:${server.id}`, (steps: Step[]) => onSteps(steps))
    try {
      const r = await App.Connect(server.id)
      if (!r.ok) throw new ConnectError(r.problem || 'The connection failed.', r.recovery || 'Try again.')
    } finally {
      off()
    }
  },
  confirmManualVpn: (id) => void App.ConfirmManualVPN(id),
  disconnect: (id) => void App.Disconnect(id),
  onLink: (id, cb) => EventsOn(`link:${id}`, (l: Link) => cb(l)),

  async analyze(server) {
    const r = await App.Analyze(server.id)
    return { ...r, disks: r.disks ?? [], failedUnits: r.failedUnits ?? [], containers: r.containers ?? null, scannedAt: Date.now() } as unknown as Analysis
  },

  async attach(serverId, tab, { cols, rows }, onData, onEnd) {
    const offOut = EventsOn(`term:${serverId}:${tab}:out`, (b64: string) => onData(fromBase64(b64)))
    const offEnd = EventsOn(`term:${serverId}:${tab}:end`, onEnd)
    const off = () => (offOut(), offEnd())
    try {
      await App.AttachShell(serverId, tab, cols, rows)
    } catch (e) {
      off()
      throw new Error(String(e))
    }
    return off
  },
  write: (id, tab, data) => void App.TermWrite(id, tab, data),
  resize: (id, tab, cols, rows) => void App.TermResize(id, tab, cols, rows),
  closeShell: (id, tab) => void App.CloseShell(id, tab),
  async openContainer(id, tab, container, user, sudo) {
    try {
      await App.OpenContainerShell(id, tab, container, user, sudo)
    } catch (e) {
      throw new Error(String(e))
    }
  },
  deps: () => App.CheckDependencies(),
  async installDeps(names, onLog) {
    const off = EventsOn('deps:log', (line: string) => onLog(line))
    try {
      await App.InstallDependencies(names)
    } catch (e) {
      throw new Error(String(e))
    } finally {
      off()
    }
  },
}
