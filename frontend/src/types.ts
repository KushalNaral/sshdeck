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
  /** The same certificate as a pin-sha256 of its public key (openconnect). Filled in with trustedCert. */
  trustedPin?: string
  /** You connect this VPN yourself with its own app; sshdeck waits for you to confirm. */
  manual?: boolean
  /** GlobalProtect: where the login goes. Empty or 'portal' is the default; 'gateway' skips the portal. */
  loginAt?: 'portal' | 'gateway'
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
  /** Production server: the app asks before connecting and before a root container shell. */
  production?: boolean
  /** Connect when sshdeck starts. */
  autoConnect?: boolean
  os: string
  software: Software[]
  logins?: Login[]
  projects: string[]
  mark: Exclude<Mark, 'confirmed'>
  note?: string
}

export type LoginKind = 'db' | 'web' | 'cpanel' | 'rdp' | 'other'
export const LOGIN_KIND: Record<LoginKind, string> = { db: 'Database', web: 'Web app', cpanel: 'cPanel', rdp: 'RDP', other: 'Other' }

/** Another credential on a server. Its secret is write-only: it never comes back to the UI, only to the clipboard. */
export interface Login {
  id: string
  label: string
  kind: LoginKind
  user?: string
  address?: string
  note?: string
}

/** Software tracked on a server: what it runs now and the version it should reach. */
export interface Software {
  name: string
  version: string
  target: string
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
  /** Versions found on the server by the scan. */
  software: { name: string; version: string }[]
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

/** A dropped connection being redialled by the Go side. */
export interface Link {
  state: 'reconnecting' | 'live' | 'lost'
  attempt?: number
  max?: number
  problem?: string
  recovery?: string
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
  error?: { problem: string; recovery: string; details?: string }
  link?: Link
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
