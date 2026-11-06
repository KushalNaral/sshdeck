import { Copy, RefreshCw } from 'lucide-react'
import { useState, type ReactNode } from 'react'
import { versionGap } from '../lib/version'
import { LEVEL_NAME, type Triage } from '../lib/triage'
import type { Backend } from '../api'
import type { Analysis as A, ContainerTab, Server } from '../types'
import { ContainerActions, ServiceRow, type Ask } from './Actions'
import { Mark } from './Mark'
import { Tag } from './Tag'

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
  triage?: Triage
  data?: A
  scanning: boolean
  error?: string
  /** Whether a shell is open; a scan is only offered automatically then. */
  connected: boolean
  onRescan: () => void
  /** Open a shell inside a running container. */
  onExec: (c: ContainerTab) => void
  /** Follow a log: a failed service's journal or a container's output. */
  onLog: (kind: 'journal' | 'container', arg: string) => void
  /** Start tracking a found piece of software, or update the version already tracked. */
  onTrack: (name: string, version: string) => void
  /** Copies the ssh command for this server; resolves with an error message or null. */
  onCopySsh: () => Promise<string | null>
  connection?: ReactNode
  logins?: ReactNode
  backend: Backend
  /** The confirm dialog, for restarts and stops. */
  ask: Ask
}

/** Read-only picture of the box, laid out like departure columns. */
export function Analysis({ server, triage, data, scanning, error, connected, onRescan, onExec, onLog, onTrack, onCopySsh, connection, logins, backend, ask }: Props) {
  const [runAs, setRunAs] = useState<'default' | 'root' | 'other'>('root')
  const [other, setOther] = useState('')
  const [sudo, setSudo] = useState(false)
  const [copied, setCopied] = useState<string | null>(null)
  const user = runAs === 'default' ? '' : runAs === 'root' ? 'root' : other.trim()
  const userOk = runAs !== 'other' || /^[A-Za-z0-9_.][A-Za-z0-9_.:-]*$/.test(user)
  const memPct = data?.memTotalMb ? Math.round((data.memUsedMb / data.memTotalMb) * 100) : 0
  return (
    <aside className="pane analysis" aria-label={`Analysis of ${server.name}`}>
      <div className="sect" style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <h2 className="name">{server.name}</h2>
        <button
          className="btn"
          title="Copy the ssh command for this server"
          onClick={async () => {
            const m = await onCopySsh()
            setCopied(m ?? 'Copied')
            setTimeout(() => setCopied(null), 2000)
          }}
          style={{ color: 'var(--ink)', marginLeft: 'auto', marginRight: 6 }}
        >
          <Copy size={14} strokeWidth={2} />
          {copied ?? 'Copy ssh'}
        </button>
        <button className="btn" onClick={onRescan} disabled={scanning} style={{ color: 'var(--ink)' }}>
          <RefreshCw size={14} strokeWidth={2} />
          {scanning ? 'Scanning' : 'Rescan'}
        </button>
      </div>

      {connection}
      {logins}

      {triage && (
        <div className="sect">
          <h3 className="label">
            Level {triage.level}: {LEVEL_NAME[triage.level]}
          </h3>
          <ul className="why">
            {triage.findings.map((f) => (
              <li key={f.text}>
                <Tag level={f.level} still />
                <span>{f.text}</span>
              </li>
            ))}
          </ul>
        </div>
      )}

      {!data ? (
        <div className="sect dim">
          {scanning ? (
            'Reading the server: OS, software, disks, services, containers.'
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
            <h3 className="label">Software</h3>
            {data.software.length === 0 ? (
              <span className="dim">Nothing recognised on this server.</span>
            ) : (
              <dl className="cols">
                {data.software.map((f) => {
                  const t = server.software.find((x) => x.name.toLowerCase() === f.name.toLowerCase())
                  const gap = t?.target ? versionGap(f.version, t.target) : null
                  return (
                    <div key={f.name} style={{ display: 'contents' }}>
                      <dt>{f.name}</dt>
                      <dd>
                        {gap && <Mark kind={gap === 'current' ? 'confirmed' : gap === 'branch' ? 'struck' : 'hollow'} />}
                        <span>
                          {f.version}
                          {t && gap && gap !== 'current' && ` → ${t.target}`}
                        </span>
                        {(!t || t.version !== f.version) && (
                          <button className="btn" onClick={() => onTrack(f.name, f.version)} title="Remember this version for this server">
                            {t ? 'Update' : 'Track'}
                          </button>
                        )}
                      </dd>
                    </div>
                  )
                })}
              </dl>
            )}
          </div>

          <div className="sect">
            <h3 className="label">Failed services</h3>
            {data.failedUnits.length === 0 ? (
              <span className="good" style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
                <Mark kind="confirmed" /> None failed
              </span>
            ) : (
              data.failedUnits.map((u) => <ServiceRow key={u} unit={u} server={server} connected={connected} backend={backend} ask={ask} onDone={onRescan} onLog={() => onLog('journal', u)} />)
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
                        </button>{' '}
                        <button className="btn" disabled={!connected} onClick={() => onLog('container', c.name)} title={connected ? `Follow the output of ${c.name}` : 'Connect to the server first'}>
                          Log
                        </button>{' '}
                        <ContainerActions c={c} server={server} connected={connected} backend={backend} ask={ask} onDone={onRescan} />
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
