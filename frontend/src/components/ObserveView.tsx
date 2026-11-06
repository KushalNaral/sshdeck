import { useEffect, useMemo, useState } from 'react'
import type { Backend } from '../api'
import { freshness, fmtRate, series, worstDisk, ago } from '../lib/metrics'
import { useSamples } from '../lib/observe'
import type { Triage } from '../lib/triage'
import type { Alert, Analysis, Server, Session, Vpn } from '../types'
import type { Ask } from './Actions'
import { Spark } from './Chart'
import { ObserveDetail } from './ObserveDetail'
import { Tag } from './Tag'

interface Props {
  servers: Server[]
  vpns: Vpn[]
  sessions: Record<string, Session>
  levels: Record<string, Triage>
  analysis: Record<string, Analysis>
  alerts: Alert[]
  backend: Backend
  ask: Ask
  onConnect: (id: string) => Promise<void>
  onTerminal: (id: string) => void
  onLog: (id: string, kind: 'journal' | 'container', arg: string) => void
  onRescan: (id: string) => void
  /** Open this server's detail first, for a jump from an alert elsewhere. */
  open?: string
}

const pct = (n: number) => `${Math.round(n)}%`

/** Every server at a glance: live numbers for the ones that are connected, a Connect button for the rest. */
export function ObserveView({ servers, vpns, sessions, levels, analysis, alerts, backend, ask, onConnect, onTerminal, onLog, onRescan, open }: Props) {
  const { latest, rings } = useSamples(backend)
  const [detail, setDetail] = useState<string | null>(open ?? null)
  const [project, setProject] = useState('')
  const [vpn, setVpn] = useState('')
  const [onlyAlerts, setOnlyAlerts] = useState(false)
  const [now, setNow] = useState(Date.now() / 1000)
  useEffect(() => {
    const t = setInterval(() => setNow(Date.now() / 1000), 5000)
    return () => clearInterval(t)
  }, [])
  useEffect(() => void (open && setDetail(open)), [open])

  const live = (id: string) => sessions[id]?.state === 'live'
  const firing = alerts.filter((a) => !a.resolved)
  const firingBy = (id: string) => firing.filter((a) => a.server === id)
  const projects = useMemo(() => [...new Set(servers.flatMap((s) => s.projects ?? []))].sort(), [servers])
  const shown = useMemo(() => {
    const keep = servers.filter((s) => (!project || s.projects?.includes(project)) && (!vpn || (vpn === '-' ? !s.vpnId : s.vpnId === vpn)) && (!onlyAlerts || firing.some((a) => a.server === s.id)))
    return keep.sort(
      (a, b) =>
        Number(live(b.id)) - Number(live(a.id)) ||
        (levels[a.id]?.level ?? 5) - (levels[b.id]?.level ?? 5) ||
        firing.filter((x) => x.server === b.id).length - firing.filter((x) => x.server === a.id).length ||
        a.name.localeCompare(b.name),
    )
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [servers, project, vpn, onlyAlerts, sessions, levels, alerts])
  const name = (id: string) => servers.find((s) => s.id === id)?.name ?? id

  const server = servers.find((s) => s.id === detail)
  if (server) {
    return (
      <ObserveDetail
        server={server}
        session={sessions[server.id]}
        level={levels[server.id]}
        analysis={analysis[server.id]}
        alerts={alerts.filter((a) => a.server === server.id)}
        backend={backend}
        ask={ask}
        connected={live(server.id)}
        onBack={() => setDetail(null)}
        onTerminal={() => onTerminal(server.id)}
        onLog={(k, a) => onLog(server.id, k, a)}
        onRescan={() => onRescan(server.id)}
      />
    )
  }

  const nLive = servers.filter((s) => live(s.id)).length
  return (
    <section className="observe" aria-label="Observe">
      <header className="observe-head">
        <h2 className="name">Fleet</h2>
        <span className="dim">
          {nLive} of {servers.length} connected · {firing.length ? `${firing.length} alert${firing.length === 1 ? '' : 's'} firing` : 'nothing firing'}
        </span>
        <span className="observe-filters">
          <label>
            <span className="sr">Project</span>
            <select value={project} onChange={(e) => setProject(e.target.value)} aria-label="Project">
              <option value="">All projects</option>
              {projects.map((p) => (
                <option key={p}>{p}</option>
              ))}
            </select>
          </label>
          <label>
            <span className="sr">VPN</span>
            <select value={vpn} onChange={(e) => setVpn(e.target.value)} aria-label="VPN">
              <option value="">Any VPN</option>
              <option value="-">No VPN</option>
              {vpns.map((v) => (
                <option key={v.id} value={v.id}>
                  {v.name}
                </option>
              ))}
            </select>
          </label>
          <label className="check">
            <input type="checkbox" checked={onlyAlerts} onChange={(e) => setOnlyAlerts(e.target.checked)} />
            <span>Only with alerts</span>
          </label>
        </span>
      </header>

      {firing.length > 0 && (
        <ul className="alert-strip" aria-label="Firing alerts">
          {firing.map((a) => (
            <li key={a.id}>
              <button className="chip" data-sev={a.severity} onClick={() => setDetail(a.server)}>
                <b>{name(a.server)}</b> {a.text} <span className="dim">{ago(a.since, now)}</span>
              </button>
            </li>
          ))}
        </ul>
      )}

      <div className="ocards">
        {shown.map((s) => {
          const smp = latest[s.id]
          const ring = rings[s.id] ?? []
          const on = live(s.id)
          const fresh = freshness(smp, now)
          const disk = worstDisk(smp)
          const mine = firingBy(s.id)
          const lvl = levels[s.id]
          return (
            <article key={s.id} className="ocard" data-live={on} data-stale={on && fresh !== 'live'} aria-label={s.name}>
              <header>
                {lvl && <Tag level={lvl.level} still />}
                <button className="olink" onClick={() => setDetail(s.id)} title={`Open ${s.name} in detail`}>
                  {s.name}
                </button>
                {s.production && <span className="tag-prod">production</span>}
              </header>
              {on && smp ? (
                <>
                  <dl className="ometrics">
                    <div>
                      <dt>CPU</dt>
                      <dd>{smp.hasRates ? pct(smp.cpu) : '–'}</dd>
                      <Spark pts={series(ring, (x) => x.cpu, true)} label={`${s.name} CPU`} />
                    </div>
                    <div>
                      <dt>Memory</dt>
                      <dd>{pct(smp.memPct)}</dd>
                      <Spark pts={series(ring, (x) => x.memPct)} label={`${s.name} memory`} />
                    </div>
                  </dl>
                  <p className="ofoot">
                    <span className={disk && disk.usedPct >= 90 ? 'bad' : ''}>{disk ? `Disk ${disk.mount} ${disk.usedPct}%` : 'No disk data'}</span>
                    <span>{smp.hasRates ? `↓ ${fmtRate(smp.netRx)}  ↑ ${fmtRate(smp.netTx)}` : ''}</span>
                  </p>
                </>
              ) : on ? (
                <p className="dim ofoot">Waiting for the first reading…</p>
              ) : (
                <div className="ooff">
                  <span className="dim">Not connected</span>
                  <button className="btn" onClick={() => void onConnect(s.id)}>
                    Connect
                  </button>
                </div>
              )}
              {mine.length > 0 && (
                <ul className="ochips">
                  {mine.slice(0, 2).map((a) => (
                    <li key={a.id} data-sev={a.severity}>
                      {a.text}
                    </li>
                  ))}
                  {mine.length > 2 && <li className="dim">+{mine.length - 2} more</li>}
                </ul>
              )}
            </article>
          )
        })}
        {shown.length === 0 && <p className="dim">No servers match.</p>}
      </div>
    </section>
  )
}
