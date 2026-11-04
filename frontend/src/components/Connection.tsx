import { useEffect, useState } from 'react'
import { vpnOf } from '../lib/status'
import type { Server, Session, Vpn } from '../types'
import { VPN_LABEL } from '../types'
import { Mark } from './Mark'

const hash = (s: string) => [...s].reduce((a, c) => (a * 31 + c.charCodeAt(0)) >>> 0, 7)
const clock = (ms: number) => {
  const s = Math.floor(ms / 1000)
  return [Math.floor(s / 3600), Math.floor(s / 60) % 60, s % 60].map((n) => String(n).padStart(2, '0')).join(':')
}

/** The route in use: you, the VPN, the host. Tunnel address and latency are demo values until the Go side reports them. */
export function Connection({ server, vpns, session }: { server: Server; vpns: Vpn[]; session?: Session }) {
  const vpn = vpnOf(server, vpns)
  const live = session?.state === 'live'
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    if (!live) return
    const t = setInterval(() => setNow(Date.now()), 1000)
    return () => clearInterval(t)
  }, [live])
  const h = hash(server.id)

  return (
    <div className="sect" aria-label="Connection in use">
      <h3 className="label">Connection</h3>
      <dl className="cols">
        <dt>Route</dt>
        <dd className="route">
          <span>You</span>
          <span aria-hidden>→</span>
          {(vpn || server.vpnRequired) && (
            <>
              <span>{vpn?.name ?? 'VPN (not set)'}</span>
              <span aria-hidden>→</span>
            </>
          )}
          <span>{server.name}</span>
        </dd>
        <dt>VPN</dt>
        <dd>
          {vpn ? (
            <>
              <Mark kind={live ? 'confirmed' : 'hollow'} />
              <span>
                {VPN_LABEL[vpn.kind]} · {vpn.gateway}:{vpn.port} · {vpn.user}
              </span>
            </>
          ) : (
            <span className={server.vpnRequired ? 'bad' : 'dim'}>{server.vpnRequired ? 'Required, but no profile is set' : 'None, direct network'}</span>
          )}
        </dd>
        {vpn && live && (
          <>
            <dt>Tunnel</dt>
            <dd>address 10.212.{h % 200}.{(h >> 3) % 250}</dd>
          </>
        )}
        <dt>SSH</dt>
        <dd>
          <Mark kind={live ? 'confirmed' : 'hollow'} />
          <span>
            {server.user}@{server.host}:{server.port} · {server.auth === 'key' ? 'key' : 'password'}
          </span>
        </dd>
        {live && session?.since && (
          <>
            <dt>Latency</dt>
            <dd>{18 + (h % 60)} ms</dd>
            <dt>Session</dt>
            <dd>{clock(now - session.since)}</dd>
          </>
        )}
        {!live && (
          <>
            <dt>State</dt>
            <dd className="dim">Not connected</dd>
          </>
        )}
      </dl>
    </div>
  )
}
