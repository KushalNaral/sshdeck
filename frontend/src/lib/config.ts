import * as App from '../../wailsjs/go/main/App'
import { SERVERS, VPNS } from '../data'
import type { Login, Server, Task, Vpn } from '../types'

export interface Config {
  servers: Server[]
  vpns: Vpn[]
  tasks: Task[]
}

/** Where servers and VPN profiles live. Passwords only ever go one way: into save*, never back out. */
export interface ConfigStore {
  load(): Promise<Config>
  saveServer(s: Server, password: string): Promise<Server>
  deleteServer(id: string): Promise<void>
  saveVpn(v: Vpn, password: string): Promise<Vpn>
  deleteVpn(id: string): Promise<void>
  saveTask(t: Task): Promise<Task>
  deleteTask(id: string): Promise<void>
  saveLogin(serverId: string, l: Login, secret: string): Promise<Login>
  deleteLogin(serverId: string, id: string): Promise<void>
  /** Puts the user name or the secret on the clipboard (the secret clears itself). */
  copyLogin(serverId: string, id: string, field: 'user' | 'secret'): Promise<void>
  /** Puts `ssh -p PORT user@host` on the clipboard. It holds no secret. */
  copySSH(serverId: string): Promise<void>
}

/** Wails rejects with a plain string; make every failure a readable message. */
export const messageOf = (e: unknown) => (e instanceof Error ? e.message : String(e))

// Go has no `mark` and uses "" for direct; the UI uses null, and derives the mark from how the server is reached.
const fromGoServer = (g: any): Server => ({
  ...g,
  vpnId: g.vpnId || null,
  os: g.os ?? '',
  logins: g.logins ?? [],
  software: (g.software ?? []).map((x: any) => ({ name: x.name, version: x.version ?? '', target: x.target ?? '' })),
  projects: g.projects ?? [],
  mark: g.vpnId || g.vpnRequired ? 'hollow' : 'filled',
})
const toGoServer = ({ mark: _mark, ...s }: Server) => ({ ...s, vpnId: s.vpnId ?? '' }) as any

const wailsStore: ConfigStore = {
  async load() {
    const [servers, vpns, tasks] = await Promise.all([App.ListServers(), App.ListVPNs(), App.ListTasks()])
    return { servers: servers.map(fromGoServer), vpns: vpns as Vpn[], tasks: tasks as Task[] }
  },
  async saveServer(s, pw) {
    return fromGoServer(await App.SaveServer(toGoServer(s), pw))
  },
  deleteServer: (id) => App.DeleteServer(id),
  async saveVpn(v, pw) {
    return (await App.SaveVPN(v as any, pw)) as Vpn
  },
  deleteVpn: (id) => App.DeleteVPN(id),
  async saveTask(t) {
    return (await App.SaveTask(t as any)) as Task
  },
  deleteTask: (id) => App.DeleteTask(id),
  async saveLogin(sid, l, pw) {
    return (await App.SaveLogin(sid, l as any, pw)) as Login
  },
  deleteLogin: (sid, id) => App.DeleteLogin(sid, id),
  copyLogin: (sid, id, field) => App.CopyLogin(sid, id, field),
  copySSH: (sid) => App.CopySSHCommand(sid),
}

// Browser-only dev mode: demo data kept in localStorage, passwords discarded.
const KEY = 'sshdeck.config'
const localStore: ConfigStore = (() => {
  let cfg: Config | undefined
  const persist = () => {
    try {
      localStorage.setItem(KEY, JSON.stringify(cfg))
    } catch {
      /* storage unavailable: edits last for this run */
    }
  }
  const upsert = <T extends { id: string }>(list: T[], x: T) => (list.some((y) => y.id === x.id) ? list.map((y) => (y.id === x.id ? x : y)) : [...list, x])
  return {
    async load() {
      try {
        const c = JSON.parse(localStorage.getItem(KEY) ?? 'null')
        if (Array.isArray(c?.servers) && Array.isArray(c?.vpns)) return (cfg = { tasks: [], ...c, servers: c.servers.map((s: any) => ({ ...s, software: s.software ?? [] })) }) // older saves had no software
      } catch {
        /* fall through to demo data */
      }
      return (cfg = { servers: SERVERS, vpns: VPNS, tasks: [] })
    },
    async saveServer(s) {
      s = { ...s, id: s.id || `s-${Date.now().toString(36)}`, logins: cfg!.servers.find((x) => x.id === s.id)?.logins }
      cfg = { ...cfg!, servers: upsert(cfg!.servers, s) }
      persist()
      return s
    },
    async deleteServer(id) {
      cfg = { ...cfg!, servers: cfg!.servers.filter((x) => x.id !== id) }
      persist()
    },
    async saveVpn(v) {
      v = { ...v, id: v.id || `v-${Date.now().toString(36)}` }
      cfg = { ...cfg!, vpns: upsert(cfg!.vpns, v) }
      persist()
      return v
    },
    async deleteVpn(id) {
      cfg = { ...cfg!, vpns: cfg!.vpns.filter((x) => x.id !== id) }
      persist()
    },
    async saveTask(t) {
      const now = Date.now()
      t = { ...t, id: t.id || `t-${now.toString(36)}`, created: t.created || now, updated: now }
      cfg = { ...cfg!, tasks: upsert(cfg!.tasks, t) }
      persist()
      return t
    },
    async deleteTask(id) {
      cfg = { ...cfg!, tasks: cfg!.tasks.filter((x) => x.id !== id) }
      persist()
    },
    async saveLogin(sid, l) {
      l = { ...l, id: l.id || `l-${Date.now().toString(36)}` }
      cfg = { ...cfg!, servers: cfg!.servers.map((x) => (x.id === sid ? { ...x, logins: upsert(x.logins ?? [], l) } : x)) }
      persist()
      return l
    },
    async deleteLogin(sid, id) {
      cfg = { ...cfg!, servers: cfg!.servers.map((x) => (x.id === sid ? { ...x, logins: (x.logins ?? []).filter((y) => y.id !== id) } : x)) }
      persist()
    },
    async copySSH(sid) {
      const x = cfg!.servers.find((y) => y.id === sid)
      if (x) await navigator.clipboard?.writeText(`ssh${x.port === 22 ? '' : ` -p ${x.port}`} ${x.user}@${x.host}`)
    },
    async copyLogin(sid, id, field) {
      if (field === 'secret') throw new Error('This browser demo keeps no passwords.')
      const l = cfg!.servers.find((x) => x.id === sid)?.logins?.find((y) => y.id === id)
      await navigator.clipboard?.writeText(l?.user ?? '')
    },
  }
})()

export const configStore: ConfigStore = 'go' in window ? wailsStore : localStore
