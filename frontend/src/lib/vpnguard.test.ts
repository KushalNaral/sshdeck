import { describe, expect, it } from 'vitest'
import type { Server, Session, Vpn } from '../types'
import { vpnsInTheWay } from './status'

const vpns = [{ id: 'f', name: 'Office' }, { id: 'g', name: 'Client' }] as Vpn[]
const sv = (id: string, vpnId = '') => ({ id, name: id.toUpperCase(), vpnId }) as unknown as Server
const servers = [sv('a', 'f'), sv('b', 'f'), sv('c', 'g'), sv('d')]
const on = (...ids: string[]) =>
  Object.fromEntries(ids.map((id) => [id, { serverId: id, state: 'live' } as Session])) as Record<string, Session>

describe('vpnsInTheWay', () => {
  it('lists another VPN that is already in use', () => {
    const got = vpnsInTheWay(servers[2], servers, vpns, on('a', 'b'))
    expect(got).toHaveLength(1)
    expect(got[0].vpn.name).toBe('Office')
    expect(got[0].servers.map((s) => s.name)).toEqual(['A', 'B'])
  })
  it('ignores the VPN the server itself uses', () => {
    expect(vpnsInTheWay(servers[1], servers, vpns, on('a'))).toEqual([])
  })
  it('does not bother direct servers', () => {
    expect(vpnsInTheWay(servers[3], servers, vpns, on('a', 'c'))).toEqual([])
  })
  it('follows a jump host to its VPN', () => {
    const inner = { ...sv('e'), jumpId: 'c' } as Server // no VPN of its own, jumps through C on the Client VPN
    const all = [...servers, inner]
    expect(vpnsInTheWay(inner, all, vpns, on('a'))[0].vpn.name).toBe('Office')
    expect(vpnsInTheWay(servers[0], all, vpns, on('e'))[0].vpn.name).toBe('Client')
    expect(vpnsInTheWay(inner, all, vpns, on('c'))).toEqual([])
  })
  it('counts a connect in flight but not a failed one', () => {
    const s = { a: { serverId: 'a', state: 'connecting' }, b: { serverId: 'b', state: 'failed' } } as unknown as Record<string, Session>
    expect(vpnsInTheWay(servers[2], servers, vpns, s)[0].servers.map((x) => x.id)).toEqual(['a'])
  })
})
