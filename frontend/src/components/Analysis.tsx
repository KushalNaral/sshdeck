import { RefreshCw } from 'lucide-react'
import type { ReactNode } from 'react'
import { phpGap } from '../lib/php'
import type { Analysis as A, Server } from '../types'
import { Mark } from './Mark'

function Bar({ pct }: { pct: number }) {
  const on = Math.round(pct / 10)
  return (
    <span className={`cells ${pct >= 90 ? 'bad' : ''}`} role="img" aria-label={`${pct} percent used`}>
      {Array.from({ length: 10 }, (_, i) => (
        <i key={i} className={i < on ? 'on' : ''} />
      ))}
    </span>
  )
}

interface Props {
  server: Server
  data?: A
  scanning: boolean
  onRescan: () => void
  connection?: ReactNode
}

/** Read-only picture of the box, laid out like departure columns. */
export function Analysis({ server, data, scanning, onRescan, connection }: Props) {
  const gap = phpGap(server.php, server.phpTarget)
  return (
    <aside className="pane analysis" aria-label={`Analysis of ${server.name}`}>
      <div className="sect" style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <h2 className="name">{server.name}</h2>
        <button className="btn" onClick={onRescan} disabled={scanning} style={{ color: 'var(--ink)' }}>
          <RefreshCw size={14} strokeWidth={2} />
          {scanning ? 'Scanning' : 'Rescan'}
        </button>
      </div>

      {connection}

      {!data ? (
        <div className="sect dim">{scanning ? 'Reading the server: OS, PHP, disks, services, containers.' : 'Not scanned yet.'}</div>
      ) : (
        <>
          <div className="sect">
            <h3 className="label">System</h3>
            <dl className="cols">
              <dt>OS</dt>
              <dd>{data.os}</dd>
              <dt>Kernel</dt>
              <dd>{data.kernel}</dd>
              <dt>Uptime</dt>
              <dd>{data.uptime}</dd>
              <dt>CPU</dt>
              <dd>{data.cpu} cores</dd>
              <dt>Memory</dt>
              <dd>
                <Bar pct={Math.round((data.memUsedMb / data.memTotalMb) * 100)} />
                {Math.round((data.memUsedMb / data.memTotalMb) * 100)}%
              </dd>
              {data.disks.map((d) => (
                <div key={d.mount} style={{ display: 'contents' }}>
                  <dt>Disk {d.mount}</dt>
                  <dd>
                    <Bar pct={d.usedPct} />
                    <span className={d.usedPct >= 90 ? 'bad' : ''}>{d.usedPct}%</span>
                  </dd>
                </div>
              ))}
            </dl>
          </div>

          <div className="sect">
            <h3 className="label">Stack</h3>
            <dl className="cols">
              <dt>PHP</dt>
              <dd>
                <Mark kind={gap === 'current' ? 'confirmed' : gap === 'branch' ? 'struck' : 'hollow'} />
                <span>
                  {data.php}
                  {gap !== 'current' && ` → ${server.phpTarget}`}
                </span>
              </dd>
              <dt>Web</dt>
              <dd>{data.webserver}</dd>
              <dt>Database</dt>
              <dd>{data.db}</dd>
            </dl>
          </div>

          <div className="sect">
            <h3 className="label">Failed services</h3>
            {data.failedUnits.length === 0 ? (
              <span className="good" style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
                <Mark kind="confirmed" /> None failed
              </span>
            ) : (
              data.failedUnits.map((u) => (
                <div key={u} className="bad" style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
                  <Mark kind="struck" /> {u}
                </div>
              ))
            )}
          </div>

          <div className="sect">
            <h3 className="label">Containers</h3>
            {data.containers === null ? (
              <span className="dim" style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
                <Mark kind="hollow" /> Docker is not installed here
              </span>
            ) : (
              <table>
                <thead>
                  <tr>
                    <th />
                    <th>Name</th>
                    <th>Image</th>
                    <th>Ports</th>
                  </tr>
                </thead>
                <tbody>
                  {data.containers.map((c) => (
                    <tr key={c.name}>
                      <td>
                        <Mark kind={c.status === 'running' ? 'filled' : 'struck'} />
                      </td>
                      <td>{c.name}</td>
                      <td>{c.image}</td>
                      <td>{c.ports}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </div>
        </>
      )}
    </aside>
  )
}
