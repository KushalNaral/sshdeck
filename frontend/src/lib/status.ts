import { versionGap } from './version'
import type { Mark, Server, Session, Vpn } from '../types'

export function markFor(server: Server, session?: Session): Mark {
  if (session?.state === 'live') return 'confirmed'
  if (session?.state === 'failed') return 'struck'
  return server.mark
}

export const vpnOf = (server: Server, vpns: Vpn[]) => vpns.find((v) => v.id === server.vpnId)

/** The VPN a server needs: its own, otherwise the nearest jump host's. */
export function vpnIdFor(server: Server, servers: Server[]): string {
  for (let s: Server | undefined = server, hops = 0; s && hops < 5; s = servers.find((x) => x.id === s!.jumpId), hops++) {
    if (s.vpnId) return s.vpnId
  }
  return ''
}

/** Other VPNs that have a session in progress or live. Two tunnels at once tend to fight over routes and DNS. */
export function vpnsInTheWay(server: Server, servers: Server[], vpns: Vpn[], sessions: Record<string, Session>) {
  const mine = vpnIdFor(server, servers)
  if (!mine) return []
  return vpns
    .filter((v) => v.id !== mine)
    .map((vpn) => ({
      vpn,
      servers: servers.filter((s) => vpnIdFor(s, servers) === vpn.id && (sessions[s.id]?.state === 'live' || sessions[s.id]?.state === 'connecting')),
    }))
    .filter((g) => g.servers.length)
}

/** How the server is reached, in words. */
export function viaLabel(server: Server, vpns: Vpn[]): string {
  const v = vpnOf(server, vpns)
  if (v) return v.name
  return server.vpnRequired ? 'VPN required, not set' : 'Direct network'
}

/** One plain sentence per server, never codes. */
export function sentence(server: Server, vpns: Vpn[], session?: Session): string {
  if (session?.state === 'live') return `Connected as ${server.user}@${server.host}`
  if (session?.state === 'connecting') return 'Connecting…'
  if (session?.state === 'failed') return session.error?.problem ?? 'Connection failed'
  const tracked = server.software.filter((x) => x.version && x.target)
  const behind = tracked.filter((x) => versionGap(x.version, x.target) !== 'current').length
  const php = !tracked.length ? 'No software tracked' : behind ? `${behind} of ${tracked.length} tracked behind` : 'Tracked software current'
  const n = server.projects.length
  const via = vpnOf(server, vpns) || server.vpnRequired ? ` · via ${viaLabel(server, vpns)}` : ''
  return `${php} · ${n} project${n === 1 ? '' : 's'}${via}`
}
