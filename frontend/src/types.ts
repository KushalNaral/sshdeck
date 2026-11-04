export type VpnKind = 'forti' | 'gp' | 'hillstone'

/** A VPN profile. The password lives in the OS keyring, never here. */
export interface Vpn {
  id: string
  name: string
  kind: VpnKind
  gateway: string
  port: number
  user: string
  note?: string
  /** sha256 of a gateway certificate to accept (FortiClient gateways with a self-signed certificate). */
  trustedCert?: string
}

// One cell of the fixed-cell state mark: filled holds, hollow does not, struck is a problem, confirmed is live.
export type Mark = 'filled' | 'hollow' | 'struck' | 'confirmed'

export interface Server {
  id: string
  name: string
  host: string
  port: number
  user: string
  auth: 'password' | 'key'
  /** Key login: the private key file; empty means use ssh-agent. The passphrase is stored in the keyring like a password. */
  keyPath?: string
  /** Profile used to reach this server; null means direct, unless vpnRequired says one is needed but not set. */
  vpnId: string | null
  vpnRequired?: boolean
  os: string
  php: string
  phpTarget: string
  webserver: string
  projects: string[]
  /** Patch journey: 0 none, 1 found, 2 scheduled, 3 patched, 4 verified, 5 signed off */
  stage: number
  mark: Exclude<Mark, 'confirmed'>
  note?: string
}

export interface Container {
  name: string
  image: string
  status: 'running' | 'exited' | 'restarting'
  ports: string
}

export interface Analysis {
  os: string
  kernel: string
  uptime: string
  php: string
  webserver: string
  db: string
  cpu: number
  memUsedMb: number
  memTotalMb: number
  disks: { mount: string; usedPct: number }[]
  failedUnits: string[]
  containers: Container[] | null
  scannedAt: number
}

export type StepKey = 'vpn' | 'ssh' | 'shell'
export type StepStatus = 'wait' | 'run' | 'manual' | 'done' | 'fail'

export interface Step {
  key: StepKey
  label: string
  status: StepStatus
  ms?: number
  detail?: string
}

export type SessionState = 'connecting' | 'live' | 'failed'

/** A tab that runs inside a container instead of the server's own shell. */
export interface ContainerTab {
  container: string
  /** '' is the container's default user; 'root' is the admin user. */
  user: string
  sudo: boolean
}

export interface Dep {
  name: string
  purpose: string
  installed: boolean
  installable: boolean
}

export interface Session {
  serverId: string
  state: SessionState
  steps: Step[]
  tabs: number[]
  activeTab: number
  nextTab: number
  tabInfo?: Record<number, ContainerTab>
  since?: number
  error?: { problem: string; recovery: string }
}

export type Mode = 'firefight' | 'tasks' | 'admin'

export type TaskStatus = 'todo' | 'doing' | 'done'

/** Work to do on one or more servers. Due is YYYY-MM-DD or empty. */
export interface Task {
  id: string
  title: string
  note?: string
  status: TaskStatus
  tags: string[]
  serverIds: string[]
  due?: string
  created: number
  updated: number
}

export const VPN_LABEL: Record<VpnKind, string> = {
  forti: 'FortiClient',
  gp: 'GlobalProtect',
  hillstone: 'Hillstone (manual)',
}
