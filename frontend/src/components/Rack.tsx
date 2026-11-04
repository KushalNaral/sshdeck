import { useCallback, useEffect, useRef, useState } from 'react'
import { scannedLabel, type Triage } from '../lib/triage'
import { markFor } from '../lib/status'
import type { Analysis, Server, Session, Vpn } from '../types'
import { Mark } from './Mark'
import { Tag } from './Tag'

interface Props {
  servers: Server[]
  vpns: Vpn[]
  sessions: Record<string, Session>
  analysis: Record<string, Analysis>
  levels: Record<string, Triage>
  now: number
  selected: string
  onSelect: (id: string) => void
  onConnect: (id: string) => void
}

/** VPN gates as ruled boards. Inside each gate the worst server comes first; the strip on top names the first and last in view. */
export function Rack({ servers, vpns, sessions, analysis, levels, now, selected, onSelect, onConnect }: Props) {
  const nav = useRef<HTMLElement>(null)
  const [span, setSpan] = useState<[string, string] | null>(null)

  const measure = useCallback(() => {
    const root = nav.current
    if (!root) return
    const box = root.getBoundingClientRect()
    const seen = [...root.querySelectorAll<HTMLElement>('[data-server]')].filter((el) => {
      const r = el.getBoundingClientRect()
      return r.bottom > box.top + 24 && r.top < box.bottom
    })
    setSpan(seen.length ? [seen[0].dataset.server!, seen[seen.length - 1].dataset.server!] : null)
  }, [])
  useEffect(() => {
    measure()
    const ro = new ResizeObserver(measure)
    if (nav.current) ro.observe(nav.current)
    return () => ro.disconnect()
  }, [measure, servers, levels])

  const rank = (a: Server, b: Server) => (levels[a.id]?.level ?? 5) - (levels[b.id]?.level ?? 5) || a.name.localeCompare(b.name)

  return (
    <nav className="rack" aria-label="Servers by VPN, worst level first" ref={nav} onScroll={measure}>
      <div className="guide" aria-hidden>
        {span ? (span[0] === span[1] ? span[0] : `${span[0]} to ${span[1]}`) : ''}
      </div>
      {[
        ...vpns.map((v) => ({ key: v.id, title: v.name, vpn: v, list: servers.filter((s) => s.vpnId === v.id) })),
        { key: 'direct', title: 'Direct network', vpn: undefined, list: servers.filter((s) => !s.vpnId && !s.vpnRequired) },
        { key: 'req', title: 'VPN required, not set', vpn: undefined, list: servers.filter((s) => !s.vpnId && s.vpnRequired) },
      ]
        .filter((g) => g.list.length)
        .map(({ key, title, vpn, list }) => {
          const up = list.some((s) => sessions[s.id]?.state === 'live')
          return (
            <section key={key} className="rack-group" aria-label={title}>
              <div className="slide head">
                <div className="slide-in label">
                  <span>{title}</span>
                  <span style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
                    {vpn && (
                      <>
                        <Mark kind={up ? 'confirmed' : 'hollow'} />
                        {up ? 'up' : 'idle'}
                      </>
                    )}
                    <span>{list.length}</span>
                  </span>
                </div>
              </div>
              {[...list].sort(rank).map((s) => {
                const sess = sessions[s.id]
                const live = sess?.state === 'live'
                const tri = levels[s.id]
                const line = tri?.findings[0]?.text ?? ''
                const scanned = analysis[s.id]?.scannedAt
                return (
                  <div
                    key={s.id}
                    className="slide"
                    data-server={s.name}
                    title={live ? `${line}. Connected as ${s.user}@${s.host}` : line}
                    role="button"
                    tabIndex={0}
                    aria-current={selected === s.id}
                    onClick={() => onSelect(s.id)}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter' || e.key === ' ') {
                        e.preventDefault()
                        onSelect(s.id)
                        if (e.key === 'Enter' && !sess) onConnect(s.id)
                      }
                    }}
                  >
                    <div className="slide-in">
                      <div className="row1">
                        {tri && <Tag level={tri.level} />}
                        <span className="name trunc">{s.name}</span>
                        {s.production && (
                          <span className="prod" title="Production server">
                            PROD
                          </span>
                        )}
                        <Mark kind={markFor(s, sess)} />
                        {!live && sess?.state !== 'connecting' && (
                          <button
                            className="btn"
                            onClick={(e) => {
                              e.stopPropagation()
                              onSelect(s.id)
                              onConnect(s.id)
                            }}
                          >
                            Connect
                          </button>
                        )}
                      </div>
                      <div className="row2">
                        <span className="trunc">{line}</span>
                        {(scanned || selected === s.id) && <span>{scannedLabel(scanned, now)}</span>}
                      </div>
                    </div>
                  </div>
                )
              })}
            </section>
          )
        })}
    </nav>
  )
}
