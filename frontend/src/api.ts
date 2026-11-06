import type { Analysis, Dep, Link, HistorySettings, Listing, Policy, Server, StartResult, Step, TextFile, Transfer, TunnelState, Version, Vpn } from './types'
import { mockFiles, mockHistory } from './mockFiles'

/** Everything the UI needs from the Go side. The Wails bindings will implement this same shape. */
/** Files on the server (SFTP) and the transfer queue. Every call needs a live connection. */
export interface FilesApi {
  /** Running containers on the server, for the location picker. Rejects when docker is not available. */
  containers(serverId: string): Promise<string[]>
  // In every call below, serverId may be "server@container" to work inside that container.
  list(serverId: string, dir: string): Promise<Listing>
  read(serverId: string, path: string): Promise<TextFile>
  /** Returns the new version. Rejects with a message containing "changed on the server" when someone else saved first. */
  write(serverId: string, path: string, content: string, version: string): Promise<string>
  mkdir(serverId: string, path: string): Promise<void>
  newFile(serverId: string, path: string): Promise<void>
  rename(serverId: string, from: string, to: string): Promise<void>
  remove(serverId: string, paths: string[]): Promise<void>
  pickUpload(serverId: string, dir: string, folder: boolean): Promise<StartResult>
  upload(serverId: string, dir: string, locals: string[], policy: Policy): Promise<StartResult>
  pickDownload(serverId: string, remotes: string[]): Promise<StartResult>
  download(serverId: string, remotes: string[], dir: string, policy: Policy): Promise<StartResult>
  cancel(id: string): void
  retry(id: string): void
  clear(): void
  /** Hears the whole queue each time it changes; calls back once with the current queue. */
  onTransfers(cb: (t: Transfer[]) => void): () => void
  /** Hears files dropped on the window from the desktop. */
  onDrop(cb: (d: { x: number; y: number; paths: string[] }) => void): () => void
}

/** Earlier versions of edited files. Listing and reading need no connection; restoring does. */
export interface HistoryApi {
  file(serverId: string, path: string): Promise<Version[]>
  /** The latest edits across all files of a server, newest first. */
  server(serverId: string, limit: number): Promise<Version[]>
  content(serverId: string, versionId: string): Promise<string>
  pin(serverId: string, versionId: string, pinned: boolean, note: string): Promise<void>
  /** Writes the version back to the server as a new version. Returns the file's new version. */
  restore(serverId: string, path: string, versionId: string): Promise<string>
  /** Forgets a file's history, or all of the server's when path is empty. */
  remove(serverId: string, path: string): Promise<void>
  settings(): Promise<HistorySettings>
  saveSettings(s: HistorySettings): Promise<void>
  /** Keep a copy of the history on the server itself: on, off, or '' for the default. */
  setCopy(serverId: string, mode: '' | 'on' | 'off'): Promise<void>
}

export interface TunnelsApi {
  list(serverId: string): Promise<TunnelState[]>
  /** Rejects with a readable message, for example when the port is already in use. */
  start(serverId: string, id: string): Promise<void>
  stop(serverId: string, id: string): Promise<void>
  /** Hears about changes to a server's tunnels. Returns a function that stops listening. */
  onChange(serverId: string, cb: (t: TunnelState[]) => void): () => void
}

export interface Backend {
  files: FilesApi
  tunnels: TunnelsApi
  history: HistoryApi
  connect(server: Server, vpn: Vpn | undefined, onSteps: (steps: Step[]) => void): Promise<void>
  confirmManualVpn(serverId: string): void
  disconnect(serverId: string): void
  /** Hears about a dropped connection being redialled. Returns a function that stops listening. */
  onLink(serverId: string, cb: (l: Link) => void): () => void
  analyze(server: Server): Promise<Analysis>
  /** Start a shell's output flowing into onData. Resolves with a function that stops listening. Rejects if the shell cannot open. */
  attach(serverId: string, tab: number, size: { cols: number; rows: number }, onData: (d: string | Uint8Array) => void, onEnd: () => void): Promise<() => void>
  write(serverId: string, tab: number, data: string): void
  resize(serverId: string, tab: number, cols: number, rows: number): void
  closeShell(serverId: string, tab: number): void
  /** Make a tab an SSH session that drops into a running container. Call attach for it afterwards. */
  openContainer(serverId: string, tab: number, container: string, user: string, sudo: boolean): Promise<void>
  deps(): Promise<Dep[]>
  /** Installs through the system package manager; the log streams into onLog. Rejects if it fails. */
  installDeps(names: string[], onLog: (line: string) => void): Promise<void>
}

export class ConnectError extends Error {
  problem: string
  recovery: string
  /** What the VPN client was run as and printed, after a failed VPN step. */
  details?: string
  constructor(problem: string, recovery: string, details?: string) {
    super(problem)
    this.problem = problem
    this.recovery = recovery
    this.details = details
  }
}

const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms))
const hash = (s: string) => [...s].reduce((a, c) => (a * 31 + c.charCodeAt(0)) >>> 0, 7)

const VPN_DEMO_MS = { forti: 2400, gp: 3100, hillstone: 0 } as const

function makeAnalysis(server: Server): Analysis {
  const h = hash(server.id)
  const failed = server.mark === 'struck' && server.id === 'quartz' ? ['php8.5-fpm.service', 'supervisor-queue.service'] : []
  const diskHot = server.id === 'web'
  const docker = h % 3 === 0
  return {
    os: server.os,
    kernel: `5.15.0-${100 + (h % 60)}-generic`,
    uptime: `${20 + (h % 300)} days`,
    software: [
      ...server.software.map((x) => ({ name: x.name, version: x.version })),
      { name: h % 2 ? 'MySQL' : 'MariaDB', version: h % 2 ? '8.0.36' : '10.6.16' },
      { name: 'Node', version: '18.19.1' },
    ],
    cpu: 2 + (h % 3) * 2,
    memTotalMb: 8192,
    memUsedMb: 2600 + (h % 4500),
    disks: [
      { mount: '/', usedPct: diskHot ? 94 : 28 + (h % 40) },
      { mount: '/var', usedPct: diskHot ? 94 : 18 + (h % 55) },
    ],
    failedUnits: failed,
    containers: docker
      ? [
          { name: 'app', image: 'php:8.2-fpm', status: 'running', ports: '9000/tcp' },
          { name: 'redis', image: 'redis:7', status: 'running', ports: '6379/tcp' },
          { name: 'queue-worker', image: 'php:8.2-cli', status: server.id === 'quartz' ? 'restarting' : 'running', ports: '-' },
        ]
      : null,
    scannedAt: Date.now(),
  }
}

const manual = new Map<string, () => void>()

const mockTunnels: TunnelsApi = (() => {
  const running = new Set<string>()
  const subs = new Map<string, Set<(t: TunnelState[]) => void>>()
  const list = (sid: string): TunnelState[] =>
    (demoServers.get(sid)?.tunnels ?? []).map((t) => ({ id: t.id, state: running.has(sid + '/' + t.id) ? 'running' : 'stopped', conns: running.has(sid + '/' + t.id) ? 2 : 0 }))
  const push = (sid: string) => subs.get(sid)?.forEach((cb) => cb(list(sid)))
  return {
    list: async (sid) => list(sid),
    start: async (sid, id) => (running.add(sid + '/' + id), push(sid)),
    stop: async (sid, id) => (running.delete(sid + '/' + id), push(sid)),
    onChange(sid, cb) {
      subs.set(sid, (subs.get(sid) ?? new Set()).add(cb))
      return () => void subs.get(sid)?.delete(cb)
    },
  }
})()

export const mockBackend: Backend = {
  files: mockFiles,
  tunnels: mockTunnels,
  history: mockHistory,
  async connect(server, vpn, onSteps) {
    demoServers.set(server.id, server)
    const steps: Step[] = []
    if (vpn || server.vpnRequired) steps.push({ key: 'vpn', label: 'VPN', status: 'wait', detail: vpn?.name ?? 'not set' })
    steps.push({ key: 'ssh', label: 'SSH', status: 'wait', detail: `${server.user}@${server.host}:${server.port}` }, { key: 'shell', label: 'Shell', status: 'wait' })
    const set = (key: Step['key'], patch: Partial<Step>) => {
      const i = steps.findIndex((x) => x.key === key)
      steps[i] = { ...steps[i], ...patch }
      onSteps([...steps])
    }
    const timed = async (key: Step['key'], ms: number) => {
      set(key, { status: 'run' })
      const t = Date.now()
      await sleep(ms)
      set(key, { status: 'done', ms: Date.now() - t })
    }

    if (!vpn && server.vpnRequired) {
      set('vpn', { status: 'fail' })
      throw new ConnectError(`${server.name} needs a VPN, but no VPN type is recorded.`, 'Set the VPN type in Credentials, then connect again.')
    }
    if (vpn?.kind === 'hillstone') {
      set('vpn', { status: 'manual' })
      const t = Date.now()
      await new Promise<void>((resolve) => manual.set(server.id, resolve))
      set('vpn', { status: 'done', ms: Date.now() - t })
    } else if (vpn) {
      await timed('vpn', VPN_DEMO_MS[vpn.kind])
    }
    await timed('ssh', 420)
    await timed('shell', 160)
  },
  confirmManualVpn: (id) => manual.get(id)?.(),
  async analyze(server) {
    await sleep(700)
    return makeAnalysis(server)
  },
  disconnect: () => {},
  onLink: () => () => {},
  async attach(serverId, tab, _size, onData) {
    const server = demoServers.get(serverId)!
    const inBox = containerTabs.get(`${serverId}:${tab}`)
    const prompt = inBox ? `root@${inBox}:/# ` : promptFor(server)
    shells.set(`${serverId}:${tab}`, { server, onData: onData as (d: string) => void, line: '' })
    onData(`Last login: today from your desk\r\n(demo shell: try uptime, php -v, df -h, free -m, docker ps, systemctl --failed)\r\n${prompt}`)
    return () => {}
  },
  write(serverId, tab, d) {
    const sh = shells.get(`${serverId}:${tab}`)
    if (!sh) return
    if (d === '\r') {
      sh.onData('\r\n' + runDemo(sh.server, sh.line) + promptFor(sh.server))
      sh.line = ''
    } else if (d === '\x7f') {
      if (sh.line) {
        sh.line = sh.line.slice(0, -1)
        sh.onData('\b \b')
      }
    } else if (d >= ' ') {
      sh.line += d
      sh.onData(d)
    }
  },
  resize: () => {},
  closeShell: (serverId, tab) => void shells.delete(`${serverId}:${tab}`),
  async openContainer(serverId, tab, container) {
    containerTabs.set(`${serverId}:${tab}`, container)
  },
  async deps() {
    return demoDeps.map((d) => ({ ...d }))
  },
  async installDeps(names, onLog) {
    for (const n of names) {
      onLog(`Reading package lists... Done`)
      await sleep(500)
      onLog(`Setting up ${n} ...`)
      await sleep(500)
      const d = demoDeps.find((x) => x.name === n)
      if (d) d.installed = true
    }
  },
}

const containerTabs = new Map<string, string>()
const demoDeps: Dep[] = [
  { name: 'openfortivpn', purpose: 'FortiClient VPN (HQ old, Quartz Reports)', installed: false, installable: true },
  { name: 'openconnect', purpose: 'GlobalProtect VPN (HQ, Portal)', installed: true, installable: true },
  { name: 'pkexec', purpose: 'Asks for your admin password to start a VPN (part of polkit)', installed: true, installable: false },
]

const demoServers = new Map<string, Server>()
const shells = new Map<string, { server: Server; onData: (d: string) => void; line: string }>()
const promptFor = (s: Server) => `\x1b[32m${s.user}@${s.name.toLowerCase().replace(/\s+/g, '-')}\x1b[0m:~$ `

function runDemo(server: Server, line: string): string {
    const a = makeAnalysis(server)
    const cmd = line.trim()
    const out = (s: string) => s.replace(/\n/g, '\r\n') + '\r\n'
    if (!cmd) return ''
    if (cmd === 'uptime') return out(` up ${a.uptime}, 1 user, load average: 0.42, 0.51, 0.47`)
    if (cmd === 'php -v') return out(`PHP ${a.software.find((x) => x.name === 'PHP')?.version ?? '?'} (cli)\nZend Engine`)
    if (cmd === 'df -h') return out('Filesystem  Size  Used Avail Use% Mounted on\n' + a.disks.map((d) => `/dev/vda1    80G  ${d.usedPct}%  ${100 - d.usedPct}% ${d.usedPct}% ${d.mount}`).join('\n'))
    if (cmd === 'free -m') return out(`              total   used\nMem:          ${a.memTotalMb}   ${a.memUsedMb}`)
    if (cmd === 'systemctl --failed') return out(a.failedUnits.length ? a.failedUnits.map((u) => `● ${u} failed`).join('\n') : '0 loaded units listed.')
    if (cmd === 'docker ps') return out(a.containers ? 'NAMES         IMAGE          STATUS\n' + a.containers.map((c) => `${c.name.padEnd(13)} ${c.image.padEnd(14)} ${c.status}`).join('\n') : 'bash: docker: command not found')
    if (cmd === 'ls') return out('app  releases  shared  storage')
    return out(`bash: ${cmd.split(' ')[0]}: command not found (demo shell)`)
}
