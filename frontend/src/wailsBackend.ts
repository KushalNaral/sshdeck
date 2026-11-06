import * as App from '../wailsjs/go/main/App'
import { EventsOn } from '../wailsjs/runtime/runtime'
import { ConnectError, type ActionsApi, type Backend, type FilesApi, type ObserveApi, type HistoryApi, type LogsApi, type RunApi, type TunnelsApi } from './api'
import type { ActionResult, Alert, Analysis, HistorySettings, Link, Listing, MonitorSettings, Proc, PushResult, RunResult, Sample, SearchHit, Step, Transfer, TunnelState, Version } from './types'

const fromBase64 = (b64: string) => Uint8Array.from(atob(b64), (c) => c.charCodeAt(0))

/** The real backend: every call goes to Go. Progress and terminal output arrive as events. */
const msg = (e: unknown) => new Error(String(e instanceof Error ? e.message : e))
/** Wraps a Go call so a rejection becomes an Error with the Go message. */
const go = async <T>(p: Promise<T>): Promise<T> => {
  try {
    return await p
  } catch (e) {
    throw msg(e)
  }
}

const files: FilesApi = {
  containers: async (s) => ((await go(App.Containers(s))) ?? []) as string[],
  list: (s, d) => go(App.ListDir(s, d)) as Promise<Listing>,
  search: async (s, d, q, c) => ((await go(App.SearchFiles(s, d, q, c))) ?? []) as SearchHit[],
  read: (s, p) => go(App.ReadText(s, p)),
  write: (s, p, c, v) => go(App.WriteText(s, p, c, v)),
  mkdir: (s, p) => go(App.Mkdir(s, p)),
  newFile: (s, p) => go(App.NewFile(s, p)),
  rename: (s, a, b) => go(App.Rename(s, a, b)),
  remove: (s, p) => go(App.Delete(s, p)),
  pickUpload: (s, d, f) => go(App.PickUpload(s, d, f)),
  upload: (s, d, l, p) => go(App.Upload(s, d, l, p)),
  pickDownload: (s, r) => go(App.PickDownload(s, r)),
  download: (s, r, d, p) => go(App.Download(s, r, d, p)),
  cancel: (id) => void App.CancelTransfer(id),
  retry: (id) => void App.RetryTransfer(id),
  clear: () => void App.ClearTransfers(),
  onTransfers(cb) {
    void App.Transfers().then((t) => cb((t ?? []) as Transfer[]))
    return EventsOn('transfers', (t: Transfer[]) => cb(t ?? []))
  },
  onDrop: (cb) => EventsOn('files:dropped', cb),
}

const tunnels: TunnelsApi = {
  list: async (s) => ((await App.Tunnels(s)) ?? []) as TunnelState[],
  start: (s, id) => go(App.StartTunnel(s, id)),
  stop: (s, id) => go(App.StopTunnel(s, id)),
  onChange: (s, cb) => EventsOn('tunnels:' + s, (t: TunnelState[]) => cb(t ?? [])),
}

const logs: LogsApi = {
  start: (t, k, a) => go(App.StartLog(t, k, a)),
  stop: (id) => void App.StopLog(id),
  onLines: (id, cb) => EventsOn('log:' + id, (l: string[]) => cb(l ?? [])),
  onEnd: (id, cb) => EventsOn('log:' + id + ':end', (w: string) => cb(w ?? '')),
}

const run: RunApi = {
  start: (ids, cmd) => go(App.RunOn(ids, cmd)),
  cancel: (id) => void App.CancelRun(id),
  push: async (ids, dir, folder, policy) => ((await go(App.PickPush(ids, dir, folder, policy))) ?? null) as PushResult[] | null,
  results: async (id) => ((await App.RunResults(id)) ?? []) as RunResult[],
  onResult: (id, cb) => EventsOn('run:' + id, cb),
}

const observe: ObserveApi = {
  latest: async () => ((await App.MetricsLatest()) ?? {}) as unknown as Record<string, Sample>,
  range: async (id, from, to) => ((await App.MetricsRange(id, Math.floor(from), Math.floor(to))) ?? []) as unknown as Sample[],
  onSamples: (cb) => EventsOn('metrics', (m: Record<string, Sample>) => cb(m ?? {})),
  alerts: async () => ((await App.Alerts()) ?? []) as unknown as Alert[],
  onAlerts: (cb) => EventsOn('alerts', (a: Alert[]) => cb(a ?? [])),
  onAnalysis: (id, cb) =>
    EventsOn('analysis:' + id, (r: Analysis) => cb({ ...r, disks: r.disks ?? [], failedUnits: r.failedUnits ?? [], containers: r.containers ?? null, scannedAt: Date.now() })),
  top: async (id) => ((await go(App.TopProcesses(id))) ?? []) as Proc[],
  settings: () => go(App.GetMonitorSettings()) as Promise<MonitorSettings>,
  saveSettings: (s) => go(App.SetMonitorSettings(s as never)),
}

const actions: ActionsApi = {
  service: (id, unit, action) => go(App.ServiceAction(id, unit, action)) as Promise<ActionResult>,
  container: (id, name, action) => go(App.ContainerAction(id, name, action)) as Promise<ActionResult>,
}

const history: HistoryApi = {
  file: async (s, p) => ((await go(App.FileHistory(s, p))) ?? []) as Version[],
  server: async (s, n) => ((await go(App.ServerHistory(s, n))) ?? []) as Version[],
  content: (s, id) => go(App.HistoryContent(s, id)),
  pin: (s, id, pinned, note) => go(App.PinVersion(s, id, pinned, note)),
  restore: (s, p, id) => go(App.RestoreVersion(s, p, id)),
  remove: (s, p) => go(App.DeleteHistory(s, p)),
  settings: async () => {
    const h = await go(App.GetHistorySettings())
    return { ...h, deny: h.deny ?? [] } as HistorySettings
  },
  saveSettings: (s) => go(App.SetHistorySettings(s as never)),
  setCopy: (s, m) => go(App.SetHistoryCopy(s, m)),
}

export const wailsBackend: Backend = {
  files,
  logs,
  tunnels,
  run,
  observe,
  actions,
  history,
  async connect(server, _vpn, onSteps) {
    const off = EventsOn(`connect:${server.id}`, (steps: Step[]) => onSteps(steps))
    try {
      const r = await App.Connect(server.id)
      if (!r.ok) throw new ConnectError(r.problem || 'The connection failed.', r.recovery || 'Try again.', r.details)
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
