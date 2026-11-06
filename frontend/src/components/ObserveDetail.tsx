import { useEffect, useMemo, useState } from 'react'
import type { Backend } from '../api'
import { messageOf } from '../lib/config'
import { ago, fmtRate, RANGES, series, type RangeId } from '../lib/metrics'
import type { Triage } from '../lib/triage'
import type { Alert, Analysis, Proc, Sample, Server, Session } from '../types'
import { ContainerActions, ServiceRow, type Ask } from './Actions'
import { LineChart } from './Chart'
import { Tag } from './Tag'

interface Props {
  server: Server
  session?: Session
  level?: Triage
  analysis?: Analysis
  alerts: Alert[]
  backend: Backend
  ask: Ask
  connected: boolean
  onBack: () => void
  onTerminal: () => void
  onLog: (kind: 'journal' | 'container', arg: string) => void
  onRescan: () => void
}

const pct = (v: number) => `${Math.round(v)}%`

/** One server in detail: charts over a chosen range, the busiest processes, services and containers, and its alerts. */
export function ObserveDetail({ server, level, analysis, alerts, backend, ask, connected, onBack, onTerminal, onLog, onRescan }: Props) {
  const [range, setRange] = useState<RangeId>('1h')
  const [data, setData] = useState<Sample[]>([])
  const [procs, setProcs] = useState<Proc[] | null>(null)
  const [procErr, setProcErr] = useState('')
  const sec = RANGES.find((r) => r.id === range)!.sec

  useEffect(() => {
    let live = true
    const load = () => {
      const to = Date.now() / 1000
      void backend.observe.range(server.id, to - sec, to).then((d) => live && setData(d))
    }
    load()
    const t = setInterval(load, sec <= 3600 ? 5000 : 30000)
    return () => {
      live = false
      clearInterval(t)
    }
  }, [backend, server.id, sec])

  useEffect(() => {
    if (!connected) return setProcs(null)
    let live = true
    const load = () =>
      backend.observe.top(server.id).then(
        (p) => live && (setProcs(p), setProcErr('')),
        (e) => live && setProcErr(messageOf(e)),
      )
    void load()
    const t = setInterval(load, 5000)
    return () => {
      live = false
      clearInterval(t)
    }
  }, [backend, server.id, connected])

  const last = data[data.length - 1]
  const steal = useMemo(() => series(data, (s) => s.steal, true), [data])
  const showSteal = steal.some((p) => (p.v ?? 0) >= 0.5)
  const now = Date.now() / 1000
  const firing = alerts.filter((a) => !a.resolved)

  return (
    <section className="odetail" aria-label={`${server.name} in detail`}>
      <header className="observe-head">
        <button className="btn" onClick={onBack}>
          ← Fleet
        </button>
        {level && <Tag level={level.level} still />}
        <h2 className="name">{server.name}</h2>
        {server.production && <span className="tag-prod">production</span>}
        <span className="dim">{connected ? (last ? `${analysis?.os ?? ''} · ${last.cores} cores · ${(last.memTotalMb / 1024).toFixed(1)} GB` : 'waiting for readings') : 'Not connected'}</span>
        <span className="observe-filters">
          <button className="btn" disabled={!connected} onClick={onTerminal}>
            Terminal
          </button>
          <button className="btn" disabled={!connected} onClick={() => onLog('journal', '')}>
            System log
          </button>
          <button className="btn" disabled={!connected} onClick={onRescan}>
            Rescan
          </button>
        </span>
      </header>

      <div className="orange" role="group" aria-label="Time range">
        {RANGES.map((r) => (
          <button key={r.id} className="btn" aria-pressed={range === r.id} onClick={() => setRange(r.id)}>
            {r.label}
          </button>
        ))}
        {!connected && <span className="dim"> Showing what was recorded; nothing new arrives while disconnected.</span>}
      </div>

      <div className="ocharts">
        <LineChart title="CPU" max={100} floor={100} fmt={pct} lines={[{ name: 'Busy', slot: 1, pts: series(data, (s) => s.cpu, true) }, ...(showSteal ? [{ name: 'Stolen by host', slot: 2 as const, pts: steal }] : [])]} />
        <LineChart
          title="Load average"
          floor={Math.max(2, last?.cores ?? 2)}
          fmt={(v) => v.toFixed(1)}
          refLine={last?.cores ? { v: last.cores, label: `${last.cores} cores` } : undefined}
          lines={[{ name: '1 min', slot: 1, pts: series(data, (s) => s.load1) }, { name: '15 min', slot: 2, pts: series(data, (s) => s.load15) }]}
        />
        <LineChart title="Memory" max={100} floor={100} fmt={pct} lines={[{ name: 'Used', slot: 1, pts: series(data, (s) => s.memPct) }, { name: 'Swap', slot: 2, pts: series(data, (s) => s.swapPct) }]} />
        <LineChart title="Network" floor={10240} fmt={fmtRate} lines={[{ name: 'Received', slot: 1, pts: series(data, (s) => s.netRx, true) }, { name: 'Sent', slot: 2, pts: series(data, (s) => s.netTx, true) }]} />
        <LineChart title="Disk activity" floor={10240} fmt={fmtRate} lines={[{ name: 'Read', slot: 1, pts: series(data, (s) => s.diskR, true) }, { name: 'Write', slot: 2, pts: series(data, (s) => s.diskW, true) }]} />
        <div>
          <h3 className="label">Disk space</h3>
          {last?.disks?.length ? (
            last.disks.map((d) => (
              <div key={d.mount} className="dbar">
                <span>{d.mount}</span>
                <span className="dtrack" role="img" aria-label={`${d.mount} ${d.usedPct} percent used`}>
                  <i className={d.usedPct >= 90 ? 'bad' : ''} style={{ width: `${d.usedPct}%` }} />
                </span>
                <span className={d.usedPct >= 90 ? 'bad' : ''}>{d.usedPct}%</span>
              </div>
            ))
          ) : (
            <p className="dim">No disk readings.</p>
          )}
        </div>
      </div>

      <div className="ocols">
        <div className="sect">
          <h3 className="label">Busiest processes</h3>
          {!connected ? (
            <p className="dim">Connect to see live processes.</p>
          ) : procErr ? (
            <p className="bad">{procErr}</p>
          ) : !procs ? (
            <p className="dim">Reading…</p>
          ) : (
            <table>
              <thead>
                <tr>
                  <th>PID</th>
                  <th>User</th>
                  <th>CPU</th>
                  <th>Mem</th>
                  <th>Command</th>
                </tr>
              </thead>
              <tbody>
                {procs.map((p) => (
                  <tr key={p.pid}>
                    <td>{p.pid}</td>
                    <td>{p.user}</td>
                    <td>{p.cpu.toFixed(1)}%</td>
                    <td>{p.mem.toFixed(1)}%</td>
                    <td>{p.command}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>

        <div className="sect">
          <h3 className="label">Alerts</h3>
          {alerts.length === 0 ? (
            <p className="dim">Nothing has fired for this server.</p>
          ) : (
            <ul className="alist">
              {alerts.map((a) => (
                <li key={a.id + a.since} data-sev={a.severity} data-done={!!a.resolved}>
                  <b>{a.resolved ? 'Resolved' : a.severity === 'critical' ? 'Critical' : 'Warning'}</b> {a.text}{' '}
                  <span className="dim">{a.resolved ? `ended ${ago(a.resolved, now)}` : `since ${ago(a.since, now)}`}</span>
                </li>
              ))}
            </ul>
          )}
          {firing.length === 0 && alerts.length > 0 && <p className="good">Nothing firing now.</p>}
        </div>
      </div>

      {analysis && (
        <div className="ocols">
          <div className="sect">
            <h3 className="label">Failed services</h3>
            {analysis.failedUnits.length === 0 ? (
              <p className="good">None failed.</p>
            ) : (
              analysis.failedUnits.map((u) => <ServiceRow key={u} unit={u} server={server} connected={connected} backend={backend} ask={ask} onDone={onRescan} onLog={() => onLog('journal', u)} />)
            )}
          </div>
          <div className="sect">
            <h3 className="label">Containers</h3>
            {analysis.containers === null ? (
              <p className="dim">Docker is not installed here.</p>
            ) : (
              <table>
                <tbody>
                  {analysis.containers.map((c) => (
                    <tr key={c.name}>
                      <td className={c.status === 'running' ? '' : 'bad'}>{c.name}</td>
                      <td className="dim">{c.status}</td>
                      <td>
                        <button className="btn" disabled={!connected} onClick={() => onLog('container', c.name)}>
                          Log
                        </button>{' '}
                        <ContainerActions c={c} server={server} connected={connected} backend={backend} ask={ask} onDone={onRescan} />
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </div>
        </div>
      )}
    </section>
  )
}
