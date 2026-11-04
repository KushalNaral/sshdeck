import type { Analysis, Server, Step, Vpn } from './types'

/** Everything the UI needs from the Go side. The Wails bindings will implement this same shape. */
export interface Backend {
  connect(server: Server, vpn: Vpn | undefined, onSteps: (steps: Step[]) => void): Promise<void>
  confirmManualVpn(serverId: string): void
  analyze(server: Server): Promise<Analysis>
  exec(server: Server, line: string): string
}

export class ConnectError extends Error {
  problem: string
  recovery: string
  constructor(problem: string, recovery: string) {
    super(problem)
    this.problem = problem
    this.recovery = recovery
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
    php: server.php,
    webserver: server.webserver,
    db: h % 2 ? 'MySQL 8.0.36' : 'MariaDB 10.6.16',
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

export const mockBackend: Backend = {
  async connect(server, vpn, onSteps) {
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
  exec(server, line) {
    const a = makeAnalysis(server)
    const cmd = line.trim()
    const out = (s: string) => s.replace(/\n/g, '\r\n') + '\r\n'
    if (!cmd) return ''
    if (cmd === 'uptime') return out(` up ${a.uptime}, 1 user, load average: 0.42, 0.51, 0.47`)
    if (cmd === 'php -v') return out(`PHP ${a.php} (cli)\nZend Engine`)
    if (cmd === 'df -h') return out('Filesystem  Size  Used Avail Use% Mounted on\n' + a.disks.map((d) => `/dev/vda1    80G  ${d.usedPct}%  ${100 - d.usedPct}% ${d.usedPct}% ${d.mount}`).join('\n'))
    if (cmd === 'free -m') return out(`              total   used\nMem:          ${a.memTotalMb}   ${a.memUsedMb}`)
    if (cmd === 'systemctl --failed') return out(a.failedUnits.length ? a.failedUnits.map((u) => `● ${u} failed`).join('\n') : '0 loaded units listed.')
    if (cmd === 'docker ps') return out(a.containers ? 'NAMES         IMAGE          STATUS\n' + a.containers.map((c) => `${c.name.padEnd(13)} ${c.image.padEnd(14)} ${c.status}`).join('\n') : 'bash: docker: command not found')
    if (cmd === 'ls') return out('app  releases  shared  storage')
    return out(`bash: ${cmd.split(' ')[0]}: command not found (demo shell)`)
  },
}
