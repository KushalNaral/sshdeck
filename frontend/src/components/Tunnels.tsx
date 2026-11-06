import { useEffect, useState } from 'react'
import type { Backend } from '../api'
import type { Server, TunnelState } from '../types'
import { messageOf } from '../lib/config'

/** The server's saved tunnels, with a switch for each. Shown once the server is connected. */
export function Tunnels({ server, live, backend }: { server: Server; live: boolean; backend: Backend }) {
  const [states, setStates] = useState<Record<string, TunnelState>>({})
  const [err, setErr] = useState<string | null>(null)
  const list = server.tunnels ?? []
  const id = server.id
  useEffect(() => {
    setErr(null)
    if (!live || !list.length) return setStates({})
    const apply = (t: TunnelState[]) => setStates(Object.fromEntries(t.map((x) => [x.id, x])))
    void backend.tunnels.list(id).then(apply)
    return backend.tunnels.onChange(id, apply)
  }, [id, live, list.length, backend])
  if (!list.length) return null

  const toggle = async (tid: string, running: boolean) => {
    setErr(null)
    try {
      await (running ? backend.tunnels.stop(id, tid) : backend.tunnels.start(id, tid))
    } catch (e) {
      setErr(messageOf(e))
    }
  }
  return (
    <div className="sect" aria-label="Tunnels">
      <h3 className="label">Tunnels</h3>
      <ul className="tunnels">
        {list.map((t) => {
          const s = states[t.id]
          const on = s?.state === 'running'
          return (
            <li key={t.id}>
              <div style={{ minWidth: 0 }}>
                <strong>{t.name}</strong>
                {on && <span className="muted"> · {s.conns} open</span>}
                <div className="muted addr">
                  {t.kind === 'local' ? `${t.listen} → ${t.target}` : `server ${t.listen} → ${t.target}`}
                </div>
                {on && s.error && <div className="bad">{s.error}</div>}
              </div>
              <div style={{ display: 'flex', gap: 6 }}>
                {on && t.kind === 'local' && (
                  <button className="btn" type="button" onClick={() => navigator.clipboard?.writeText(t.listen).catch(() => {})} title="Copy the address to connect to">
                    Copy
                  </button>
                )}
                <button className="btn" type="button" disabled={!live} aria-pressed={on} onClick={() => void toggle(t.id, on)} title={live ? '' : 'Connect to the server first'}>
                  {on ? 'Stop' : 'Start'}
                </button>
              </div>
            </li>
          )
        })}
      </ul>
      {err && <p className="bad" role="alert">{err}</p>}
    </div>
  )
}
