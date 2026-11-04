import { phpGap } from './php'
import type { Mark, Server, Session, Vpn } from '../types'

export function markFor(server: Server, session?: Session): Mark {
  if (session?.state === 'live') return 'confirmed'
  if (session?.state === 'failed') return 'struck'
  return server.mark
}

export const vpnOf = (server: Server, vpns: Vpn[]) => vpns.find((v) => v.id === server.vpnId)

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
  const gap = phpGap(server.php, server.phpTarget)
  const php = !server.php ? 'PHP version unknown' : gap === 'current' ? `PHP ${server.php} is current` : `PHP ${server.php} → ${server.phpTarget}`
  const n = server.projects.length
  const via = vpnOf(server, vpns) || server.vpnRequired ? ` · via ${viaLabel(server, vpns)}` : ''
  return `${php} · ${n} project${n === 1 ? '' : 's'}${via}`
}
