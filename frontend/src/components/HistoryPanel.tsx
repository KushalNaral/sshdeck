import { RefreshCw } from 'lucide-react'
import { useCallback, useEffect, useMemo, useState } from 'react'
import type { Backend } from '../api'
import { ago, by, exact, KIND_LABEL, matches } from '../lib/history'
import { basename, dirname, splitHist, targetOf } from '../lib/files'
import type { Version } from '../types'

interface Props {
  serverId: string
  serverName: string
  backend: Backend
  /** Opens a file's history in the editor. target is the server id, or "serverId@container". */
  onOpen: (target: string, path: string) => void
  live: boolean
}

/** Who edited what, when, on this server, newest first. */
export function HistoryPanel({ serverId, serverName, backend, onOpen, live }: Props) {
  const [rows, setRows] = useState<Version[] | null>(null)
  const [q, setQ] = useState('')
  const [err, setErr] = useState('')
  const load = useCallback(async () => {
    setErr('')
    try {
      setRows(await backend.history.server(serverId, 200))
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e))
    }
  }, [backend, serverId])
  useEffect(() => void load(), [load, live])
  const shown = useMemo(() => (rows ?? []).filter((v) => matches(v, q)), [rows, q])
  return (
    <div className="files hpanel">
      <div className="files-bar">
        <input className="search" type="search" placeholder="Filter by file, person or kind" aria-label="Filter edits" value={q} onChange={(e) => setQ(e.target.value)} style={{ flex: 1 }} />
        <button className="btn icon" onClick={() => void load()} title="Refresh" aria-label="Refresh">
          <RefreshCw size={14} strokeWidth={2} />
        </button>
      </div>
      <ol className="hfeed" aria-label={`Edits on ${serverName}`}>
        {rows === null && !err && <li className="hnone">Reading history…</li>}
        {rows?.length === 0 && <li className="hnone">Nothing edited through sshdeck on {serverName} yet. Open a file from the Files tab and save it.</li>}
        {shown.map((v) => {
          const { container, path } = splitHist(v.path)
          return (
          <li key={v.id}>
            <button className="hrow" onClick={() => onOpen(targetOf(serverId, container), path)} disabled={!live} title={live ? `Open the history of ${v.path}` : 'Connect to open files'}>
              <span className="hfile">
                <strong>{basename(path)}</strong> <span className="dim">{container ? `${container}: ` : ''}{dirname(path)}</span>
              </span>
              <span className="hby" data-kind={v.kind}>
                {by(v)}
              </span>
              <span className="hmeta" title={exact(v.time)}>
                {KIND_LABEL[v.kind]} · {ago(v.time)}
              </span>
            </button>
          </li>
          )
        })}
      </ol>
      {err && (
        <div className="notice bad" role="alert">
          <span>{err}</span>
        </div>
      )}
    </div>
  )
}
