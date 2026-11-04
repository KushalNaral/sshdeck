import { markFor, sentence } from '../lib/status'
import type { Server, Session, Task, Vpn } from '../types'
import { Mark } from './Mark'

interface Props {
  servers: Server[]
  vpns: Vpn[]
  sessions: Record<string, Session>
  tasks: Task[]
  selected: string
  onSelect: (id: string) => void
  onConnect: (id: string) => void
}

/** VPN groups as raked slides: each server is a slide, groups are headed by their gate. */
export function Rack({ servers, vpns, sessions, tasks, selected, onSelect, onConnect }: Props) {
  return (
    <nav className="rack" aria-label="Servers by VPN">
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
            {list.map((s) => {
              const sess = sessions[s.id]
              const live = sess?.state === 'live'
              return (
                <div
                  key={s.id}
                  className="slide"
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
                      <Mark kind={markFor(s, sess)} />
                      <span className="name trunc">{s.name}</span>
                      {s.production && <span className="prod" title="Production server">PROD</span>}
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
                      <span className="trunc">{sentence(s, vpns, sess)}</span>
                      {(() => {
                        const n = tasks.filter((t) => t.status !== 'done' && t.serverIds.includes(s.id)).length
                        return n > 0 && <span title="Open tasks">{n} task{n === 1 ? '' : 's'}</span>
                      })()}
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
