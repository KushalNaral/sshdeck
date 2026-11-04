import * as App from '../../wailsjs/go/main/App'
import { SERVERS, VPNS } from '../data'
import type { Server, Vpn } from '../types'

export interface Config {
  servers: Server[]
  vpns: Vpn[]
}

/** Where servers and VPN profiles live. Passwords only ever go one way: into save*, never back out. */
export interface ConfigStore {
  load(): Promise<Config>
  saveServer(s: Server, password: string): Promise<Server>
  deleteServer(id: string): Promise<void>
  saveVpn(v: Vpn, password: string): Promise<Vpn>
  deleteVpn(id: string): Promise<void>
}

/** Wails rejects with a plain string; make every failure a readable message. */
export const messageOf = (e: unknown) => (e instanceof Error ? e.message : String(e))

// Go has no `mark` and uses "" for direct; the UI uses null, and derives the mark from how the server is reached.
const fromGoServer = (g: any): Server => ({
  ...g,
  vpnId: g.vpnId || null,
  os: g.os ?? '',
  php: g.php ?? '',
  phpTarget: g.phpTarget ?? '',
  webserver: g.webserver ?? '',
  projects: g.projects ?? [],
  mark: g.vpnId || g.vpnRequired ? 'hollow' : 'filled',
})
const toGoServer = ({ mark: _mark, ...s }: Server) => ({ ...s, vpnId: s.vpnId ?? '' }) as any

const wailsStore: ConfigStore = {
  async load() {
    const [servers, vpns] = await Promise.all([App.ListServers(), App.ListVPNs()])
    return { servers: servers.map(fromGoServer), vpns: vpns as Vpn[] }
  },
  async saveServer(s, pw) {
    return fromGoServer(await App.SaveServer(toGoServer(s), pw))
  },
  deleteServer: (id) => App.DeleteServer(id),
  async saveVpn(v, pw) {
    return (await App.SaveVPN(v as any, pw)) as Vpn
  },
  deleteVpn: (id) => App.DeleteVPN(id),
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
        if (Array.isArray(c?.servers) && Array.isArray(c?.vpns)) return (cfg = c)
      } catch {
        /* fall through to demo data */
      }
      return (cfg = { servers: SERVERS, vpns: VPNS })
    },
    async saveServer(s) {
      s = { ...s, id: s.id || `s-${Date.now().toString(36)}` }
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
  }
})()

export const configStore: ConfigStore = 'go' in window ? wailsStore : localStore
