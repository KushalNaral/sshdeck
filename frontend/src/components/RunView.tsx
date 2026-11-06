import { useEffect, useMemo, useRef, useState } from 'react'
import type { Backend, PushPolicy } from '../api'
import { messageOf } from '../lib/config'
import { snippetsApi, useSnippets } from '../lib/snippets'
import { viaLabel } from '../lib/status'
import type { PushResult, RunResult, Server, Session, Transfer, Vpn } from '../types'
import { Mark } from './Mark'

interface Props {
  servers: Server[]
  vpns: Vpn[]
  sessions: Record<string, Session>
  backend: Backend
  /** The normal connect flow, so the VPN and production questions still get asked. Resolves when it has finished. */
  onConnect: (id: string) => Promise<void>
  ask: (a: { title: string; body: string; label: string; run: () => void }) => void
}

const STATE: Record<RunResult['state'], string> = {
  waiting: 'Waiting',
  running: 'Running…',
  done: 'Done',
  failed: 'Failed',
  cancelled: 'Cancelled',
  skipped: 'Skipped',
}

/** Run one command on several connected servers and read the answers side by side. */
export function RunView({ servers, vpns, sessions, backend, onConnect, ask }: Props) {
  const [picked, setPicked] = useState<Set<string>>(new Set())
  const [cmd, setCmd] = useState('')
  const [order, setOrder] = useState<string[]>([])
  const [results, setResults] = useState<Record<string, RunResult>>({})
  const [runId, setRunId] = useState<string | null>(null)
  const [onlyFailed, setOnlyFailed] = useState(false)
  const [err, setErr] = useState<string | null>(null)
  const [naming, setNaming] = useState<string | null>(null)
  const [connecting, setConnecting] = useState(false)
  const [what, setWhat] = useState<'command' | 'copy'>('command')
  const [dir, setDir] = useState('')
  const [policy, setPolicy] = useState<PushPolicy>('skip')
  const [pushRes, setPushRes] = useState<PushResult[] | null>(null)
  const [xfers, setXfers] = useState<Transfer[]>([])
  useEffect(() => backend.files.onTransfers(setXfers), [backend])
  const off = useRef<() => void>(() => {})
  const { list: snippets, reload } = useSnippets()
  useEffect(() => () => off.current(), [])

  const live = (id: string) => sessions[id]?.state === 'live'
  const sorted = useMemo(() => [...servers].sort((a, b) => a.name.localeCompare(b.name)), [servers])
  const liveIds = sorted.filter((s) => live(s.id)).map((s) => s.id)
  const idle = [...picked].filter((id) => !live(id))
  const busy = order.some((id) => ['waiting', 'running'].includes(results[id]?.state ?? ''))
  const toggle = (id: string) => setPicked((p) => (p.has(id) ? new Set([...p].filter((x) => x !== id)) : new Set(p).add(id)))

  const start = async () => {
    setErr(null)
    const ids = [...picked]
    off.current()
    try {
      const id = await backend.run.start(ids, cmd)
      const merge = (r: RunResult) => setResults((p) => ({ ...p, [r.server]: r }))
      off.current = backend.run.onResult(id, merge)
      setRunId(id)
      setOrder(ids)
      setResults(Object.fromEntries(ids.map((s) => [s, { server: s, name: servers.find((x) => x.id === s)?.name ?? s, state: 'waiting', exit: 0, output: '', ms: 0 } as RunResult])))
      void backend.run.results(id).then((rs) => rs.forEach(merge)) // anything that happened before we listened
    } catch (e) {
      setErr(messageOf(e))
    }
  }
  const run = () => {
    const prod = [...picked].filter((id) => live(id) && servers.find((s) => s.id === id)?.production).map((id) => servers.find((s) => s.id === id)!.name)
    if (!prod.length) return void start()
    ask({
      title: `Run on production?`,
      body: `“${cmd.trim().split('\n')[0].slice(0, 80)}” will run on ${prod.join(', ')}.`,
      label: 'Run on production',
      run: () => void start(),
    })
  }
  const push = (folder: boolean) => {
    const go = async () => {
      setErr(null)
      setPushRes(null)
      try {
        const r = await backend.run.push([...picked], dir.trim(), folder, policy)
        if (r) setPushRes(r)
      } catch (e) {
        setErr(messageOf(e))
      }
    }
    const prod = [...picked].filter((id) => live(id) && servers.find((s) => s.id === id)?.production).map((id) => servers.find((s) => s.id === id)!.name)
    if (!prod.length) return void go()
    ask({ title: 'Copy to production?', body: `Files will be copied into ${dir.trim()} on ${prod.join(', ')}.`, label: 'Copy to production', run: () => void go() })
  }
  const sent = xfers.filter((x) => x.up && picked.has(x.serverId))
  const nameOf = (id: string) => servers.find((s) => s.id === id)?.name ?? id
  const connectPicked = async () => {
    setConnecting(true)
    for (const id of idle) await onConnect(id) // one at a time: a VPN asks for approval
    setConnecting(false)
  }
  const saveSnippet = async () => {
    if (!naming?.trim()) return
    try {
      await snippetsApi.save({ id: '', name: naming.trim(), command: cmd })
      setNaming(null)
      reload()
    } catch (e) {
      setErr(messageOf(e))
    }
  }

  const shown = order.filter((id) => !onlyFailed || ['failed', 'skipped'].includes(results[id]?.state ?? ''))
  const failed = order.filter((id) => results[id]?.state === 'failed').length

  return (
    <div className="runview">
      <section className="run-servers" aria-label="Servers to run on">
        <h3 className="label">Servers · {picked.size} picked</h3>
        <div className="actions">
          <button className="btn" onClick={() => setPicked(new Set(liveIds))} disabled={!liveIds.length}>Pick connected</button>
          <button className="btn" onClick={() => setPicked(new Set())} disabled={!picked.size}>Clear</button>
        </div>
        <ul className="run-list">
          {sorted.map((s) => (
            <li key={s.id}>
              <label>
                <input type="checkbox" checked={picked.has(s.id)} onChange={() => toggle(s.id)} />
                <Mark kind={live(s.id) ? 'confirmed' : 'hollow'} />
                <span className="run-name">
                  {s.name}
                  {s.production && <span className="tag"> production</span>}
                </span>
                <span className="dim">{viaLabel(s, vpns)}</span>
              </label>
            </li>
          ))}
        </ul>
        {idle.length > 0 && (
          <div>
            <p className="dim">{idle.length} picked {idle.length === 1 ? 'server is' : 'servers are'} not connected and would be skipped.</p>
            <button className="btn" onClick={() => void connectPicked()} disabled={connecting}>{connecting ? 'Connecting…' : `Connect ${idle.length}`}</button>
          </div>
        )}
      </section>

      <section className="run-main" aria-label="Command">
        <div className="orange" role="group" aria-label="What to do">
          <button className="btn" aria-pressed={what === 'command'} onClick={() => setWhat('command')}>Run a command</button>
          <button className="btn" aria-pressed={what === 'copy'} onClick={() => setWhat('copy')}>Copy files</button>
        </div>
        {what === 'copy' ? (
          <>
            <h3 className="label">Copy to the picked servers</h3>
            <label className="run-dir">
              <span>Folder on the servers</span>
              <input aria-label="Folder on the servers" spellCheck={false} placeholder="/var/www/app/config" value={dir} onChange={(e) => setDir(e.target.value)} />
            </label>
            <label className="run-dir">
              <span>If a file is already there</span>
              <select aria-label="If a file is already there" value={policy} onChange={(e) => setPolicy(e.target.value as PushPolicy)}>
                <option value="skip">Leave it alone</option>
                <option value="replace">Replace it</option>
                <option value="keep">Keep both</option>
              </select>
            </label>
            <div className="actions">
              <button className="btn solid" disabled={!dir.trim() || !picked.size} onClick={() => push(false)}>Files…</button>
              <button className="btn solid" disabled={!dir.trim() || !picked.size} onClick={() => push(true)}>Folder…</button>
              <span className="dim">Only connected servers get the files; the rest are skipped.</span>
            </div>
            {err && <p className="bad" role="alert">{err}</p>}
            {pushRes && (
              <ul className="push-res" aria-live="polite">
                {pushRes.map((r) => (
                  <li key={r.server} className={r.error ? 'bad' : ''}>
                    <strong>{r.name}</strong>{' '}
                    {r.error ? r.error : r.started ? `Copying ${r.started} item${r.started === 1 ? '' : 's'}.` : 'Nothing copied.'}
                    {r.conflicts?.length ? ` Already there: ${r.conflicts.join(', ')}.` : ''}
                  </li>
                ))}
              </ul>
            )}
            {sent.length > 0 && (
              <ul className="push-res" aria-label="Copy progress">
                {sent.map((x) => (
                  <li key={x.id} className={x.state === 'failed' ? 'bad' : ''}>
                    <strong>{nameOf(x.serverId)}</strong> {x.name}{' '}
                    <span className="dim">{x.state === 'running' && x.total > 0 ? `${Math.round((x.done / x.total) * 100)}%` : x.state}{x.error ? `: ${x.error}` : ''}</span>
                  </li>
                ))}
              </ul>
            )}
          </>
        ) : (
          <>
        <h3 className="label">Command</h3>
        <div className="run-bar">
          <select aria-label="Saved snippets" value="" onChange={(e) => e.target.value && setCmd(snippets.find((x) => x.id === e.target.value)?.command ?? '')}>
            <option value="">Snippets…</option>
            {snippets.map((x) => (
              <option key={x.id} value={x.id}>{x.name}</option>
            ))}
          </select>
          {naming === null ? (
            <button className="btn" onClick={() => setNaming('')} disabled={!cmd.trim()}>Save as snippet</button>
          ) : (
            <span className="run-name-box">
              <input aria-label="Snippet name" autoFocus value={naming} placeholder="Name" onChange={(e) => setNaming(e.target.value)} onKeyDown={(e) => e.key === 'Enter' && void saveSnippet()} />
              <button className="btn" onClick={() => void saveSnippet()}>Save</button>
              <button className="btn" onClick={() => setNaming(null)}>Cancel</button>
            </span>
          )}
        </div>
        <textarea className="run-cmd" aria-label="Command to run" spellCheck={false} rows={4} value={cmd} placeholder="df -h" onChange={(e) => setCmd(e.target.value)} />
        <div className="actions">
          <button className="btn solid" onClick={run} disabled={!cmd.trim() || !picked.size || busy}>Run on {picked.size} server{picked.size === 1 ? '' : 's'}</button>
          {busy && runId && <button className="btn" onClick={() => backend.run.cancel(runId)}>Stop</button>}
          {order.length > 0 && !busy && failed > 0 && (
            <label className="run-only"><input type="checkbox" checked={onlyFailed} onChange={(e) => setOnlyFailed(e.target.checked)} /> Only failed ({failed})</label>
          )}
        </div>
        {err && <p className="bad" role="alert">{err}</p>}

        <div className="run-cards" aria-live="polite">
          {shown.map((id) => {
            const r = results[id]
            if (!r) return null
            return (
              <article key={id} className={`run-card ${r.state}`}>
                <header>
                  <strong>{r.name}</strong>
                  <span className="dim">
                    {STATE[r.state]}
                    {r.state === 'failed' && r.exit > 0 ? ` · exit ${r.exit}` : ''}
                    {r.ms > 0 ? ` · ${(r.ms / 1000).toFixed(1)} s` : ''}
                  </span>
                  <button className="btn" onClick={() => navigator.clipboard?.writeText(r.output).catch(() => {})} disabled={!r.output}>Copy</button>
                </header>
                {r.error && <p className="bad">{r.error}</p>}
                {r.truncated && <p className="dim">Showing the last part of the output.</p>}
                {r.output && <pre>{r.output}</pre>}
              </article>
            )
          })}
        </div>
          </>
        )}
      </section>
    </div>
  )
}
