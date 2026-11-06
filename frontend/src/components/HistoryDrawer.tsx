import { History, Pin, PinOff, Search, X } from 'lucide-react'
import { useCallback, useEffect, useMemo, useState } from 'react'
import type { Backend } from '../api'
import { ago, by, exact, KIND_LABEL, matches, sizeDelta } from '../lib/history'
import type { Server, Version } from '../types'

export interface DiffPick {
  /** Older side, then newer side. */
  a: { label: string; text: string }
  b: { label: string; text: string }
  /** The version Restore would put back (the older side when it is a stored version). */
  restore?: Version
}

interface Props {
  server: Server
  path: string
  backend: Backend
  /** Changes when the list should be read again (after a save or restore). */
  rev: number
  /** The text in the editor right now. */
  current: () => string
  live: boolean
  noHistory: boolean
  onPick: (d: DiffPick | null) => void
  onCopy: (mode: '' | 'on' | 'off') => void
  onClose: () => void
}

/** The versions of one file, newest first. Click one to see what changed against the editor; shift-click a second to compare two. */
export function HistoryDrawer({ server, path, backend, rev, current, live, noHistory, onPick, onCopy, onClose }: Props) {
  const [list, setList] = useState<Version[] | null>(null)
  const [err, setErr] = useState('')
  const [q, setQ] = useState('')
  const [sel, setSel] = useState<string[]>([])
  const [noteText, setNoteText] = useState('')
  const [busy, setBusy] = useState(false)

  const load = useCallback(async () => {
    try {
      setErr('')
      setList(await backend.history.file(server.id, path))
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e))
    }
  }, [backend, server.id, path])
  useEffect(() => void load(), [load, rev])
  useEffect(() => (setSel([]), onPick(null)), [path, rev]) // eslint-disable-line react-hooks/exhaustive-deps

  const shown = useMemo(() => (list ?? []).filter((v) => matches(v, q)), [list, q])
  const byId = useMemo(() => new Map((list ?? []).map((v) => [v.id, v])), [list])
  const label = (v: Version) => `${KIND_LABEL[v.kind]}, ${ago(v.time)}`

  const pick = async (e: React.MouseEvent, v: Version) => {
    const next = e.shiftKey && sel.length ? [sel[0], v.id] : [v.id]
    setSel(next)
    setNoteText(v.note ?? '')
    setBusy(true)
    try {
      const vs = next.map((id) => byId.get(id)!).sort((x, y) => x.time - y.time)
      const texts = await Promise.all(vs.map((x) => backend.history.content(server.id, x.id)))
      if (vs.length === 2) onPick({ a: { label: label(vs[0]), text: texts[0] }, b: { label: label(vs[1]), text: texts[1] }, restore: vs[0] })
      else onPick({ a: { label: label(vs[0]), text: texts[0] }, b: { label: 'Editor now', text: current() }, restore: vs[0] })
    } catch (er) {
      setErr(er instanceof Error ? er.message : String(er))
      onPick(null)
    }
    setBusy(false)
  }

  const one = sel.length === 1 ? byId.get(sel[0]) : undefined
  const pinIt = async (pinned: boolean) => {
    if (!one) return
    try {
      await backend.history.pin(server.id, one.id, pinned, noteText)
      await load()
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e))
    }
  }

  const copyOn = server.historyCopy === 'on' || (!server.historyCopy && !server.production)
  return (
    <aside className="hdrawer" aria-label="File history">
      <header>
        <strong>
          <History size={14} strokeWidth={2} /> History
        </strong>
        <button className="btn icon" onClick={onClose} aria-label="Close history">
          <X size={14} strokeWidth={2} />
        </button>
      </header>
      {noHistory ? (
        <p className="hnote">History is not kept for this file. Files that usually hold keys, such as <code>.pem</code> and <code>id_rsa</code>, are never recorded, and history can be switched off in Admin.</p>
      ) : (
        <>
          <label className="hsearch">
            <Search size={13} strokeWidth={2} />
            <input type="search" placeholder="Filter by person, note or kind" aria-label="Filter versions" value={q} onChange={(e) => setQ(e.target.value)} />
          </label>
          <ol className="hlist" aria-label="Versions">
            {list === null && !err && <li className="hnone">Reading history…</li>}
            {list?.length === 0 && <li className="hnone">No versions yet. The first save keeps the file as it is now.</li>}
            {shown.map((v, i) => (
              <li key={v.id}>
                <button className="hitem" aria-pressed={sel.includes(v.id)} onClick={(e) => void pick(e, v)} title={exact(v.time)}>
                  <span className="hwhen">
                    {ago(v.time)}
                    {i === 0 && !q && <span className="hnew">newest</span>}
                    {v.pinned && <Pin size={12} strokeWidth={2} aria-label="pinned" />}
                  </span>
                  <span className="hby" data-kind={v.kind}>
                    {by(v)}
                  </span>
                  <span className="hmeta">
                    {KIND_LABEL[v.kind]} · {v.size} B {sizeDelta(v, shown[i + 1])}
                    {v.remote ? ' · from the server copy' : ''}
                  </span>
                  {v.note && <span className="hnotee">{v.note}</span>}
                </button>
              </li>
            ))}
          </ol>
          {one && (
            <div className="hpin">
              <input value={noteText} onChange={(e) => setNoteText(e.target.value)} placeholder="Add a note, e.g. before the upgrade" aria-label="Note for this version" maxLength={120} />
              <button className="btn" onClick={() => void pinIt(true)} title="Keep this version even when old ones are cleaned up">
                <Pin size={13} strokeWidth={2} /> {one.pinned ? 'Save note' : 'Pin'}
              </button>
              {one.pinned && (
                <button className="btn icon" onClick={() => void pinIt(false)} aria-label="Unpin">
                  <PinOff size={13} strokeWidth={2} />
                </button>
              )}
            </div>
          )}
          {!live && <p className="hnote">Connect to this server to restore a version.</p>}
          {err && (
            <p className="hnote bad" role="alert">
              {err}
            </p>
          )}
          <footer>
            <label className="hcopy">
              <input type="checkbox" checked={copyOn} onChange={(e) => onCopy(e.target.checked ? 'on' : 'off')} />
              <span>Also keep a copy on this server{server.production && !server.historyCopy ? ' (production: off until you turn it on)' : ''}</span>
            </label>
            <span className="hnote">Copies live in <code>~/.sshdeck/history</code> so teammates see each other’s edits. Old versions can hold secrets; delete them in Admin, History.</span>
          </footer>
        </>
      )}
      {busy && <span className="sr-only" role="status">Loading version</span>}
    </aside>
  )
}
