import { RefreshCw } from 'lucide-react'
import { useState, type ReactNode } from 'react'
import { phpGap } from '../lib/php'
import type { Analysis as A, ContainerTab, Server } from '../types'
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
  error?: string
  /** Whether a shell is open; a scan is only offered automatically then. */
  connected: boolean
  onRescan: () => void
  /** Open a shell inside a running container. */
  onExec: (c: ContainerTab) => void
  connection?: ReactNode
}

/** Read-only picture of the box, laid out like departure columns. */
export function Analysis({ server, data, scanning, error, connected, onRescan, onExec, connection }: Props) {
  const gap = phpGap(server.php, server.phpTarget)
  const [runAs, setRunAs] = useState<'default' | 'root' | 'other'>('root')
  const [other, setOther] = useState('')
  const [sudo, setSudo] = useState(false)
  const user = runAs === 'default' ? '' : runAs === 'root' ? 'root' : other.trim()
  const userOk = runAs !== 'other' || /^[A-Za-z0-9_.][A-Za-z0-9_.:-]*$/.test(user)
  const memPct = data?.memTotalMb ? Math.round((data.memUsedMb / data.memTotalMb) * 100) : 0
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
        <div className="sect dim">
          {scanning ? (
            'Reading the server: OS, PHP, disks, services, containers.'
          ) : error ? (
            <span className="bad" role="alert">{error}</span>
          ) : connected ? (
            'Not scanned yet.'
          ) : (
            'Connect to read this server, or press Rescan if it is reachable.'
          )}
        </div>
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
                <Bar pct={memPct} />
                {memPct}%
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
              <>
              <div className="execbar">
                <label>
                  <span>Shell as</span>
                  <select value={runAs} onChange={(e) => setRunAs(e.target.value as typeof runAs)}>
                    <option value="root">root (admin)</option>
                    <option value="default">Container's default user</option>
                    <option value="other">Another user…</option>
                  </select>
                </label>
                {runAs === 'other' && <input aria-label="User name or uid:gid" placeholder="www-data or 1000:1000" value={other} onChange={(e) => setOther(e.target.value)} aria-invalid={!userOk} />}
                <label className="check">
                  <input type="checkbox" checked={sudo} onChange={(e) => setSudo(e.target.checked)} />
                  <span>with sudo</span>
                </label>
              </div>
              <table>
                <thead>
                  <tr>
                    <th />
                    <th>Name</th>
                    <th>Image</th>
                    <th>Ports</th>
                    <th />
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
                      <td>
                        <button
                          className="btn"
                          disabled={!connected || c.status !== 'running' || !userOk || (runAs === 'other' && !user)}
                          title={!connected ? 'Connect to the server first' : c.status !== 'running' ? 'The container is not running' : `Open a shell in ${c.name} as ${user || 'its default user'}`}
                          onClick={() => onExec({ container: c.name, user, sudo })}
                        >
                          Shell
                        </button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
              </>
            )}
          </div>
        </>
      )}
    </aside>
  )
}
